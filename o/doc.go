package main

import (
	"context"
	"fmt"
	"github.com/container-storage-interface/spec/lib/go/csi"
	volumehelpers "k8s.io/cloud-provider/volume/helpers"
)

type ControllerServer interface {
	CreateVolume(context.Context, *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error)                                           // provisioner
	DeleteVolume(context.Context, *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error)                                           // provisioner、kubelet
	ControllerPublishVolume(context.Context, *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error)          // attacher
	ControllerUnpublishVolume(context.Context, *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error)    // attacher
	ValidateVolumeCapabilities(context.Context, *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) //
	ListVolumes(context.Context, *csi.ListVolumesRequest) (*csi.ListVolumesResponse, error)                                              // attacher、health
	GetCapacity(context.Context, *csi.GetCapacityRequest) (*csi.GetCapacityResponse, error)                                              // provisioner、kubelet
	ControllerGetCapabilities(context.Context, *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error)    // snapshotter、kubelet
	CreateSnapshot(context.Context, *csi.CreateSnapshotRequest) (*csi.CreateSnapshotResponse, error)                                     // snapshotter
	DeleteSnapshot(context.Context, *csi.DeleteSnapshotRequest) (*csi.DeleteSnapshotResponse, error)                                     // snapshotter
	ListSnapshots(context.Context, *csi.ListSnapshotsRequest) (*csi.ListSnapshotsResponse, error)                                        // snapshotter
	ControllerExpandVolume(context.Context, *csi.ControllerExpandVolumeRequest) (*csi.ControllerExpandVolumeResponse, error)             // resizer
	ControllerGetVolume(context.Context, *csi.ControllerGetVolumeRequest) (*csi.ControllerGetVolumeResponse, error)                      // health
	ControllerModifyVolume(context.Context, *csi.ControllerModifyVolumeRequest) (*csi.ControllerModifyVolumeResponse, error)             // resizer
}

type GroupControllerServer interface {
	GroupControllerGetCapabilities(context.Context, *csi.GroupControllerGetCapabilitiesRequest) (*csi.GroupControllerGetCapabilitiesResponse, error) // csi-test
	CreateVolumeGroupSnapshot(context.Context, *csi.CreateVolumeGroupSnapshotRequest) (*csi.CreateVolumeGroupSnapshotResponse, error)                // snapshotter
	DeleteVolumeGroupSnapshot(context.Context, *csi.DeleteVolumeGroupSnapshotRequest) (*csi.DeleteVolumeGroupSnapshotResponse, error)                // snapshotter
	GetVolumeGroupSnapshot(context.Context, *csi.GetVolumeGroupSnapshotRequest) (*csi.GetVolumeGroupSnapshotResponse, error)                         // snapshotter
}

type NodeServer interface {
	NodeStageVolume(context.Context, *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error)             // kubelet
	NodeUnstageVolume(context.Context, *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error)       // kubelet
	NodePublishVolume(context.Context, *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error)       // kubelet
	NodeUnpublishVolume(context.Context, *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) // kubelet
	NodeGetVolumeStats(context.Context, *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error)    // kubelet
	NodeExpandVolume(context.Context, *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error)          // kubelet
	NodeGetCapabilities(context.Context, *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) // kubelet
	NodeGetInfo(context.Context, *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error)                         // kubelet
}

type IdentityServer interface {
	GetPluginInfo(context.Context, *csi.GetPluginInfoRequest) (*csi.GetPluginInfoResponse, error)                         // health-monitor、resizer、attacher、provisioner、snapshotter、registrar
	GetPluginCapabilities(context.Context, *csi.GetPluginCapabilitiesRequest) (*csi.GetPluginCapabilitiesResponse, error) // health-monitor、resizer、attacher、provisioner
	Probe(context.Context, *csi.ProbeRequest) (*csi.ProbeResponse, error)                                                 // provisioner、kubelet
}

func main() {
	set, _ := volumehelpers.LabelZonesToSet("zone_a__zone_b")
	fmt.Println(set.List())
	fmt.Println(volumehelpers.ZonesSetToLabelValue(set))
}
