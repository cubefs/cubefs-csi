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

package chubaofs

import (
	"github.com/golang/glog"
	"golang.org/x/net/context"
	"k8s.io/kubernetes/pkg/volume/util"
	"strings"
	"sync"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/container-storage-interface/spec/lib/go/csi"
)

const (
	KMountPoint = "mountPoint"
	KVolumeName = "volName"
	KMasterAddr = "masterAddr"
	KLogDir     = "logDir"
	KWarnLogDir = "warnLogDir"
	KLogLevel   = "logLevel"
	KOwner      = "owner"
	KProfPort   = "profPort"
)

const (
	MinVolumeSize = util.GIB
)

const (
	defaultOwner        = "chubaofs"
	defaultClientConfig = "/etc/chubaofs/fuse.json"
)

type controllerServer struct {
	caps []*csi.ControllerServiceCapability

	masterHostsLock sync.RWMutex
	masterHosts     map[string]string
}

func NewControllerServer() *controllerServer {
	return &controllerServer{
		caps: getControllerServiceCapabilities(
			[]csi.ControllerServiceCapability_RPC_Type{
				csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
			}),
		masterHosts: make(map[string]string),
	}
}

func (cs *controllerServer) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	glog.V(2).Infof("CreateVolume req: %v", req)

	if err := cs.validateControllerServiceRequest(csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME); err != nil {
		glog.V(3).Infof("Invalid create volume req: %v", req)
		return nil, err
	}

	if len(req.GetName()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Name missing in request")
	}

	caps := req.GetVolumeCapabilities()
	if caps == nil {
		return nil, status.Error(codes.InvalidArgument, "Volume Capabilities missing in request")
	}

	var mountOptions *csi.VolumeCapability_MountVolume
	for _, cap := range caps {
		if cap.GetMount() != nil {
			mountOptions = cap.GetMount()
		}
	}

	if mountOptions == nil {
		return nil, status.Error(codes.InvalidArgument, "Volume lack of mount access type")
	}

	if strings.Compare(mountOptions.GetFsType(), "chubaofs") != 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume fstype is not chubaofs")
	}

	capacity := int64(req.GetCapacityRange().GetRequiredBytes())
	if capacity < MinVolumeSize {
		capacity = MinVolumeSize
	}
	capacityInGIB, err := util.RoundUpSizeInt(capacity, util.GIB)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	paras := req.GetParameters()

	// TODO: check if parameters are valid
	volumeid := paras[KVolumeName]
	owner := defaultOwner
	masterAddr := strings.ReplaceAll(paras[KMasterAddr], ";", ",")
	master := strings.Split(masterAddr, ",")

	cs.putMasterHosts(volumeid, masterAddr)
	glog.V(4).Infof("GetName:%v", req.GetName())
	glog.V(4).Infof("GetParameters:%v", paras)

	leader, err := getClusterInfo(master[0])
	if err != nil {
		return nil, err
	}

	glog.V(4).Infof("ChubaoFS master leader addr is:%v", leader)

	if err := createOrDeleteVolume(createVolumeRequest, leader, volumeid, owner, int64(capacityInGIB)); err != nil {
		return nil, err
	}

	glog.V(2).Infof("ChubaoFS create volume:%v success.", volumeid)

	resp := &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:      volumeid,
			CapacityBytes: capacity,
			VolumeContext: paras,
		},
	}
	return resp, nil
}

func (cs *controllerServer) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	glog.V(2).Infof("DeleteVolume req: %v", req)

	if err := cs.validateControllerServiceRequest(csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME); err != nil {
		glog.Errorf("invalid delete volume req: %v", req)
		return nil, err
	}
	volumeid := req.VolumeId

	masterAddr := cs.getMasterHosts(volumeid)
	if masterAddr == "" {
		return nil, status.Errorf(codes.InvalidArgument, "chubaofs: cannot find master addr, volumeid(%v)", volumeid)
	}
	master := strings.Split(masterAddr, ",")

	leader, err := getClusterInfo(master[0])
	if err != nil {
		return nil, err
	}

	if err := createOrDeleteVolume(deleteVolumeRequest, leader, volumeid, defaultOwner, 0); err != nil {
		return nil, err
	}

	glog.V(2).Infof("Delete cfs volume :%s deleted success", volumeid)

	return &csi.DeleteVolumeResponse{}, nil
}

func (cs *controllerServer) ControllerGetCapabilities(ctx context.Context, req *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error) {
	return &csi.ControllerGetCapabilitiesResponse{
		Capabilities: cs.caps,
	}, nil
}

func (cs *controllerServer) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	for _, cap := range req.VolumeCapabilities {
		if cap.GetAccessMode().GetMode() != csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER {
			return nil, status.Error(codes.InvalidArgument, "No multi node multi writer capability")
		}
	}

	return &csi.ValidateVolumeCapabilitiesResponse{
		Confirmed: &csi.ValidateVolumeCapabilitiesResponse_Confirmed{
			VolumeContext:      req.GetVolumeContext(),
			VolumeCapabilities: req.GetVolumeCapabilities(),
			Parameters:         req.GetParameters(),
		},
	}, nil
}

func (cs *controllerServer) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (cs *controllerServer) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (cs *controllerServer) GetCapacity(ctx context.Context, req *csi.GetCapacityRequest) (*csi.GetCapacityResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (cs *controllerServer) ListVolumes(ctx context.Context, req *csi.ListVolumesRequest) (*csi.ListVolumesResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (cs *controllerServer) CreateSnapshot(ctx context.Context, req *csi.CreateSnapshotRequest) (*csi.CreateSnapshotResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (cs *controllerServer) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (*csi.DeleteSnapshotResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (cs *controllerServer) ListSnapshots(ctx context.Context, req *csi.ListSnapshotsRequest) (*csi.ListSnapshotsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (cs *controllerServer) putMasterHosts(volName string, hosts string) {
	cs.masterHostsLock.Lock()
	defer cs.masterHostsLock.Unlock()
	cs.masterHosts[volName] = hosts
}

func (cs *controllerServer) getMasterHosts(volName string) string {
	cs.masterHostsLock.Lock()
	defer cs.masterHostsLock.Unlock()
	hosts, found := cs.masterHosts[volName]
	if found {
		return hosts
	}
	return ""
}

func (cs *controllerServer) validateControllerServiceRequest(c csi.ControllerServiceCapability_RPC_Type) error {
	if c == csi.ControllerServiceCapability_RPC_UNKNOWN {
		return nil
	}

	for _, cap := range cs.caps {
		if c == cap.GetRpc().GetType() {
			return nil
		}
	}
	return status.Errorf(codes.InvalidArgument, "unsupported capability %s", c)
}

func getControllerServiceCapabilities(cl []csi.ControllerServiceCapability_RPC_Type) []*csi.ControllerServiceCapability {
	var csc []*csi.ControllerServiceCapability

	for _, cap := range cl {
		glog.Infof("Enabling controller service capability: %v", cap.String())
		csc = append(csc, &csi.ControllerServiceCapability{
			Type: &csi.ControllerServiceCapability_Rpc{
				Rpc: &csi.ControllerServiceCapability_RPC{
					Type: cap,
				},
			},
		})
	}

	return csc
}
