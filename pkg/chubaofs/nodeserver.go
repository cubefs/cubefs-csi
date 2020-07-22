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
	"github.com/chubaofs/chubaofs-csi/pkg/csi-common"
	"github.com/container-storage-interface/spec/lib/go/csi/v0"
	"github.com/golang/glog"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/utils/mount"
	"sync"
	"time"
)

type nodeServer struct {
	*csicommon.DefaultNodeServer
	mounter mount.Interface
	mutex   sync.RWMutex
}

func (ns *nodeServer) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	ns.mutex.Lock()
	defer ns.mutex.Unlock()
	start := time.Now()
	stagingTargetPath := req.GetStagingTargetPath()
	targetPath := req.GetTargetPath()

	err := mount.CleanupMountPoint(targetPath, ns.mounter, false)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "CleanupMountPoint fail, targetPath:%v error: %v", targetPath, err)
	}

	err = createMountPoint(targetPath)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "createMountPoint fail, targetPath:%s error: %v", targetPath, err)
	}

	err = bindMount(stagingTargetPath, targetPath)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "mount bind fail. stagingTargetPath:%v, targetPath:%v error:%v",
			stagingTargetPath, targetPath, err)
	}

	duration := time.Since(start)
	glog.Infof("NodePublishVolume mount success, targetPath:%v cost:%v", targetPath, duration)
	return &csi.NodePublishVolumeResponse{}, nil
}

func (ns *nodeServer) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	ns.mutex.Lock()
	defer ns.mutex.Unlock()
	targetPath := req.GetTargetPath()
	err := mount.CleanupMountPoint(targetPath, ns.mounter, false)
	if err != nil {
		return nil, err
	}

	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (ns *nodeServer) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	ns.mutex.Lock()
	defer ns.mutex.Unlock()
	start := time.Now()
	stagingTargetPath := req.GetStagingTargetPath()

	pathExists, pathErr := mount.PathExists(stagingTargetPath)
	corruptedMnt := mount.IsCorruptedMnt(pathErr)
	if pathExists && !corruptedMnt {
		duration := time.Since(start)
		glog.Infof("NodeStageVolume already mount, stagingTargetPath:%v cost:%v", stagingTargetPath, duration)
		return &csi.NodeStageVolumeResponse{}, nil
	}

	err := mount.CleanupMountPoint(stagingTargetPath, ns.mounter, false)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "CleanupMountPoint fail, stagingTargetPath:%v error: %v", stagingTargetPath, err)
	}

	err = createMountPoint(stagingTargetPath)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "createMountPoint fail, stagingTargetPath:%v error: %v", stagingTargetPath, err)
	}

	volumeName := req.GetVolumeId()
	param := req.VolumeAttributes
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
	glog.Infof("NodeStageVolume mount, stagingTargetPath:%v cost:%v", stagingTargetPath, duration)
	return &csi.NodeStageVolumeResponse{}, nil
}

func (ns *nodeServer) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	ns.mutex.Lock()
	defer ns.mutex.Unlock()
	stagingTargetPath := req.GetStagingTargetPath()
	err := mount.CleanupMountPoint(stagingTargetPath, ns.mounter, false)
	if err != nil {
		return nil, err
	}

	return &csi.NodeUnstageVolumeResponse{}, nil
}

func (ns *nodeServer) NodeGetInfo(ctx context.Context, req *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	return &csi.NodeGetInfoResponse{
		NodeId: ns.Driver.NodeID,
	}, nil
}
