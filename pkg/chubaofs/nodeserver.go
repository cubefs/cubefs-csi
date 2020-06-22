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
	csicommon "github.com/chubaofs/chubaofs-csi/pkg/csi-common"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/glog"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"sync"
	"time"
)

type nodeServer struct {
	*csicommon.DefaultNodeServer
	mutex sync.RWMutex
}

func (ns *nodeServer) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	ns.mutex.Lock()
	defer ns.mutex.Unlock()
	stagingTargetPath := req.GetStagingTargetPath()
	targetPath := req.GetTargetPath()
	if err := createMountPoint(targetPath); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	hasMount, err := isMountPoint(targetPath)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "check isMountPoint[%v] error: %v", targetPath, err)
	}

	if hasMount {
		return &csi.NodePublishVolumeResponse{}, nil
	}

	err = bindMount(stagingTargetPath, targetPath)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "mount bind fail. stagingTargetPath[%v], targetPath[%v] error:%v",
			stagingTargetPath, targetPath, err)
	}

	return &csi.NodePublishVolumeResponse{}, nil
}

func (ns *nodeServer) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	ns.mutex.Lock()
	defer ns.mutex.Unlock()
	targetPath := req.GetTargetPath()
	hasMount, err := isMountPoint(targetPath)
	if err != nil || !hasMount {
		glog.Warningf("targetPath is already not a MountPoint, path:%v error:%v", targetPath, err)
		return &csi.NodeUnpublishVolumeResponse{}, nil
	}

	err = umountVolume(targetPath)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "umount targetPath[%v] fail, error: %v", targetPath, err)
	}

	if err = CleanPath(targetPath); err != nil {
		glog.Warningf("remove targetPath: %v with error: %v", targetPath, err)
	}

	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (ns *nodeServer) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	ns.mutex.Lock()
	defer ns.mutex.Unlock()
	start := time.Now()
	stagingTargetPath := req.GetStagingTargetPath()
	err := createMountPoint(stagingTargetPath)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "createMountPoint[%v] fail, error: %v", stagingTargetPath, err)
	}

	hasMount, err := isMountPoint(stagingTargetPath)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "check isMountPoint[%v] error: %v", stagingTargetPath, err)
	}

	if hasMount {
		return &csi.NodeStageVolumeResponse{}, nil
	}

	volumeName := req.GetVolumeId()
	param := req.VolumeContext
	cfsServer, err := newCfsServer(volumeName, param)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "new cfs server error, %v", err)
	}

	err = cfsServer.persistClientConf(stagingTargetPath)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "persist client config file fail, error: %v", err)
	}

	if err = cfsServer.runClient(); err != nil {
		return nil, status.Errorf(codes.Internal, "mount fail, error: %v", err)
	}

	duration := time.Since(start)
	glog.Infof("NodeStageVolume stagingTargetPath:%v cost time:%v", stagingTargetPath, duration)

	return &csi.NodeStageVolumeResponse{}, nil
}

func (ns *nodeServer) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	ns.mutex.Lock()
	defer ns.mutex.Unlock()
	stagingTargetPath := req.GetStagingTargetPath()
	hasMount, err := isMountPoint(stagingTargetPath)
	if err != nil || !hasMount {
		glog.Warningf("stagingTargetPath is already not a MountPoint, path:%v, error: %v", stagingTargetPath, err)
		return &csi.NodeUnstageVolumeResponse{}, nil
	}

	err = umountVolume(stagingTargetPath)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "umount stagingTargetPath[%v] fail, error: %v", stagingTargetPath, err)
	}

	if err = CleanPath(stagingTargetPath); err != nil {
		glog.Warningf("remove stagingTargetPath: %v with error: %v", stagingTargetPath, err)
	}

	return &csi.NodeUnstageVolumeResponse{}, nil
}

func (ns *nodeServer) NodeGetInfo(ctx context.Context, req *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	monitor := NewMountPointerMonitor(&ns.mutex)
	go monitor.checkInvalidMountPointPeriod()
	return &csi.NodeGetInfoResponse{
		NodeId: ns.Driver.NodeID,
	}, nil
}

func (ns *nodeServer) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	return &csi.NodeGetCapabilitiesResponse{
		Capabilities: []*csi.NodeServiceCapability{
			{
				Type: &csi.NodeServiceCapability_Rpc{
					Rpc: &csi.NodeServiceCapability_RPC{
						Type: csi.NodeServiceCapability_RPC_STAGE_UNSTAGE_VOLUME,
					},
				},
			},
		},
	}, nil
}