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
	"encoding/json"
	"os"

	"context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/kubernetes/pkg/util/mount"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/glog"
)

type nodeServer struct {
	nodeID        string
	masterAddress string
}

func NewNodeServer(nodeId string, masterAddress string) *nodeServer {
	return &nodeServer{
		nodeID:        nodeId,
		masterAddress: masterAddress,
	}
}

func (ns *nodeServer) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	glog.V(2).Infof("NodePublishVolume req:%v", req)

	// Check arguments
	if req.GetVolumeCapability() == nil {
		return nil, status.Error(codes.InvalidArgument, "Volume capability missing in request")
	}

	if req.GetVolumeCapability().GetMount() == nil {
		return nil, status.Error(codes.InvalidArgument, "Volume missing mount capability")
	}

	if len(req.GetVolumeId()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume ID missing in request")
	}

	if len(req.GetTargetPath()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Target path missing in request")
	}

	targetPath := req.GetTargetPath()
	notMnt, err := mount.New("").IsLikelyNotMountPoint(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			if err := os.MkdirAll(targetPath, 0750); err != nil {
				return nil, status.Errorf(codes.Internal, "Failed to mkdir targetPath(%v) err(%v)", targetPath, err)
			}
			notMnt = true
		} else {
			return nil, status.Errorf(codes.Internal, "Failed to stat targetPath(%v) err(%v)", targetPath, err)
		}
	}

	if !notMnt {
		return &csi.NodePublishVolumeResponse{}, nil
	}

	// Get mount options
	mo := req.GetVolumeCapability().GetMount().GetMountFlags()
	if req.GetReadonly() {
		mo = append(mo, "ro")
	}

	masterAddr := ns.masterAddress
	volumeId := req.GetVolumeId()

	cfgmap := make(map[string]interface{})
	cfgmap[KMountPoint] = targetPath
	cfgmap[KVolumeName] = volumeId
	cfgmap[KMasterAddr] = masterAddr
	// FIXME
	cfgmap[KLogDir] = "/export/Logs/cfs"
	cfgmap[KWarnLogDir] = "/export/Logs/cfs/client/warn/"
	cfgmap[KLogLevel] = "error"
	cfgmap[KOwner] = defaultOwner
	cfgmap[KProfPort] = "10094"
	//the parameters below are all set by default value
	cfgmap[KLookupValid] = "30"
	cfgmap[KIcacheTimeout] = ""
	cfgmap[KAttrValid] = ""
	cfgmap[KEnSyncWrite] = ""
	cfgmap[KAutoInvalData] = ""
	cfgmap[KRdonly] = "false"
	cfgmap[KWriteCache] = "false"
	cfgmap[KKeepCache] = "false"

	cfgstr, err := json.MarshalIndent(cfgmap, "", "      ")
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to marshal client config err(%v)", err)
	}

	if _, err = generateFile(defaultClientConfig, cfgstr); err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to generate client config file, path(%v) err(%v)", defaultClientConfig, err)
	}

	glog.V(4).Infof("Parameters of cfs-client is %v", string(cfgstr))

	if err = doMount("/usr/bin/cfs-client", defaultClientConfig); err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to mount, err(%v)", err)
	}

	return &csi.NodePublishVolumeResponse{}, nil
}

func (ns *nodeServer) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	glog.V(2).Infof("NodeUnpublishVolume req:%v", req)

	// Check arguments
	if len(req.GetVolumeId()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume ID missing in request")
	}

	if len(req.GetTargetPath()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Target path missing in request")
	}

	targetPath := req.GetTargetPath()

	notMnt, err := mount.New("").IsLikelyNotMountPoint(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, status.Error(codes.NotFound, "Targetpath not found")
		} else {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}
	//assuming success if already unmounted
	if notMnt {
		return &csi.NodeUnpublishVolumeResponse{}, nil
	}

	err = mount.New("").Unmount(req.GetTargetPath())
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (ns *nodeServer) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	glog.V(2).Infof("---------NodeStageVolume req:%v", req)
	return &csi.NodeStageVolumeResponse{}, nil
}

func (ns *nodeServer) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	glog.V(2).Infof("---------NodeUnstageVolume req:%v", req)
	return &csi.NodeUnstageVolumeResponse{}, nil
}

func (ns *nodeServer) NodeGetInfo(ctx context.Context, req *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {

	return &csi.NodeGetInfoResponse{
		NodeId: ns.nodeID,
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

func (ns *nodeServer) NodeGetVolumeStats(ctx context.Context, in *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}
