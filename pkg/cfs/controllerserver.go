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

package cfs

import (
	"fmt"
	"github.com/chubaofs/chubaofs-csi/pkg/csi-common"
	"github.com/container-storage-interface/spec/lib/go/csi/v0"
	"github.com/golang/glog"
	"golang.org/x/net/context"
	"k8s.io/kubernetes/pkg/volume/util"
	"sync"
)

const (
	KEY_VOLUME_NAME          = "volName"
	KEY_CFS_MASTER1          = "cfsMaster1"
	KEY_CFS_MASTER2          = "cfsMaster2"
	KEY_CFS_MASTER3          = "cfsMaster3"
	CFS_FUSE_CONFIG_PATH     = "/etc/cfs/fuse.json"
	FUSE_KEY_LOG_PATH_V1     = "logpath"
	FUSE_KEY_LOG_PATH_V2     = "logDir"
	FUSE_KEY_MASTER_ADDR_V1  = "master"
	FUSE_KEY_MASTER_ADDR_V2  = "masterAddr"
	FUSE_KEY_MOUNT_POINT_V1  = "mountpoint"
	FUSE_KEY_MOUNT_POINT_V2  = "mountPoint"
	FUSE_KEY_VOLUME_NAME_V1  = "volname"
	FUSE_KEY_VOLUME_NAME_V2  = "volName"
	FUSE_KEY_PROF_PORT_V1    = "profport"
	FUSE_KEY_PROF_PORT_V2    = "profPort"
	FUSE_KEY_LOG_LEVEL_V1    = "loglvl"
	FUSE_KEY_LOG_LEVEL_V2    = "logLevel"
	FUSE_KEY_LOOKUP_VALID_V1 = "lookupValid"
	FUSE_KEY_OWNER_V1        = "owner"

	ALLOCATE_MIN_VOL_SIZE_BYTE = 1024 * 1024 * 1024
	ALLOCATE_CFS_VOL_SIZE_UNIT = 1024 * 1024 * 1024
)

type controllerServer struct {
	*csicommon.DefaultControllerServer

	cfsMasterHostsLock sync.RWMutex
	cfsMasterHosts     map[string][]string
}

func (cs *controllerServer) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	glog.V(2).Infof("--1-------CreateVolume req:%v", req)
	if err := cs.Driver.ValidateControllerServiceRequest(csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME); err != nil {
		glog.Errorf("invalid create cfs volume req: %v", req)
		return nil, err
	}

	// Volume Size - Default is 1 GiB
	var volSizeBytes int64 = ALLOCATE_MIN_VOL_SIZE_BYTE
	if req.GetCapacityRange() != nil {
		required := int64(req.GetCapacityRange().GetRequiredBytes())
		glog.V(4).Infof("GetRequiredBytes:%v", volSizeBytes)
		if required > ALLOCATE_MIN_VOL_SIZE_BYTE {
			volSizeBytes = required
		}
	}
	cfsVolSizeGB := int(util.RoundUpSize(volSizeBytes, ALLOCATE_CFS_VOL_SIZE_UNIT))

	volName := req.GetParameters()[KEY_VOLUME_NAME]
	cfsMasterHost1 := req.GetParameters()[KEY_CFS_MASTER1]
	cfsMasterHost2 := req.GetParameters()[KEY_CFS_MASTER2]
	cfsMasterHost3 := req.GetParameters()[KEY_CFS_MASTER3]
	cs.putMasterHosts(volName, cfsMasterHost1, cfsMasterHost2, cfsMasterHost3)
	glog.V(4).Infof("GetName:%v", req.GetName())
	glog.V(4).Infof("GetParameters:%v", req.GetParameters())
	glog.V(4).Infof("allocate volume size(GB):%v for name:%v", cfsVolSizeGB, volName)

	cfsMasterLeader, err := GetClusterInfo(cfsMasterHost1)
	if err != nil {
		return nil, err
	}
	glog.V(4).Infof("CFS Master Leader Host is:%v", cfsMasterLeader)

	if err := CreateVolume(cfsMasterLeader, volName, cfsVolSizeGB); err != nil {
		return nil, err
	}
	glog.V(2).Infof("CFS Create Volume:%v success.", volName)

	resp := &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			Id:            volName,
			CapacityBytes: volSizeBytes,
			Attributes: map[string]string{
				KEY_VOLUME_NAME: volName,
				KEY_CFS_MASTER1: cfsMasterHost1,
				KEY_CFS_MASTER2: cfsMasterHost2,
				KEY_CFS_MASTER3: cfsMasterHost3,
			},
		},
	}
	return resp, nil
}

func (cs *controllerServer) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	glog.V(2).Infof("----------DeleteVolume req:%v", req)
	if err := cs.Driver.ValidateControllerServiceRequest(csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME); err != nil {
		glog.Errorf("invalid delete volume req: %v", req)
		return nil, err
	}
	volumeId := req.VolumeId

	cfsMasterHosts := cs.getMasterHosts(volumeId)
	if len(cfsMasterHosts) == 0 {
		glog.Errorf("Not Found CFS master hosts for volumeId:%v", volumeId)
		return nil, fmt.Errorf("no master hosts")
	}

	GetClusterInfo(cfsMasterHosts[0])
	cfsMasterLeader, err := GetClusterInfo(cfsMasterHosts[0])
	if err != nil {
		return nil, err
	}
	glog.V(4).Infof("CFS Master Leader Host is:%v", cfsMasterLeader)

	if err := DeleteVolume(cfsMasterLeader, volumeId); err != nil {
		return nil, err
	}
	glog.V(2).Infof("Delete cfs volume :%s deleted success", volumeId)

	return &csi.DeleteVolumeResponse{}, nil
}

func (cs *controllerServer) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	for _, cap := range req.VolumeCapabilities {
		if cap.GetAccessMode().GetMode() != csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER {
			return &csi.ValidateVolumeCapabilitiesResponse{Supported: false, Message: ""}, nil
		}
	}
	return &csi.ValidateVolumeCapabilitiesResponse{Supported: true, Message: ""}, nil
}

func (cs *controllerServer) putMasterHosts(volumeName string, hosts ...string) {
	cs.cfsMasterHostsLock.Lock()
	defer cs.cfsMasterHostsLock.Unlock()
	cs.cfsMasterHosts[volumeName] = hosts
}

func (cs *controllerServer) getMasterHosts(volumeName string) []string {
	cs.cfsMasterHostsLock.Lock()
	defer cs.cfsMasterHostsLock.Unlock()
	hosts, found := cs.cfsMasterHosts[volumeName]
	if found {
		return hosts
	}
	return nil
}
