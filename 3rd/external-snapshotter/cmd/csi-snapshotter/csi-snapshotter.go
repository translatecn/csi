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
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"

	"google.golang.org/grpc"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	coreinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/kubernetes-csi/csi-lib-utils/connection"
	"github.com/kubernetes-csi/csi-lib-utils/leaderelection"
	"github.com/kubernetes-csi/csi-lib-utils/metrics"
	csirpc "github.com/kubernetes-csi/csi-lib-utils/rpc"
	controller "local/3rd/external-snapshotter/pkg/sidecar-controller"
	"local/3rd/external-snapshotter/pkg/snapshotter"

	clientset "local/3rd/external-snapshotter/client/clientset/versioned"
	snapshotscheme "local/3rd/external-snapshotter/client/clientset/versioned/scheme"
	informers "local/3rd/external-snapshotter/client/informers/externalversions"
	"local/3rd/external-snapshotter/pkg/group_snapshotter"
	utils "local/3rd/external-snapshotter/pkg/utils"
)

const (
	// Default timeout of short CSI calls like GetPluginInfo
	defaultCSITimeout = time.Minute
)

// Command line flags
var (
	kubeconfig                  = pointer.String("")
	csiAddress                  = pointer.String("/csi/csi.sock")
	resyncPeriod                = pointer.Duration(15 * time.Minute)
	snapshotNamePrefix          = pointer.String("snapshot")
	snapshotNameUUIDLength      = pointer.Int(-1)
	showVersion                 = pointer.Bool(false)
	threads                     = pointer.Int(10)
	csiTimeout                  = pointer.Duration(defaultCSITimeout)
	extraCreateMetadata         = pointer.Bool(false)
	leaderElection              = pointer.Bool(false)
	leaderElectionNamespace     = pointer.String("")
	leaderElectionLeaseDuration = pointer.Duration(15 * time.Second)
	leaderElectionRenewDeadline = pointer.Duration(10 * time.Second)
	leaderElectionRetryPeriod   = pointer.Duration(5 * time.Second)
	kubeAPIQPS                  = pointer.Float64(5)
	kubeAPIBurst                = pointer.Int(10)
	metricsAddress              = pointer.String("")
	httpEndpoint                = pointer.String("")
	metricsPath                 = pointer.String("/metrics")
	retryIntervalStart          = pointer.Duration(time.Second)
	retryIntervalMax            = pointer.Duration(5 * time.Minute)
	enableNodeDeployment        = pointer.Bool(false)
	enableVolumeGroupSnapshots  = pointer.Bool(false)
	groupSnapshotNamePrefix     = pointer.String("groupsnapshot")
	groupSnapshotNameUUIDLength = pointer.Int(-1)
)

var (
	version = "unknown"
	prefix  = "external-snapshotter-leader"
)

