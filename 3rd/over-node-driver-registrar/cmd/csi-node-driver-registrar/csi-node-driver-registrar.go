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

package csi_node_driver_registrar

import (
	"context"
	"fmt"
	klog "k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	_ "net/http/pprof"
	"path/filepath"
	"time"

	"github.com/kubernetes-csi/csi-lib-utils/metrics"
	"local/3rd/over-node-driver-registrar/pkg/util"

	"github.com/kubernetes-csi/csi-lib-utils/connection"
	csirpc "github.com/kubernetes-csi/csi-lib-utils/rpc"
	registerapi "k8s.io/kubelet/pkg/apis/pluginregistration/v1"
)

const (
	// ModeRegistration runs node-driver-registrar as a long running process
	ModeRegistration = "registration"

	// ModeKubeletRegistrationProbe makes node-driver-registrar act as an exec probe
	// that checks if the kubelet plugin registration succeeded.
	ModeKubeletRegistrationProbe = "kubelet-registration-probe"
)

var (
	// The registration probe path, set when the program runs and used as the path of the file
	// to create when the kubelet plugin registration succeeds.
	registrationProbePath = ""
)

// Command line flags
var (
	connectionTimeout       = pointer.Duration(0)
	operationTimeout        = pointer.Duration(time.Second)
	csiAddress              = pointer.String("/csi/csi.sock") // csi 插件的地址
	pluginRegistrationPath  = pointer.String("/registration")
	kubeletRegistrationPath = pointer.String("/var/lib/kubelet/plugins/csi-hostpath/csi.sock") // register 与 kubelet 注册的地址
	mode                    = pointer.String(ModeRegistration)
	supportedVersions       = []string{"1.0.0"}
)

// registrationServer is a sample plugin to work with plugin watcher
type registrationServer struct {
	driverName string
	endpoint   string
	version    []string
}

var _ registerapi.RegistrationServer = registrationServer{}

// newRegistrationServer returns an initialized registrationServer instance
func newRegistrationServer(driverName string, endpoint string, versions []string) registerapi.RegistrationServer {
	return &registrationServer{
		driverName: driverName,
		endpoint:   endpoint,
		version:    versions,
	}
}

// GetInfo is the RPC invoked by plugin watcher
func (e registrationServer) GetInfo(ctx context.Context, req *registerapi.InfoRequest) (*registerapi.PluginInfo, error) {
	klog.Infof("Received GetInfo call: %+v", req)

	// on successful registration, create the registration probe file
	err := util.TouchFile(registrationProbePath)
	if err != nil {
		klog.ErrorS(err, "Failed to create registration probe file", "registrationProbePath", registrationProbePath)
	} else {
		klog.InfoS("Kubelet registration probe created", "path", registrationProbePath)
	}

	return &registerapi.PluginInfo{
		Type:              registerapi.CSIPlugin,
		Name:              e.driverName,
		Endpoint:          e.endpoint,
		SupportedVersions: e.version,
	}, nil
}

func (e registrationServer) NotifyRegistrationStatus(ctx context.Context, status *registerapi.RegistrationStatus) (*registerapi.RegistrationStatusResponse, error) {
	klog.Infof("Received NotifyRegistrationStatus call: %+v", status)
	if !status.PluginRegistered {

		return nil, fmt.Errorf("registration process failed with error: %+v, restarting registration container", status.Error)
	}

	return &registerapi.RegistrationStatusResponse{}, nil
}

func Main() {

	if *kubeletRegistrationPath == "" {
		klog.Error("kubelet-registration-path is a required parameter")
		return
	}
	// set after we made sure that *kubeletRegistrationPath exists
	kubeletRegistrationPathDir := filepath.Dir(*kubeletRegistrationPath)
	registrationProbePath = filepath.Join(kubeletRegistrationPathDir, "registration")

	klog.Infof("Running node-driver-registrar in mode=%s", *mode)

	if *connectionTimeout != 0 {
		klog.Warning("--connection-timeout is deprecated and will have no effect")
	}

	// Unused metrics manager, necessary for connection.Connect below
	cmm := metrics.NewCSIMetricsManagerForSidecar("")

	// Once https://github.com/container-storage-interface/spec/issues/159 is
	// resolved, if plugin does not support PUBLISH_UNPUBLISH_VOLUME, then we
	// can skip adding mapping to "csi.volume.kubernetes.io/nodeid" annotation.

	klog.V(1).Infof("Attempting to open a gRPC connection with: %q", *csiAddress)
	csiConn, err := connection.Connect(*csiAddress, cmm)
	if err != nil {
		klog.Errorf("error connecting to CSI driver: %v", err)
		return
	}

	klog.V(1).Infof("Calling CSI driver to discover driver name")
	ctx, cancel := context.WithTimeout(context.Background(), *operationTimeout)
	defer cancel()

	csiDriverName, err := csirpc.GetDriverName(ctx, csiConn)
	if err != nil {
		klog.Errorf("error retreiving CSI driver name: %v", err)
		return
	}

	klog.V(2).Infof("CSI driver name: %q", csiDriverName)
	cmm.SetDriverName(csiDriverName)

	// Run forever
	nodeRegister(csiDriverName)
}
