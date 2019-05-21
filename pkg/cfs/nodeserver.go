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
	"encoding/json"
	"os"
	"os/exec"
	"path"

	"github.com/container-storage-interface/spec/lib/go/csi/v0"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/kubernetes/pkg/util/mount"
	"k8s.io/kubernetes/pkg/volume/util"

	"github.com/chubaofs/chubaofs-csi/pkg/csi-common"
	"github.com/golang/glog"
	"time"
)

type nodeServer struct {
	*csicommon.DefaultNodeServer
}

func WriteBytes(filePath string, b []byte) (int, error) {
	os.MkdirAll(path.Dir(filePath), os.ModePerm)
	fw, err := os.Create(filePath)
	if err != nil {
		return 0, err
	}
	defer fw.Close()
	return fw.Write(b)
}

func (ns *nodeServer) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	glog.V(2).Infof("-----2----NodePublishVolume req:%v", req)
	targetPath := req.GetTargetPath()
	notMnt, err := mount.New("").IsLikelyNotMountPoint(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			if err := os.MkdirAll(targetPath, 0750); err != nil {
				glog.Errorf("Create path:%v to mount is failed. err:%v", targetPath, err)
				return nil, status.Error(codes.Internal, err.Error())
			}
			notMnt = true
		} else {
			glog.Errorf("Mount path:%v is failed. err:%v", targetPath, err)
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	if !notMnt {
		return &csi.NodePublishVolumeResponse{}, nil
	}

	mo := req.GetVolumeCapability().GetMount().GetMountFlags()
	if req.GetReadonly() {
		mo = append(mo, "ro")
	}

	master1 := req.GetVolumeAttributes()[KEY_CFS_MASTER1]
	master2 := req.GetVolumeAttributes()[KEY_CFS_MASTER2]
	master3 := req.GetVolumeAttributes()[KEY_CFS_MASTER3]
	volName := req.GetVolumeAttributes()[KEY_VOLUME_NAME]

	cfgmap := make(map[string]interface{})
	cfgmap[FUSE_KEY_MOUNT_POINT_V1] = targetPath
	cfgmap[FUSE_KEY_MOUNT_POINT_V2] = targetPath
	cfgmap[FUSE_KEY_VOLUME_NAME_V1] = volName
	cfgmap[FUSE_KEY_VOLUME_NAME_V2] = volName
	cfgmap[FUSE_KEY_MASTER_ADDR_V1] = master1 + "," + master2 + "," + master3
	cfgmap[FUSE_KEY_MASTER_ADDR_V2] = master1 + "," + master2 + "," + master3
	cfgmap[FUSE_KEY_LOG_PATH_V1] = "/export/Logs/cfs/client/"
	cfgmap[FUSE_KEY_LOG_PATH_V2] = "/export/Logs/cfs/client/"
	cfgmap[FUSE_KEY_LOG_LEVEL_V1] = "error"
	cfgmap[FUSE_KEY_LOG_LEVEL_V2] = "error"
	cfgmap[FUSE_KEY_LOOKUP_VALID_V1] = "30"
	cfgmap[FUSE_KEY_OWNER_V1] = "cfs"
	cfgmap[FUSE_KEY_PROF_PORT_V1] = "10094"
	cfgmap[FUSE_KEY_PROF_PORT_V2] = "10094"

	cfgstr, err := json.MarshalIndent(cfgmap, "", "      ")
	if err != nil {
		glog.Errorf("cfs client cfg map to json err:%v \n", err)
		return &csi.NodePublishVolumeResponse{}, err
	}

	WriteBytes(CFS_FUSE_CONFIG_PATH, cfgstr)
	glog.V(4).Infof("Parameters of cfs-client is %v", string(cfgstr))

	var (
		cmdErrChan = make(chan error)
		cmdErr     error
		cmd        = exec.Command("/usr/bin/cfs-client", "-c", CFS_FUSE_CONFIG_PATH)
	)
	go func() {
		glog.V(4).Infof("In background do /usr/bin/cfs-client -c %v", CFS_FUSE_CONFIG_PATH)
		if err := cmd.Run(); err != nil {
			glog.Errorf("cfs client exec is failed. err:%v", err)
			cmdErrChan <- err
			return
		}
		glog.Error("cfs client exec had existed")
	}()
	select {
	case cmdErr = <-cmdErrChan:
	case <-time.After(5 * time.Second):
		glog.V(2).Infof("cfs client had started. mount volume:%v success", volName)
	}
	if cmdErr != nil {
		glog.Error(cmdErr)
		return nil, status.Error(codes.Internal, cmdErr.Error())
	}

	return &csi.NodePublishVolumeResponse{}, nil
}

func (ns *nodeServer) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	glog.V(2).Infof("---------NodeUnpublishVolume req:%v", req)
	targetPath := req.GetTargetPath()
	notMnt, err := mount.New("").IsLikelyNotMountPoint(targetPath)

	if err != nil {
		if os.IsNotExist(err) {
			return nil, status.Error(codes.NotFound, "Targetpath not found")
		} else {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}
	if notMnt {
		return nil, status.Error(codes.NotFound, "Volume not mounted")
	}

	err = util.UnmountPath(req.GetTargetPath(), mount.New(""))
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
