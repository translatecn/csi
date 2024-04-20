/*
Copyright 2020 The Kubernetes Authors.

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

package csi_external_health_monitor_controller

import (
	"context"
	"flag"
	"fmt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"net/http"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/record"
	"k8s.io/component-base/featuregate"
	logsapi "k8s.io/component-base/logs/api/v1"
	_ "k8s.io/component-base/logs/json/register"
	"k8s.io/klog/v2"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/kubernetes-csi/csi-lib-utils/connection"
	"github.com/kubernetes-csi/csi-lib-utils/leaderelection"
	"github.com/kubernetes-csi/csi-lib-utils/metrics"
	"github.com/kubernetes-csi/csi-lib-utils/rpc"
	"google.golang.org/grpc"

	monitorcontroller "local/3rd/external-health-monitor/pkg/controller"
)

const (

	// Default timeout of short CSI calls like GetPluginInfo
	csiTimeout = time.Second
)

// Command line flags
var (
	monitorInterval             = pointer.Duration(1 * time.Minute)
	kubeconfig                  = pointer.String("")
	resync                      = pointer.Duration(10 * time.Minute)
	csiAddress                  = pointer.String("/csi/csi.sock")
	showVersion                 = pointer.Bool(false)
	timeout                     = pointer.Duration(15 * time.Second)
	listVolumesInterval         = pointer.Duration(5 * time.Minute)
	volumeListAndAddInterval    = pointer.Duration(5 * time.Minute)
	nodeListAndAddInterval      = pointer.Duration(5 * time.Minute)
	workerThreads               = pointer.Uint(10)
	enableNodeWatcher           = pointer.Bool(false)
	enableLeaderElection        = pointer.Bool(true)
	leaderElectionNamespace     = pointer.String("")
	leaderElectionLeaseDuration = pointer.Duration(1500 * time.Second)
	leaderElectionRenewDeadline = pointer.Duration(1000 * time.Second)
	leaderElectionRetryPeriod   = pointer.Duration(5 * time.Second)
	metricsAddress              = pointer.String("")
	httpEndpoint                = pointer.String("")
	metricsPath                 = pointer.String("/metrics")
)

var (
	version = "unknown"
	fg      = featuregate.NewFeatureGate()
	c       = logsapi.NewLoggingConfiguration()
)

func init() {
	logsapi.AddFeatureGates(fg)
	logsapi.AddGoFlags(c, flag.CommandLine)
}

func Main() {

	if *metricsAddress != "" && *httpEndpoint != "" {
		klog.ErrorS(nil, "Only one of `--metrics-address` and `--http-endpoint` can be set.")
		klog.FlushAndExit(klog.ExitFlushTimeout, 1)
	}
	addr := *metricsAddress
	if addr == "" {
		addr = *httpEndpoint
	}

	// Create the client config. Use kubeconfig if given, otherwise assume in-cluster.
	config, err := buildConfig(*kubeconfig)
	if err != nil {
		klog.ErrorS(err, "Failed to build a Kubernetes config")
		klog.FlushAndExit(klog.ExitFlushTimeout, 1)
	}

	if *workerThreads == 0 {
		klog.ErrorS(nil, "Option --worker-threads must be greater than zero")
		klog.FlushAndExit(klog.ExitFlushTimeout, 1)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		klog.ErrorS(err, "Failed to create a Clientset")
		klog.FlushAndExit(klog.ExitFlushTimeout, 1)
	}

	factory := informers.NewSharedInformerFactory(clientset, *resync)

	metricsManager := metrics.NewCSIMetricsManager("" /* driverName */)

	// Connect to CSI.
	csiConn, err := connection.Connect(*csiAddress, metricsManager, connection.OnConnectionLoss(connection.ExitOnConnectionLoss()))
	if err != nil {
		klog.ErrorS(err, "Failed to connect to the CSI driver")
		klog.FlushAndExit(klog.ExitFlushTimeout, 1)
	}

	err = rpc.ProbeForever(csiConn, *timeout)
	if err != nil {
		klog.ErrorS(err, "Failed to probe the CSI driver")
		klog.FlushAndExit(klog.ExitFlushTimeout, 1)
	}

	// Find driver name.
	ctx, cancel := context.WithTimeout(context.Background(), csiTimeout)
	defer cancel()
	storageDriver, err := rpc.GetDriverName(ctx, csiConn)
	if err != nil {
		klog.ErrorS(err, "Failed to get the CSI driver name")
		klog.FlushAndExit(klog.ExitFlushTimeout, 1)
	}
	klog.V(2).InfoS("CSI driver name", "driver", storageDriver)
	metricsManager.SetDriverName(storageDriver)

	// Prepare HTTP endpoint for metrics + leader election healthz
	mux := http.NewServeMux()
	if addr != "" {
		metricsManager.RegisterToServer(mux, *metricsPath)
		go func() {
			klog.InfoS("ServeMux listening", "address", addr)
			err := http.ListenAndServe(addr, mux)
			if err != nil {
				klog.ErrorS(err, "Failed to start HTTP server at specified address and metrics path", "address", addr, "path", *metricsPath)
				klog.FlushAndExit(klog.ExitFlushTimeout, 1)
			}
		}()
	}

	supportsService, err := supportsPluginControllerService(ctx, csiConn)
	if err != nil {
		klog.ErrorS(err, "Failed to check whether the CSI driver supports the Plugin Controller Service")
		klog.FlushAndExit(klog.ExitFlushTimeout, 1)
	}
	if !supportsService {
		klog.V(2).InfoS("CSI driver does not support Plugin Controller Service, exiting")
		klog.FlushAndExit(klog.ExitFlushTimeout, 1)
	}

	supportControllerListVolumes, err := supportControllerListVolumes(ctx, csiConn)
	if err != nil {
		klog.ErrorS(err, "Failed to check whether the CSI driver supports the Controller Service ListVolumes")
		return
	}

	supportControllerGetVolume, err := supportControllerGetVolume(ctx, csiConn)
	if err != nil {
		klog.ErrorS(err, "Failed to check whether the CSI driver supports the Controller Service GetVolume")
		return
	}

	supportControllerVolumeCondition, err := supportControllerVolumeCondition(ctx, csiConn)
	if err != nil {
		klog.ErrorS(err, "Failed to check whether the CSI driver supports the Controller Service VolumeCondition")
		return
	}

	if (!supportControllerListVolumes && !supportControllerGetVolume) || !supportControllerVolumeCondition {
		klog.V(2).InfoS("CSI driver does not support Controller ListVolumes and GetVolume service or does not implement VolumeCondition, exiting")
		return
	}

	option := monitorcontroller.PVMonitorOptions{
		DriverName:        storageDriver,
		ContextTimeout:    *timeout,
		EnableNodeWatcher: *enableNodeWatcher,
		SupportListVolume: supportControllerListVolumes,

		ListVolumesInterval:      *listVolumesInterval,
		PVWorkerExecuteInterval:  *monitorInterval,
		VolumeListAndAddInterval: *volumeListAndAddInterval,

		NodeWorkerExecuteInterval: *monitorInterval,
		NodeListAndAddInterval:    *nodeListAndAddInterval,
	}

	broadcaster := record.NewBroadcaster()
	broadcaster.StartRecordingToSink(&corev1.EventSinkImpl{Interface: clientset.CoreV1().Events(v1.NamespaceAll)})
	eventRecorder := broadcaster.NewRecorder(scheme.Scheme, v1.EventSource{Component: fmt.Sprintf("csi-pv-monitor-controller-%s", option.DriverName)})

	monitorController := monitorcontroller.NewPVMonitorController(clientset, csiConn, factory.Core().V1().PersistentVolumes(),
		factory.Core().V1().PersistentVolumeClaims(), factory.Core().V1().Pods(), factory.Core().V1().Nodes(), factory.Core().V1().Events(), eventRecorder, &option)

	run := func(ctx context.Context) {
		stopCh := ctx.Done()
		factory.Start(stopCh)
		monitorController.Run(int(*workerThreads), stopCh)
	}

	if !*enableLeaderElection {
		run(context.TODO())
	} else {
		// Name of config map with leader election lock
		lockName := "external-health-monitor-leader-" + storageDriver
		clientset.CoordinationV1().Leases(*leaderElectionNamespace).Delete(context.TODO(), lockName, metav1.DeleteOptions{})
		le := leaderelection.NewLeaderElection(clientset, lockName, run)
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
			klog.ErrorS(err, "Failed to initialize leader election")
			return
		}
	}

}

