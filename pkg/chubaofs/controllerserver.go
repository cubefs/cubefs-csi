// Copyright 2019 The Chubao Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or
// implied. See the License for the specific language governing
// permissions and limitations under the License.
package chubaofs

import (
	"context"
	"fmt"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog"
	"time"
)

type controllerServer struct {
	*csi.UnimplementedControllerServer
	caps   []*csi.ControllerServiceCapability
	driver *driver
}

func NewControllerServer(driver *driver) csi.ControllerServer {
	return &controllerServer{
		driver: driver,
		caps: getControllerServiceCapabilities(
			[]csi.ControllerServiceCapability_RPC_Type{
				csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
			}),
	}
}

func (cs *controllerServer) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	if err := cs.validateControllerServiceRequest(csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME); err != nil {
		return nil, err
	}

	capacity := req.GetCapacityRange().GetRequiredBytes()
	capacityGB := capacity >> 30
	if capacityGB == 0 {
		return nil, status.Error(codes.InvalidArgument, "apply for at least 1GB of space")
	}

	params := req.GetParameters()
	volumeId := req.GetName()
	cfsServer, err := newCfsServer(volumeId, params)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	if err := cfsServer.createVolume(capacityGB); err != nil {
		return nil, err
	}

	klog.V(0).Infof("create volume:%v success.", volumeId)
	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:      volumeId,
			CapacityBytes: capacity,
			VolumeContext: params,
		},
	}, err
}

func (cs *controllerServer) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	if err := cs.validateControllerServiceRequest(csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME); err != nil {
		return nil, err
	}

	volumeId := req.VolumeId
	persistentVolume, err := cs.driver.queryPersistentVolumes(volumeId)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("not found PersistentVolume[%v], error:%v", volumeId, err))
	}

	param := persistentVolume.Spec.CSI.VolumeAttributes
	cfsServer, err := newCfsServer(volumeId, param)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	err = cfsServer.deleteVolume()
	if err != nil {
		klog.Fatalf("delete volume:%v fail. error:%v", cfsServer.volName, err)
		go cs.deleteLegacyVolume(cfsServer)
	} else {
		klog.V(0).Infof("delete volume:%v success.", volumeId)
	}

	return &csi.DeleteVolumeResponse{}, nil
}

func (cs *controllerServer) deleteLegacyVolume(cfsServer *cfsServer) {
	for {
		time.Sleep(10 * time.Second)
		if err := cfsServer.deleteVolume(); err != nil {
			klog.Fatalf("delete volume:%v fail. error:%v", cfsServer.volName, err)
			continue
		}

		break
	}

	klog.V(0).Infof("delete volume:%v success.", cfsServer.volName)
}

func (cs *controllerServer) ControllerGetCapabilities(ctx context.Context, req *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error) {
	return &csi.ControllerGetCapabilitiesResponse{
		Capabilities: cs.caps,
	}, nil
}

func (cs *controllerServer) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	for _, _cap := range req.VolumeCapabilities {
		if _cap.GetAccessMode().GetMode() != csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER {
			return nil, status.Error(codes.InvalidArgument, "no multi node multi writer capability")
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

func (cs *controllerServer) validateControllerServiceRequest(c csi.ControllerServiceCapability_RPC_Type) error {
	if c == csi.ControllerServiceCapability_RPC_UNKNOWN {
		return nil
	}

	for _, _cap := range cs.caps {
		if c == _cap.GetRpc().GetType() {
			return nil
		}
	}
	return status.Errorf(codes.InvalidArgument, "unsupported capability %s", c)
}

func getControllerServiceCapabilities(cl []csi.ControllerServiceCapability_RPC_Type) []*csi.ControllerServiceCapability {
	var csc []*csi.ControllerServiceCapability

	for _, _cap := range cl {
		csc = append(csc, &csi.ControllerServiceCapability{
			Type: &csi.ControllerServiceCapability_Rpc{
				Rpc: &csi.ControllerServiceCapability_RPC{
					Type: _cap,
				},
			},
		})
	}

	return csc
}
