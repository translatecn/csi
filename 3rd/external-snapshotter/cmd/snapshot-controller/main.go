/*
Copyright 2018 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"fmt"
	"k8s.io/utils/pointer"
	"math"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"time"

	v1 "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/workqueue"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"

	"k8s.io/klog/v2"

	"github.com/kubernetes-csi/csi-lib-utils/leaderelection"
	controller "local/3rd/external-snapshotter/pkg/common-controller"
	"local/3rd/external-snapshotter/pkg/metrics"

	coreinformers "k8s.io/client-go/informers"
	clientset "local/3rd/external-snapshotter/client/clientset/versioned"
	snapshotscheme "local/3rd/external-snapshotter/client/clientset/versioned/scheme"
	informers "local/3rd/external-snapshotter/client/informers/externalversions"
)

// Command line flags
var (
	kubeconfig                    = pointer.String("")
	resyncPeriod                  = pointer.Duration(15 * time.Minute)
	showVersion                   = pointer.Bool(false)
	threads                       = pointer.Int(10)
	leaderElection                = pointer.Bool(false)
	leaderElectionNamespace       = pointer.String("")
	leaderElectionLeaseDuration   = pointer.Duration(15 * time.Second)
	leaderElectionRenewDeadline   = pointer.Duration(10 * time.Second)
	leaderElectionRetryPeriod     = pointer.Duration(5 * time.Second)
	kubeAPIQPS                    = pointer.Float64(5)
	kubeAPIBurst                  = pointer.Int(10)
	httpEndpoint                  = pointer.String("")
	metricsPath                   = pointer.String("/metrics")
	retryIntervalStart            = pointer.Duration(time.Second)
	retryIntervalMax              = pointer.Duration(5 * time.Minute)
	enableDistributedSnapshotting = pointer.Bool(false)
	preventVolumeModeConversion   = pointer.Bool(true)
	enableVolumeGroupSnapshots    = pointer.Bool(false)
	retryCRDIntervalMax           = pointer.Duration(30 * time.Second)
)

var version = "unknown"

// Checks that the VolumeSnapshot v1 CRDs exist. It will wait at most the duration specified by retryCRDIntervalMax
func ensureCustomResourceDefinitionsExist(client *clientset.Clientset, enableVolumeGroupSnapshots bool) error {
	condition := func(ctx context.Context) (bool, error) {
		var err error
		// List calls should return faster with a limit of 0.
		// We do not care about what is returned and just want to make sure the CRDs exist.
		listOptions := metav1.ListOptions{Limit: 0}

		// scoping to an empty namespace makes `List` work across all namespaces
		_, err = client.SnapshotV1().VolumeSnapshots("").List(ctx, listOptions)
		if err != nil {
			klog.Errorf("Failed to list v1 volumesnapshots with error=%+v", err)
			return false, nil
		}

		_, err = client.SnapshotV1().VolumeSnapshotClasses().List(ctx, listOptions)
		if err != nil {
			klog.Errorf("Failed to list v1 volumesnapshotclasses with error=%+v", err)
			return false, nil
		}
		_, err = client.SnapshotV1().VolumeSnapshotContents().List(ctx, listOptions)
		if err != nil {
			klog.Errorf("Failed to list v1 volumesnapshotcontents with error=%+v", err)
			return false, nil
		}
		if enableVolumeGroupSnapshots {
			_, err = client.GroupsnapshotV1alpha1().VolumeGroupSnapshots("").List(ctx, listOptions)
			if err != nil {
				klog.Errorf("Failed to list v1alpha1 volumegroupsnapshots with error=%+v", err)
				return false, nil
			}

			_, err = client.GroupsnapshotV1alpha1().VolumeGroupSnapshotClasses().List(ctx, listOptions)
			if err != nil {
				klog.Errorf("Failed to list v1alpha1 volumegroupsnapshotclasses with error=%+v", err)
				return false, nil
			}
			_, err = client.GroupsnapshotV1alpha1().VolumeGroupSnapshotContents().List(ctx, listOptions)
			if err != nil {
				klog.Errorf("Failed to list v1alpha1 volumegroupsnapshotcontents with error=%+v", err)
				return false, nil
			}
		}

		return true, nil
	}

	const retryFactor = 1.5
	const initialDuration = 100 * time.Millisecond
	backoff := wait.Backoff{
		Duration: initialDuration,
		Factor:   retryFactor,
		Steps:    math.MaxInt32, // effectively no limit until the timeout is reached
	}

	// Sanity check to make sure we have a minimum duration of 1 second to work with
	maxBackoffDuration := max(*retryCRDIntervalMax, 1*time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), maxBackoffDuration)
	defer cancel()
	if err := wait.ExponentialBackoffWithContext(ctx, backoff, condition); err != nil {
		return err
	}

	return nil
}

func main() {

	if *showVersion {
		fmt.Println(os.Args[0], version)
		os.Exit(0)
	}
	klog.Infof("Version: %s", version)

	// Create the client config. Use kubeconfig if given, otherwise assume in-cluster.
	config, err := buildConfig(*kubeconfig)
	if err != nil {
		klog.Error(err.Error())
		return
	}

	config.QPS = (float32)(*kubeAPIQPS)
	config.Burst = *kubeAPIBurst

	kubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		klog.Error(err.Error())
		return
	}

	snapClient, err := clientset.NewForConfig(config)
	if err != nil {
		klog.Errorf("Error building snapshot clientset: %s", err.Error())
		return
	}

	factory := informers.NewSharedInformerFactory(snapClient, *resyncPeriod)
	coreFactory := coreinformers.NewSharedInformerFactory(kubeClient, *resyncPeriod)
	var nodeInformer v1.NodeInformer

	if *enableDistributedSnapshotting {
		nodeInformer = coreFactory.Core().V1().Nodes()
	}

	// Create and register metrics manager
	metricsManager := metrics.NewMetricsManager()
	wg := &sync.WaitGroup{}

	mux := http.NewServeMux()
	if *httpEndpoint != "" {
		err := metricsManager.PrepareMetricsPath(mux, *metricsPath, promklog{})
		if err != nil {
			klog.Errorf("Failed to prepare metrics path: %s", err.Error())
			return
		}
		klog.Infof("Metrics path successfully registered at %s", *metricsPath)
	}

	// Add Snapshot types to the default Kubernetes so events can be logged for them
	snapshotscheme.AddToScheme(scheme.Scheme)

	klog.V(2).Infof("Start NewCSISnapshotController with kubeconfig [%s] resyncPeriod [%+v]", *kubeconfig, *resyncPeriod)

	ctrl := controller.NewCSISnapshotCommonController(
		snapClient,
		kubeClient,
		factory.Snapshot().V1().VolumeSnapshots(),
		factory.Snapshot().V1().VolumeSnapshotContents(),
		factory.Snapshot().V1().VolumeSnapshotClasses(),
		factory.Groupsnapshot().V1alpha1().VolumeGroupSnapshots(),
		factory.Groupsnapshot().V1alpha1().VolumeGroupSnapshotContents(),
		factory.Groupsnapshot().V1alpha1().VolumeGroupSnapshotClasses(),
		coreFactory.Core().V1().PersistentVolumeClaims(),
		nodeInformer,
		metricsManager,
		*resyncPeriod,
		workqueue.NewItemExponentialFailureRateLimiter(*retryIntervalStart, *retryIntervalMax),
		workqueue.NewItemExponentialFailureRateLimiter(*retryIntervalStart, *retryIntervalMax),
		workqueue.NewItemExponentialFailureRateLimiter(*retryIntervalStart, *retryIntervalMax),
		workqueue.NewItemExponentialFailureRateLimiter(*retryIntervalStart, *retryIntervalMax),
		*enableDistributedSnapshotting,
		*preventVolumeModeConversion,
		*enableVolumeGroupSnapshots,
	)

	if err := ensureCustomResourceDefinitionsExist(snapClient, *enableVolumeGroupSnapshots); err != nil {
		klog.Errorf("Exiting due to failure to ensure CRDs exist during startup: %+v", err)
		return
	}

	run := func(context.Context) {
		// run...
		stopCh := make(chan struct{})
		factory.Start(stopCh)
		coreFactory.Start(stopCh)
		go ctrl.Run(*threads, stopCh)

		// ...until SIGINT
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt)
		<-c
		close(stopCh)
	}

	// start listening & serving http endpoint if set
	if *httpEndpoint != "" {
		l, err := net.Listen("tcp", *httpEndpoint)
		if err != nil {
			klog.Fatalf("failed to listen on address[%s], error[%v]", *httpEndpoint, err)
		}
		srv := &http.Server{Addr: l.Addr().String(), Handler: mux}
		go func() {
			defer wg.Done()
			if err := srv.Serve(l); err != http.ErrServerClosed {
				klog.Fatalf("failed to start endpoint at:%s/%s, error: %v", *httpEndpoint, *metricsPath, err)
			}
		}()
		klog.Infof("Metrics http server successfully started on %s, %s", *httpEndpoint, *metricsPath)

		defer func() {
			err := srv.Shutdown(context.Background())
			if err != nil {
				klog.Errorf("Failed to shutdown metrics server: %s", err.Error())
			}

			klog.Infof("Metrics server successfully shutdown")
			wg.Done()
		}()
	}

	if !*leaderElection {
		run(context.TODO())
	} else {
		lockName := "snapshot-controller-leader"
		// Create a new clientset for leader election to prevent throttling
		// due to snapshot controller
		leClientset, err := kubernetes.NewForConfig(config)
		if err != nil {
			klog.Fatalf("failed to create leaderelection client: %v", err)
		}
		le := leaderelection.NewLeaderElection(leClientset, lockName, run)
		if *httpEndpoint != "" {
			le.PrepareHealthCheck(mux, leaderelection.DefaultHealthCheckTimeout)
		}

		if *leaderElectionNamespace != "" {
			le.WithNamespace(*leaderElectionNamespace)
		}
		le.WithLeaseDuration(*leaderElectionLeaseDuration)
		le.WithRenewDeadline(*leaderElectionRenewDeadline)
		le.WithRetryPeriod(*leaderElectionRetryPeriod)
		if err := le.Run(); err != nil {
			klog.Fatalf("failed to initialize leader election: %v", err)
		}
	}
}

func buildConfig(kubeconfig string) (*rest.Config, error) {
	if kubeconfig != "" {
		return clientcmd.BuildConfigFromFlags("", kubeconfig)
	}
	return rest.InClusterConfig()
}

type promklog struct{}

func (pl promklog) Println(v ...interface{}) {
	klog.Error(v...)
}