func buildConfig(kubeconfig string) (*rest.Config, error) {
	if kubeconfig != "" {
		return clientcmd.BuildConfigFromFlags("", kubeconfig)
	}
	return rest.InClusterConfig()
}

func supportControllerListVolumes(ctx context.Context, csiConn *grpc.ClientConn) (supportControllerListVolumes bool, err error) {
	caps, err := rpc.GetControllerCapabilities(ctx, csiConn)
	if err != nil {
		return false, fmt.Errorf("failed to get controller capabilities: %v", err)
	}

	return caps[csi.ControllerServiceCapability_RPC_LIST_VOLUMES], nil
}

// TODO: move this to csi-lib-utils
func supportControllerGetVolume(ctx context.Context, csiConn *grpc.ClientConn) (supportControllerGetVolume bool, err error) {
	client := csi.NewControllerClient(csiConn)
	req := csi.ControllerGetCapabilitiesRequest{}
	rsp, err := client.ControllerGetCapabilities(ctx, &req)
	if err != nil {
		return false, err
	}

	for _, cap := range rsp.GetCapabilities() {
		if cap == nil {
			continue
		}
		rpc := cap.GetRpc()
		if rpc == nil {
			continue
		}
		t := rpc.GetType()
		if t == csi.ControllerServiceCapability_RPC_GET_VOLUME {
			return true, nil
		}
	}

	return false, nil
}

// TODO: move this to csi-lib-utils
func supportControllerVolumeCondition(ctx context.Context, csiConn *grpc.ClientConn) (supportControllerVolumeCondition bool, err error) {
	client := csi.NewControllerClient(csiConn)
	req := csi.ControllerGetCapabilitiesRequest{}
	rsp, err := client.ControllerGetCapabilities(ctx, &req)
	if err != nil {
		return false, err
	}

	for _, cap := range rsp.GetCapabilities() {
		if cap == nil {
			continue
		}

		rpc := cap.GetRpc()
		if rpc == nil {
			continue
		}
		t := rpc.GetType()
		if t == csi.ControllerServiceCapability_RPC_VOLUME_CONDITION {
			return true, nil
		}
	}

	return false, nil
}

// TODO: move this to csi-lib-utils
func supportsPluginControllerService(ctx context.Context, csiConn *grpc.ClientConn) (bool, error) {
	client := csi.NewIdentityClient(csiConn)
	req := csi.GetPluginCapabilitiesRequest{}
	rsp, err := client.GetPluginCapabilities(ctx, &req)
	if err != nil {
		return false, err
	}
	for _, cap := range rsp.GetCapabilities() {
		if cap == nil {
			continue
		}
		srv := cap.GetService()
		if srv == nil {
			continue
		}
		t := srv.GetType()
		if t == csi.PluginCapability_Service_CONTROLLER_SERVICE {
			return true, nil
		}
	}

	return false, nil
}
