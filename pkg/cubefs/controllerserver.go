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

package cubefs

import (
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	csicommon "github.com/cubefs/cubefs-csi/pkg/csi-common"
	"github.com/golang/glog"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type controllerServer struct {
	*csicommon.DefaultControllerServer
	driver *driver
}

func (cs *controllerServer) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	if err := cs.Driver.ValidateControllerServiceRequest(csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME); err != nil {
		return nil, err
	}

	// https://github.com/kubernetes-csi/external-provisioner/blob/master/pkg/controller/controller.go
	for _, cap := range req.VolumeCapabilities {
		if cap.GetBlock() != nil {
			return nil, status.Error(codes.InvalidArgument, "block volume is not supported")
		}
	}

	start := time.Now()
	// Volume Size - Default is 1 GiB
	capacity := req.GetCapacityRange().GetRequiredBytes()
	capacityGB := capacity >> 30
	if capacityGB == 0 {
		return nil, status.Error(codes.InvalidArgument, "apply for at least 1GB of space")
	}

	volName := req.GetName()
	cfsServer, err := newCfsServer(volName, req.GetParameters())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	if err := cfsServer.createVolume(capacityGB); err != nil {
		return nil, err
	}

	duration := time.Since(start)
	glog.V(0).Infof("create volume[%v] success. cost time:%v", volName, duration)
	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:      volName,
			CapacityBytes: capacity,
			VolumeContext: cfsServer.clientConf,
		},
	}, nil
}

func (cs *controllerServer) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	if err := cs.Driver.ValidateControllerServiceRequest(csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME); err != nil {
		return nil, err
	}

	volumeName := req.VolumeId
	persistentVolume, err := cs.driver.queryPersistentVolumes(ctx, volumeName)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "not found PersistentVolume[%v], error:%v", volumeName, err)
	}

	param := persistentVolume.Spec.CSI.VolumeAttributes
	cfsServer, err := newCfsServer(volumeName, param)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	err = cfsServer.deleteVolume()
	if err != nil {
		return nil, status.Error(codes.Unknown, err.Error())
	} else {
		glog.V(0).Infof("delete volume:%v success.", volumeName)
	}

	return &csi.DeleteVolumeResponse{}, nil
}

func (cs *controllerServer) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	for _, cap := range req.VolumeCapabilities {
		if cap.GetBlock() != nil {
			return &csi.ValidateVolumeCapabilitiesResponse{Message: "Not Supported"}, nil
		}
	}

	return &csi.ValidateVolumeCapabilitiesResponse{
		Confirmed: &csi.ValidateVolumeCapabilitiesResponse_Confirmed{
			VolumeCapabilities: req.VolumeCapabilities,
		},
	}, nil
}

func (cs *controllerServer) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (*csi.ControllerExpandVolumeResponse, error) {
	pvName := req.VolumeId
	pv, err := cs.driver.ClientSet.CoreV1().PersistentVolumes().Get(ctx, pvName, metav1.GetOptions{})
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "Not found PersistentVolumes[%v], error:%v", pvName, err)
	}

	attr := pv.Spec.CSI.VolumeAttributes
	cfsServer, err := newCfsServer(pvName, attr)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "newCfsServer[%v] error:%v", pvName, err)
	}

	capacityGB := req.CapacityRange.RequiredBytes >> 30
	err = cfsServer.expandVolume(capacityGB)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "expandVolume[%v] error:%v", pvName, err)
	}

	return &csi.ControllerExpandVolumeResponse{
		CapacityBytes:         req.CapacityRange.RequiredBytes,
		NodeExpansionRequired: false,
	}, nil
}
