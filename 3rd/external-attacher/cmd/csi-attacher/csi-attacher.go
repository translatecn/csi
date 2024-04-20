/*
Copyright 2017 The Kubernetes Authors.

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

package csi_attacher

import (
	"context"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/kubernetes-csi/csi-lib-utils/connection"
	"github.com/kubernetes-csi/csi-lib-utils/leaderelection"
	"github.com/kubernetes-csi/csi-lib-utils/metrics"
	"github.com/kubernetes-csi/csi-lib-utils/rpc"
	"google.golang.org/grpc"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/workqueue"
	csitrans "k8s.io/csi-translation-lib"
	"local/3rd/external-attacher/pkg/attacher"
	"local/3rd/external-attacher/pkg/controller"
)

const (

	// Default timeout of short CSI calls like GetPluginInfo
	csiTimeout = time.Second
)

// Command line flags
var (
	kubeconfig                  = pointer.String("")
	resync                      = pointer.Duration(10 * time.Minute)
	csiAddress                  = pointer.String("/csi/csi.sock")
	timeout                     = pointer.Duration(15 * time.Second)
	workerThreads               = pointer.Uint(10)
	retryIntervalStart          = pointer.Duration(time.Second)
	retryIntervalMax            = pointer.Duration(5 * time.Minute)
	enableLeaderElection        = pointer.Bool(false)
	leaderElectionNamespace     = pointer.String("")
	leaderElectionLeaseDuration = pointer.Duration(15 * time.Second)
	leaderElectionRenewDeadline = pointer.Duration(10 * time.Second)
	leaderElectionRetryPeriod   = pointer.Duration(5 * time.Second)
	defaultFSType               = pointer.String("")
	reconcileSync               = pointer.Duration(1 * time.Minute)
	kubeAPIQPS                  = pointer.Float64(5)
	kubeAPIBurst                = pointer.Int(10)
)

func Main() {

	// Create the client config. Use kubeconfig if given, otherwise assume in-cluster.
	config, err := buildConfig(*kubeconfig)
	if err != nil {
		klog.Error(err.Error())
		return
	}
	config.QPS = (float32)(*kubeAPIQPS)
	config.Burst = *kubeAPIBurst

	if *workerThreads == 0 {
		klog.Error("option -worker-threads must be greater than zero")
		return
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		klog.Error(err.Error())
		return
	}

	factory := informers.NewSharedInformerFactory(clientset, *resync)
	var handler controller.Handler
	metricsManager := metrics.NewCSIMetricsManager("" /* driverName */)

	// Connect to CSI.
	csiConn, err := connection.Connect(*csiAddress, metricsManager, connection.OnConnectionLoss(connection.ExitOnConnectionLoss()))
	if err != nil {
		klog.Error(err.Error())
		return
	}

	err = rpc.ProbeForever(csiConn, *timeout)
	if err != nil {
		klog.Error(err.Error())
		return
	}

	// Find driver name.
	ctx, cancel := context.WithTimeout(context.Background(), csiTimeout)
	defer cancel()
	csiAttacher, err := rpc.GetDriverName(ctx, csiConn)
	if err != nil {
		klog.Error(err.Error())
		return
	}
	klog.V(2).Infof("CSI driver name: %q", csiAttacher)

	translator := csitrans.New()
	if translator.IsMigratedCSIDriverByName(csiAttacher) {
		metricsManager = metrics.NewCSIMetricsManagerWithOptions(csiAttacher, metrics.WithMigration())
		migratedCsiClient, err := connection.Connect(*csiAddress, metricsManager, connection.OnConnectionLoss(connection.ExitOnConnectionLoss()))
		if err != nil {
			klog.Error(err.Error())
			return
		}
		csiConn.Close()
		csiConn = migratedCsiClient

		err = rpc.ProbeForever(csiConn, *timeout)
		if err != nil {
			klog.Error(err.Error())
			return
		}
	}

	supportsService, err := supportsPluginControllerService(ctx, csiConn)
	if err != nil {
		klog.Error(err.Error())
		return
	}

	var (
		supportsAttach                    bool
		supportsReadOnly                  bool
		supportsListVolumesPublishedNodes bool
		supportsSingleNodeMultiWriter     bool
	)
	if !supportsService {
		handler = controller.NewTrivialHandler(clientset)
		klog.V(2).Infof("CSI driver does not support Plugin Controller Service, using trivial handler")
	} else {
		supportsAttach, supportsReadOnly, supportsListVolumesPublishedNodes, supportsSingleNodeMultiWriter, err = supportsControllerCapabilities(ctx, csiConn)
		if err != nil {
			klog.Error(err.Error())
			return
		}

		if supportsAttach {
			pvLister := factory.Core().V1().PersistentVolumes().Lister()
			vaLister := factory.Storage().V1().VolumeAttachments().Lister()
			csiNodeLister := factory.Storage().V1().CSINodes().Lister()
			volAttacher := attacher.NewAttacher(csiConn)
			CSIVolumeLister := attacher.NewVolumeLister(csiConn)
			handler = controller.NewCSIHandler(
				clientset,
				csiAttacher,
				volAttacher,
				CSIVolumeLister,
				pvLister,
				csiNodeLister,
				vaLister,
				timeout,
				supportsReadOnly,
				supportsSingleNodeMultiWriter,
				csitrans.New(),
				*defaultFSType,
			)
			klog.V(2).Infof("CSI driver supports ControllerPublishUnpublish, using real CSI handler")
		} else {
			handler = controller.NewTrivialHandler(clientset)
			klog.V(2).Infof("CSI driver does not support ControllerPublishUnpublish, using trivial handler")
		}
	}

	if supportsListVolumesPublishedNodes {
		klog.V(2).Infof("CSI driver supports list volumes published nodes. Using capability to reconcile volume attachment objects with actual backend state")
	}

	ctrl := controller.NewCSIAttachController(
		clientset,
		csiAttacher,
		handler,
		factory.Storage().V1().VolumeAttachments(),
		factory.Core().V1().PersistentVolumes(),
		workqueue.NewItemExponentialFailureRateLimiter(*retryIntervalStart, *retryIntervalMax),
		workqueue.NewItemExponentialFailureRateLimiter(*retryIntervalStart, *retryIntervalMax),
		supportsListVolumesPublishedNodes,
		*reconcileSync,
	)

	run := func(ctx context.Context) {
		stopCh := ctx.Done()
		factory.Start(stopCh)
		ctrl.Run(int(*workerThreads), stopCh)
	}

	if !*enableLeaderElection {
		run(context.TODO())
	} else {
		// Create a new clientset for leader election. When the attacher
		// gets busy and its client gets throttled, the leader election
		// can proceed without issues.
		leClientset, err := kubernetes.NewForConfig(config)
		if err != nil {
			klog.Fatalf("Failed to create leaderelection client: %v", err)
		}

		// Name of config map with leader election lock
		lockName := "external-attacher-leader-" + csiAttacher
		le := leaderelection.NewLeaderElection(leClientset, lockName, run)

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

func supportsControllerCapabilities(ctx context.Context, csiConn *grpc.ClientConn) (bool, bool, bool, bool, error) {
	caps, err := rpc.GetControllerCapabilities(ctx, csiConn)
	if err != nil {
		return false, false, false, false, err
	}

	supportsControllerPublish := caps[csi.ControllerServiceCapability_RPC_PUBLISH_UNPUBLISH_VOLUME]
	supportsPublishReadOnly := caps[csi.ControllerServiceCapability_RPC_PUBLISH_READONLY]
	supportsListVolumesPublishedNodes := caps[csi.ControllerServiceCapability_RPC_LIST_VOLUMES] && caps[csi.ControllerServiceCapability_RPC_LIST_VOLUMES_PUBLISHED_NODES]
	supportsSingleNodeMultiWriter := caps[csi.ControllerServiceCapability_RPC_SINGLE_NODE_MULTI_WRITER]
	return supportsControllerPublish, supportsPublishReadOnly, supportsListVolumesPublishedNodes, supportsSingleNodeMultiWriter, nil
}

func supportsPluginControllerService(ctx context.Context, csiConn *grpc.ClientConn) (bool, error) {
	caps, err := rpc.GetPluginCapabilities(ctx, csiConn)
	if err != nil {
		return false, err
	}

	return caps[csi.PluginCapability_Service_CONTROLLER_SERVICE], nil
}
