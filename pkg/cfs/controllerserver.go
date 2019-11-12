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
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/kubernetes/pkg/volume/util"
)

const (
	KEY_VOLUME_NAME               = "volName"
	KEY_CFS_MASTER                = "cfsMaster"
	CFS_FUSE_CONFIG_PATH          = "/etc/cfs/"
	FUSE_KEY_LOG_PATH             = "logDir"
	FUSE_KEY_LOG_UMP_WARN_LOG_DIR = "warnLogDir"
	FUSE_KEY_MASTER_ADDR          = "masterAddr"
	FUSE_KEY_MOUNT_POINT          = "mountPoint"
	FUSE_KEY_VOLUME_NAME          = "volName"
	FUSE_KEY_PROF_PORT            = "profPort"
	FUSE_KEY_LOG_LEVEL            = "logLevel"
	FUSE_KEY_LOOKUP_VALID         = "lookupValid"
	FUSE_KEY_OWNER                = "owner"
	FUSE_KEY_ICACHE_TIMEOUT       = "icacheTimeout"
	FUSE_KEY_ATTR_VALID           = "attrValid"
	FUSE_KEY_EN_SYNC_WRITE        = "enSyncWrite"
	FUSE_KEY_AUTO_INVAL_DATA      = "autoInvalData"
	FUSE_KEY_RDONLY               = "rdonly"
	FUSE_KEY_WRITE_CACHE          = "writecache"
	FUSE_KEY_KEEP_CACHE           = "keepcache"

	ALLOCATE_MIN_VOL_SIZE_BYTE = 1024 * 1024 * 1024
	ALLOCATE_CFS_VOL_SIZE_UNIT = 1024 * 1024 * 1024
)

type controllerServer struct {
	*csicommon.DefaultControllerServer
	masterAddress string
}

func (cs *controllerServer) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	glog.V(2).Infof("--1-------CreateVolume req:%v", req)
	if err := cs.Driver.ValidateControllerServiceRequest(csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME); err != nil {
		glog.Errorf("invalid create cfs volume req: %v", req)
		return nil, err
	}
	if len(req.GetName()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Name missing in request")
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

	volName := ""
	volumeName := req.GetParameters()[KEY_VOLUME_NAME]
	if len(volumeName) == 0 {
		volName = req.GetName()
	} else {
		volName = volumeName
	}
	masterAddress := cs.masterAddress
	glog.V(4).Infof("GetName:%v", req.GetName())
	glog.V(4).Infof("GetParameters:%v", req.GetParameters())
	glog.V(4).Infof("allocate volume size(GB):%v for name:%v", cfsVolSizeGB, volName)
	glog.V(4).Infof("CFS Master Leader Host is:%v", masterAddress)

	if err := CreateVolume(masterAddress, volName, cfsVolSizeGB); err != nil {
		return nil, err
	}
	glog.V(2).Infof("CFS Create Volume:%v success.", volName)

	resp := &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			Id:            volName,
			CapacityBytes: volSizeBytes,
			Attributes: map[string]string{
				KEY_VOLUME_NAME: volName,
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

	masterAddress := cs.masterAddress
	if len(masterAddress) == 0 {
		glog.Errorf("Not Found CFS master hosts for volumeId:%v", volumeId)
		return nil, fmt.Errorf("no master hosts")
	}

	glog.V(4).Infof("CFS Master Leader hosts is:%v", masterAddress)

	if err := DeleteVolume(masterAddress, volumeId); err != nil {
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
