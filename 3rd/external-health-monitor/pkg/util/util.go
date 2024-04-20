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

package util

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"

	v1 "k8s.io/api/core/v1"
)

const (
	// CSI Plugin Name
	CSIPluginName                     = "kubernetes.io/csi"
	DefaultKubeletPluginsDirName      = "plugins"
	persistentVolumeInGlobalPath      = "pv"
	globalMountInGlobalPath           = "globalmount"
	DefaultKubeletPodsDirName         = "pods"
	DefaultKubeletVolumesDirName      = "volumes"
	DefaultKubeletBlockVolumesDirName = "volumeDevices"
	DefaultEventIndexerName           = "event-uid"
	DefaultRecoveryEventMessage       = "The Volume returns to the healthy state"
)

// MakeDeviceMountPath generates device mount path
func MakeDeviceMountPath(kubeletRootDir string, pv *v1.PersistentVolume) (string, error) {
	if pv.Name == "" {
		return "", fmt.Errorf("makeDeviceMountPath failed, pv name empty")
	}

	pluginsDir := path.Join(kubeletRootDir, DefaultKubeletPluginsDirName)
	csiPluginDir := path.Join(pluginsDir, CSIPluginName)

	return path.Join(csiPluginDir, persistentVolumeInGlobalPath, pv.Name, globalMountInGlobalPath), nil
}

// GetVolumePath generates volume path
func GetVolumePath(kubeletRootDir, pvName, podUID string, isBlock bool) string {
	volID := EscapeQualifiedName(pvName)

	podsDir := path.Join(kubeletRootDir, DefaultKubeletPodsDirName)
	podDir := path.Join(podsDir, podUID)
	podVolumesDir := path.Join(podDir, DefaultKubeletVolumesDirName)
	if isBlock {
		podVolumesDir = path.Join(podDir, DefaultKubeletBlockVolumesDirName)
	}
	podVolumeDir := filepath.Join(podVolumesDir, EscapeQualifiedName(CSIPluginName), volID)
	if isBlock {
		return podVolumeDir
	}
	return path.Join(podVolumeDir, "/mount")
}

// EscapeQualifiedName replace "/" with "~"
func EscapeQualifiedName(in string) string {
	return strings.Replace(in, "/", "~", -1)
}