func main() {

	klog.Infof("Version: %s", version)

	// If distributed snapshotting is enabled and leaderElection is also set to true, return
	if *enableNodeDeployment && *leaderElection {
		klog.Error("Leader election cannot happen when node-deployment is set to true")
		return
	}

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
	var snapshotContentfactory informers.SharedInformerFactory
	if *enableNodeDeployment {
		node := os.Getenv("NODE_NAME")
		if node == "" {
			klog.Fatal("The NODE_NAME environment variable must be set when using --enable-node-deployment.")
		}
		snapshotContentfactory = informers.NewSharedInformerFactoryWithOptions(snapClient, *resyncPeriod, informers.WithTweakListOptions(func(lo *v1.ListOptions) {
			lo.LabelSelector = labels.Set{utils.VolumeSnapshotContentManagedByLabel: node}.AsSelector().String()
		}),
		)
	} else {
		snapshotContentfactory = factory
	}

	// Add Snapshot types to the default Kubernetes so events can be logged for them
	snapshotscheme.AddToScheme(scheme.Scheme)

	if *metricsAddress != "" && *httpEndpoint != "" {
		klog.Error("only one of `--metrics-address` and `--http-endpoint` can be set.")
		return
	}
	addr := *metricsAddress
	if addr == "" {
		addr = *httpEndpoint
	}

	// Connect to CSI.
	metricsManager := metrics.NewCSIMetricsManager("" /* driverName */)
	csiConn, err := connection.Connect(
		*csiAddress,
		metricsManager,
		connection.OnConnectionLoss(connection.ExitOnConnectionLoss()))
	if err != nil {
		klog.Errorf("error connecting to CSI driver: %v", err)
		return
	}

	// Pass a context with a timeout
	ctx, cancel := context.WithTimeout(context.Background(), *csiTimeout)
	defer cancel()

	// Find driver name
	driverName, err := csirpc.GetDriverName(ctx, csiConn)
	if err != nil {
		klog.Errorf("error getting CSI driver name: %v", err)
		return
	}

	klog.V(2).Infof("CSI driver name: %q", driverName)

	// Prepare http endpoint for metrics + leader election healthz
	mux := http.NewServeMux()
	if addr != "" {
		metricsManager.RegisterToServer(mux, *metricsPath)
		metricsManager.SetDriverName(driverName)
		go func() {
			klog.Infof("ServeMux listening at %q", addr)
			err := http.ListenAndServe(addr, mux)
			if err != nil {
				klog.Fatalf("Failed to start HTTP server at specified address (%q) and metrics path (%q): %s", addr, *metricsPath, err)
			}
		}()
	}

	// Check it's ready
	if err = csirpc.ProbeForever(csiConn, *csiTimeout); err != nil {
		klog.Errorf("error waiting for CSI driver to be ready: %v", err)
		return
	}

	// Find out if the driver supports create/delete snapshot.
	supportsCreateSnapshot, err := supportsControllerCreateSnapshot(ctx, csiConn)
	if err != nil {
		klog.Errorf("error determining if driver supports create/delete snapshot operations: %v", err)
		return
	}
	if !supportsCreateSnapshot {
		klog.Errorf("CSI driver %s does not support ControllerCreateSnapshot", driverName)
		return
	}

	if len(*snapshotNamePrefix) == 0 {
		klog.Error("Snapshot name prefix cannot be of length 0")
		return
	}

	klog.V(2).Infof("Start NewCSISnapshotSideCarController with snapshotter [%s] kubeconfig [%s] csiTimeout [%+v] csiAddress [%s] resyncPeriod [%+v] snapshotNamePrefix [%s] snapshotNameUUIDLength [%d]", driverName, *kubeconfig, *csiTimeout, *csiAddress, *resyncPeriod, *snapshotNamePrefix, snapshotNameUUIDLength)

	snapShotter := snapshotter.NewSnapshotter(csiConn)
	var groupSnapshotter group_snapshotter.GroupSnapshotter
	if *enableVolumeGroupSnapshots {
		supportsCreateVolumeGroupSnapshot, err := supportsGroupControllerCreateVolumeGroupSnapshot(ctx, csiConn)
		if err != nil {
			klog.Errorf("error determining if driver supports create/delete group snapshot operations: %v", err)
		} else if !supportsCreateVolumeGroupSnapshot {
			klog.Warningf("CSI driver %s does not support GroupControllerCreateVolumeGroupSnapshot when the --enable-volume-group-snapshots flag is true", driverName)
		}
		groupSnapshotter = group_snapshotter.NewGroupSnapshotter(csiConn)
		if len(*groupSnapshotNamePrefix) == 0 {
			klog.Error("group snapshot name prefix cannot be of length 0")
			return
		}
	}

	ctrl := controller.NewCSISnapshotSideCarController(
		snapClient,
		kubeClient,
		driverName,
		snapshotContentfactory.Snapshot().V1().VolumeSnapshotContents(),
		factory.Snapshot().V1().VolumeSnapshotClasses(),
		snapShotter,
		groupSnapshotter,
		*csiTimeout,
		*resyncPeriod,
		*snapshotNamePrefix,
		*snapshotNameUUIDLength,
		*groupSnapshotNamePrefix,
		*groupSnapshotNameUUIDLength,
		*extraCreateMetadata,
		workqueue.NewItemExponentialFailureRateLimiter(*retryIntervalStart, *retryIntervalMax),
		*enableVolumeGroupSnapshots,
		snapshotContentfactory.Groupsnapshot().V1alpha1().VolumeGroupSnapshotContents(),
		snapshotContentfactory.Groupsnapshot().V1alpha1().VolumeGroupSnapshotClasses(),
		workqueue.NewItemExponentialFailureRateLimiter(*retryIntervalStart, *retryIntervalMax),
	)

	run := func(context.Context) {
		// run...
		stopCh := make(chan struct{})
		snapshotContentfactory.Start(stopCh)
		factory.Start(stopCh)
		coreFactory.Start(stopCh)
		go ctrl.Run(*threads, stopCh)

		// ...until SIGINT
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt)
		<-c
		close(stopCh)
	}

	if !*leaderElection {
		run(context.TODO())
	} else {
		lockName := fmt.Sprintf("%s-%s", prefix, strings.Replace(driverName, "/", "-", -1))
		// Create a new clientset for leader election to prevent throttling
		// due to snapshot sidecar
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

func supportsControllerCreateSnapshot(ctx context.Context, conn *grpc.ClientConn) (bool, error) {
	capabilities, err := csirpc.GetControllerCapabilities(ctx, conn)
	if err != nil {
		return false, err
	}

	return capabilities[csi.ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT], nil
}

func supportsGroupControllerCreateVolumeGroupSnapshot(ctx context.Context, conn *grpc.ClientConn) (bool, error) {
	capabilities, err := csirpc.GetGroupControllerCapabilities(ctx, conn)
	if err != nil {
		return false, err
	}

	return capabilities[csi.GroupControllerServiceCapability_RPC_CREATE_DELETE_GET_VOLUME_GROUP_SNAPSHOT], nil
}
