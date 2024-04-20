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

package csi_node_driver_registrar

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"google.golang.org/grpc"

	klog "k8s.io/klog/v2"
	registerapi "k8s.io/kubelet/pkg/apis/pluginregistration/v1"
	"local/3rd/over-node-driver-registrar/pkg/util"
)

func nodeRegister(csiDriverName string) {
	// When kubeletRegistrationPath is specified then driver-registrar ONLY acts
	// as gRPC server which replies to registration requests initiated by kubelet's
	// plugins watcher infrastructure. Node labeling is done by kubelet's csi code.
	registrar := newRegistrationServer(csiDriverName, *kubeletRegistrationPath, supportedVersions)
	socketPath := buildSocketPath(csiDriverName) // /registration/hostpath.csi.k8s.io-reg.sock
	if err := util.CleanupSocketFile(socketPath); err != nil {
		klog.Errorf("%+v", err)
		return
	}

	var oldmask int
	if runtime.GOOS == "linux" {
		// Default to only user accessible socket, caller can open up later if desired
		oldmask, _ = util.Umask(0077)
	}

	klog.Infof("Starting Registration Server at: %s\n", socketPath)
	lis, err := net.Listen("unix", socketPath)
	if err != nil {
		klog.Errorf("failed to listen on socket: %s with error: %+v", socketPath, err)
		return
	}
	if runtime.GOOS == "linux" {
		util.Umask(oldmask)
	}
	klog.Infof("Registration Server started at: %s\n", socketPath)
	grpcServer := grpc.NewServer()

	// Before registering node-driver-registrar with the kubelet ensure that the lockfile doesn't exist
	// a lockfile may exist because the container was forcefully shutdown
	util.CleanupFile(registrationProbePath)

	// Registers kubelet plugin watcher api.
	registerapi.RegisterRegistrationServer(grpcServer, registrar)

	go httpServer(socketPath)
	go removeRegSocket(csiDriverName)
	// Starts service
	if err := grpcServer.Serve(lis); err != nil {
		klog.Errorf("Registration Server stopped serving: %v", err)
		return
	}

	// clean the file on graceful shutdown
	util.CleanupFile(registrationProbePath)
	// If gRPC server is gracefully shutdown, cleanup and exit
	os.Exit(0)
}

func buildSocketPath(csiDriverName string) string {
	return fmt.Sprintf("%s/%s-reg.sock", *pluginRegistrationPath, csiDriverName)
}

func httpServer(socketPath string) {

	// Prepare http endpoint for healthz + profiling (if enabled)
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, req *http.Request) {
		socketExists, err := util.DoesSocketExist(socketPath)
		if err == nil && socketExists {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`ok`))
			klog.V(5).Infof("health check succeeded")
		} else if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
			klog.Errorf("health check failed: %+v", err)
		} else if !socketExists {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte("registration socket does not exist"))
			klog.Errorf("health check failed, registration socket does not exist")
		}
	})

}

func removeRegSocket(csiDriverName string) {
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGTERM)
	<-sigc
	socketPath := buildSocketPath(csiDriverName)
	err := os.Remove(socketPath)
	if err != nil && !os.IsNotExist(err) {
		klog.Errorf("failed to remove socket: %s with error: %+v", socketPath, err)
		return
	}
	os.Exit(0)
}
