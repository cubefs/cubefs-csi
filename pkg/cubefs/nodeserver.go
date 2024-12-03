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
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	csicommon "github.com/cubefs/cubefs-csi/pkg/csi-common"
	"github.com/golang/glog"
	"golang.org/x/net/context"
	"golang.org/x/sys/unix"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/mount"
)

type nodeServer struct {
	Config
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

	// check mountPoint
	isMnt, err := IsMountPoint(targetPath)
	glog.V(5).Infof("NodePublishVolume targetPath %s isMnt %v err %v", targetPath, isMnt, err)
	if err != nil {
		// os.IsNotExist(err)  not exist
		if strings.Contains(err.Error(), "transport endpoint is not connected") {
			if err = ns.mounter.Unmount(targetPath); err != nil {
				if strings.Contains(err.Error(), "not mounted") {
					glog.Warningf("NodePublishVolume corrupt mount Unmount targetPath %s unmounted", targetPath)
				} else {
					return &csi.NodePublishVolumeResponse{}, status.Errorf(codes.Internal, "NodePublishVolume Unmount targetPath %s corrupt mount failed error: %v", targetPath, err)
				}
			}
			isMnt = false
		} else if os.IsNotExist(err) {
			// targetPath not exists
			if err = createMountPoint(targetPath); err != nil {
				return &csi.NodePublishVolumeResponse{}, status.Errorf(codes.Internal, "NodePublishVolume createMountPoint targetPath %s failed %v", targetPath, err)
			}
		} else {
			return &csi.NodePublishVolumeResponse{}, status.Errorf(codes.Internal, "NodePublishVolume IsMountPoint failed error: %v", err)
		}
	}

	// if mountPoint is right mount, return
	if isMnt {
		glog.Infof("NodePublishVolume volume already mounted correctly, targetPath: %v", targetPath)
		return &csi.NodePublishVolumeResponse{}, nil
	}

	err = bindMount(stagingTargetPath, targetPath)
	if err != nil {
		return &csi.NodePublishVolumeResponse{}, status.Errorf(codes.Internal, "NodePublishVolume  bindMount failed stagingTargetPath:%v, targetPath:%v error:%v", stagingTargetPath, targetPath, err)
	}

	duration := time.Since(start)
	glog.Infof("NodePublishVolume mount success, targetPath:%v cost:%v", targetPath, duration)
	return &csi.NodePublishVolumeResponse{}, nil
}

func (ns *nodeServer) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	ns.mutex.Lock()
	defer ns.mutex.Unlock()
	targetPath := req.GetTargetPath()

	// check mountPoint
	isMnt, err := IsMountPoint(targetPath)
	glog.V(5).Infof("NodeUnpublishVolume targetPath %s isMnt %v err %v", targetPath, isMnt, err)
	if err != nil {
		if strings.Contains(err.Error(), "transport endpoint is not connected") {
			// crush mount point, need umount
			isMnt = true
		} else if os.IsNotExist(err) {
			// os.IsNotExist(err)  not exist
			glog.V(5).Infof("NodeUnpublishVolume IsNotExist targetPath %s not exist", targetPath)
			return &csi.NodeUnpublishVolumeResponse{}, nil
		} else {
			return &csi.NodeUnpublishVolumeResponse{}, status.Errorf(codes.Internal, "NodeUnpublishVolume IsMountPoint failed error: %v", err)
		}
	}

	// if mountPoint mount umount it
	if isMnt {
		if err = ns.mounter.Unmount(targetPath); err != nil {
			if strings.Contains(err.Error(), "not mounted") {
				glog.Warningf("NodeUnpublishVolume Unmount targetPath %s is umounted", targetPath)
			} else {
				return &csi.NodeUnpublishVolumeResponse{}, status.Errorf(codes.Internal, "NodeUnpublishVolume Unmount targetPath %s failed %v", targetPath, err)
			}
		}
	}

	// remove mountPoint
	if err = os.Remove(targetPath); err != nil {
		return &csi.NodeUnpublishVolumeResponse{}, status.Errorf(codes.Internal, "NodeUnpublishVolume Remove targetPath %s failed %v", targetPath, err)
	}
	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (ns *nodeServer) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	ns.mutex.Lock()
	defer ns.mutex.Unlock()

	start := time.Now()
	stagingTargetPath := req.GetStagingTargetPath()

	glog.V(5).Infof("NodeStageVolume globalMount stagingTargetPath %s req %v", stagingTargetPath, req)
	if err := ns.mount(stagingTargetPath, req.GetVolumeId(), req.GetVolumeContext()); err != nil {
		return &csi.NodeStageVolumeResponse{}, status.Errorf(codes.Internal, "NodeStageVolume globalMount stagingTargetPath %s req %v err %v", stagingTargetPath, req, err)
	}

	duration := time.Since(start)
	glog.Infof("NodeStageVolume globalMount success, stagingTargetPath:%v cost:%v", stagingTargetPath, duration)

	return &csi.NodeStageVolumeResponse{}, nil
}

func (ns *nodeServer) mount(targetPath, volumeName string, param map[string]string) (err error) {

	// check mountPoint
	isMnt, err := IsMountPoint(targetPath)
	glog.V(5).Infof("mount targetPath %s isMnt %v err %v", targetPath, isMnt, err)
	if err != nil {
		if strings.Contains(err.Error(), "transport endpoint is not connected") {
			if err = ns.mounter.Unmount(targetPath); err != nil {
				if strings.Contains(err.Error(), "not mounted") {
					glog.Warningf("mount corrupt mount Unmount targetPath %s unmounted", targetPath)
				} else {
					return status.Errorf(codes.Internal, "mount corrupt mount Unmount targetPath %s failed %v", targetPath, err)
				}
			}
			isMnt = false
		} else if os.IsNotExist(err) {
			// os.IsNotExist(err)  not exist
			if err = createMountPoint(targetPath); err != nil {
				return status.Errorf(codes.Internal, "mount createMountPoint failed error: %v", err)
			}
		} else {
			return status.Errorf(codes.Internal, "mount IsMountPoint failed error: %v", err)
		}
	}

	// if mountPoint is right mount, return
	if isMnt {
		glog.Infof("mount volume already mounted correctly, stagingTargetPath: %v", targetPath)
		return nil
	}

	// create cfs conn and mount cfs volume
	cfsServer, err := newCfsServer(volumeName, param)
	if err != nil {
		return status.Errorf(codes.InvalidArgument, "mount new cfs server failed: %v", err)
	}

	if err = cfsServer.persistClientConf(targetPath); err != nil {
		return status.Errorf(codes.Internal, "mount persist client config file failed: %v", err)

	}

	if err = cfsServer.runClient(); err != nil {
		return status.Errorf(codes.Internal, "mount failed: %v", err)
	}

	return
}

func (ns *nodeServer) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	ns.mutex.Lock()
	defer ns.mutex.Unlock()
	stagingTargetPath := req.GetStagingTargetPath()

	// check mountPoint
	isMnt, err := IsMountPoint(stagingTargetPath)
	glog.V(5).Infof("NodeUnstageVolume stagingTargetPath %s isMnt %v err %v", stagingTargetPath, isMnt, err)
	if err != nil {
		if strings.Contains(err.Error(), "transport endpoint is not connected") {
			// crush mount point, need umount
			isMnt = true
		} else if os.IsNotExist(err) {
			// os.IsNotExist(err)  not exist
			glog.V(5).Infof("NodeUnstageVolume  IsNotExist stagingTargetPath %s not exist", stagingTargetPath)
			return &csi.NodeUnstageVolumeResponse{}, nil
		} else {
			return &csi.NodeUnstageVolumeResponse{}, status.Errorf(codes.Internal, "NodeUnstageVolume IsMountPoint failed error: %v", err)
		}
	}

	// if mountPoint is mount exec umount
	if isMnt {
		if err = ns.mounter.Unmount(stagingTargetPath); err != nil {
			if strings.Contains(err.Error(), "not mounted") {
				glog.Warningf("NodeUnstageVolume corrupt mount Unmount stagingTargetPath %s unmounted", stagingTargetPath)
			} else {
				return &csi.NodeUnstageVolumeResponse{}, status.Errorf(codes.Internal, "NodeUnstageVolume Unmount stagingTargetPath %s failed error: %v", stagingTargetPath, err)
			}
		}
	}

	// remove mountPoint
	readonly_check := fmt.Sprintf("%s/.readonly_check", stagingTargetPath)
	// not check readonly_check delete
	if err = os.Remove(readonly_check); err != nil {
		glog.Warningf("NodeUnstageVolume Remove readonly_check %s err %v", readonly_check, err)
	}
	if err = os.Remove(stagingTargetPath); err != nil {
		return &csi.NodeUnstageVolumeResponse{}, status.Errorf(codes.Internal, "NodeUnstageVolume Remove stagingTargetPath %s failed error: %v", stagingTargetPath, err)
	}
	glog.Infof("NodeUnstageVolume  stagingTargetPath %s success", stagingTargetPath)
	return &csi.NodeUnstageVolumeResponse{}, nil
}

func (ns *nodeServer) NodeGetInfo(ctx context.Context, req *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
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
			{
				Type: &csi.NodeServiceCapability_Rpc{
					Rpc: &csi.NodeServiceCapability_RPC{
						Type: csi.NodeServiceCapability_RPC_GET_VOLUME_STATS,
					},
				},
			},
		},
	}, nil
}

// NodeGetVolumeStats provides volume space and inodes usage statistics.
func (ns *nodeServer) NodeGetVolumeStats(ctx context.Context, req *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	if req.GetVolumeId() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "argument volume id is required")
	}
	volumePath := req.GetVolumePath()
	if volumePath == "" {
		return nil, status.Errorf(codes.InvalidArgument, "argument volume path is required")
	}

	isMnt, err := IsMountPoint(volumePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, status.Errorf(codes.InvalidArgument, "volume path %s does not exist", volumePath)
		}

		return nil, status.Errorf(codes.Internal, "failed to check mount point: %v", err)
	}

	if !isMnt {
		return nil, status.Error(codes.InvalidArgument, "volume path is not a valid filesystem mount point")
	}

	return nodeGetVolumeStats(ctx, volumePath)
}

// IsMountPoint judges whether the given path is a mount point or not
func IsMountPoint(p string) (bool, error) {
	is, err := mount.New("").IsLikelyNotMountPoint(p)
	if err != nil {
		return false, err
	}

	return !is, nil
}

func nodeGetVolumeStats(_ context.Context, volumePath string) (*csi.NodeGetVolumeStatsResponse, error) {
	statfs := &unix.Statfs_t{}
	err := unix.Statfs(volumePath, statfs)
	if err != nil {
		return nil, err
	}

	// Available is blocks available * fragment size
	available := int64(statfs.Bavail) * int64(statfs.Bsize)

	// Capacity is total block count * fragment size
	capacity := int64(statfs.Blocks) * int64(statfs.Bsize)

	// Usage is block being used * fragment size (aka block size).
	usage := (int64(statfs.Blocks) - int64(statfs.Bfree)) * int64(statfs.Bsize)

	inodes := int64(statfs.Files)
	inodesFree := int64(statfs.Ffree)
	inodesUsed := inodes - inodesFree

	return &csi.NodeGetVolumeStatsResponse{
		Usage: []*csi.VolumeUsage{
			{
				Available: available,
				Total:     capacity,
				Used:      usage,
				Unit:      csi.VolumeUsage_BYTES,
			},
			{
				Available: inodesFree,
				Total:     inodes,
				Used:      inodesUsed,
				Unit:      csi.VolumeUsage_INODES,
			},
		},
	}, nil
}

// getAttachedPVOnNode finds all persistent volume objects attached in the node and controlled by me.
func (ns *nodeServer) getAttachedPVOnNode(nodeName string) ([]*v1.PersistentVolume, error) {
	vaList, err := ns.Driver.ClientSet.StorageV1().VolumeAttachments().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("unable to list VolumeAttachments: %v", err)
	}

	nodePVNames := make(map[string]struct{})
	for _, va := range vaList.Items {
		if va.Spec.NodeName == nodeName &&
			va.Spec.Attacher == DriverName &&
			va.Status.Attached &&
			va.Spec.Source.PersistentVolumeName != nil {
			nodePVNames[*va.Spec.Source.PersistentVolumeName] = struct{}{}
		}
	}

	pvList, err := ns.Driver.ClientSet.CoreV1().PersistentVolumes().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("unable to list PersistentVolumes: %v", err)
	}

	nodePVs := make([]*v1.PersistentVolume, 0, len(nodePVNames))
	for i := range pvList.Items {
		_, exist := nodePVNames[pvList.Items[i].Name]
		if exist {
			nodePVs = append(nodePVs, &pvList.Items[i])
		}
	}

	return nodePVs, nil
}

type persistentVolumeWithPods struct {
	*v1.PersistentVolume
	pods []*v1.Pod
}

func (p *persistentVolumeWithPods) appendPodUnique(new *v1.Pod) {
	for _, old := range p.pods {
		if old.UID == new.UID {
			return
		}
	}

	p.pods = append(p.pods, new)
}

// getAttachedPVWithPodsOnNode finds all persistent volume objects as well as the related pods in the node.
func (ns *nodeServer) getAttachedPVWithPodsOnNode(nodeName string) ([]*persistentVolumeWithPods, error) {
	pvs, err := ns.getAttachedPVOnNode(nodeName)
	if err != nil {
		return nil, fmt.Errorf("getAttachedPVOnNode faied: %v", err)
	}

	claimedPVWithPods := make(map[string]*persistentVolumeWithPods, len(pvs))
	for _, pv := range pvs {
		if pv.Spec.ClaimRef == nil {
			continue
		}

		pvcKey := fmt.Sprintf("%s/%s", pv.Spec.ClaimRef.Namespace, pv.Spec.ClaimRef.Name)
		claimedPVWithPods[pvcKey] = &persistentVolumeWithPods{
			PersistentVolume: pv,
		}
	}

	allPodsOnNode, err := ns.Driver.ClientSet.CoreV1().Pods("").List(context.Background(), metav1.ListOptions{
		FieldSelector: "spec.nodeName=" + nodeName,
	})
	if err != nil {
		return nil, fmt.Errorf("list pods failed: %v", err)
	}

	for i := range allPodsOnNode.Items {
		pod := allPodsOnNode.Items[i]

		for _, volume := range pod.Spec.Volumes {
			if volume.PersistentVolumeClaim == nil {
				continue
			}
			pvcKey := fmt.Sprintf("%s/%s", pod.Namespace, volume.PersistentVolumeClaim.ClaimName)
			pvWithPods, ok := claimedPVWithPods[pvcKey]
			if !ok {
				continue
			}

			pvWithPods.appendPodUnique(&pod)
		}
	}

	ret := make([]*persistentVolumeWithPods, 0, len(claimedPVWithPods))
	for _, v := range claimedPVWithPods {
		if len(v.pods) != 0 {
			ret = append(ret, v)
		}
	}

	return ret, nil
}

// remountDamagedVolumes try to remount all the volumes damaged during csi-node restart,
// includes the GlobalMount per pv and BindMount per pod.
func (ns *nodeServer) remountDamagedVolumes(nodeName string) {
	// need retry mount if err, stat will return normal
	startTime := time.Now()

	pvWithPods, err := ns.getAttachedPVWithPodsOnNode(nodeName)
	if err != nil {
		glog.Warningf("get attached pv with pods info failed: %v\n", err)
		return
	}

	if len(pvWithPods) == 0 {
		return
	}

	var wg sync.WaitGroup
	wg.Add(len(pvWithPods))
	for _, pvp := range pvWithPods {
		go func(p *persistentVolumeWithPods) {
			defer wg.Done()
			// need retry mount if error
			glog.V(5).Infof("remountDamagedVolumes begin do volume mount %s", p.Name)
			globalMountPath := filepath.Join(ns.KubeletRootDir, fmt.Sprintf("/plugins/kubernetes.io/csi/pv/%s/globalmount", p.Name))
			var err error
			for i := 0; i < 5; i++ {
				// some times mount return success, actually is broken mount point, retry 3 time
				err = ns.dealPodVolumeMount(p, globalMountPath)
				if err != nil {
					glog.Warningf("remountDamagedVolumes dealPodVolumeMount volume mount %s globalMountPath %s error %v", p.Name, globalMountPath, err)
				}
			}
			if err == nil {
				glog.V(5).Infof("remountDamagedVolumes dealPodVolumeMount volume mount %s globalMountPath %s success", globalMountPath, p.Name)
			}
		}(pvp)
	}
	wg.Wait()

	glog.Infof("remountDamagedVolumes remount finished cost %d ms", time.Since(startTime).Milliseconds())

}

func (ns *nodeServer) dealPodVolumeMount(p *persistentVolumeWithPods, globalMountPath string) error {
	// dealPodVolumeMount umount globalMountPath before
	if err := forceUmount(globalMountPath); err != nil {
		glog.Warningf("dealPodVolumeMount Unmount globalMountPath %s err %v", globalMountPath, err)
	}

	if err := ns.mount(globalMountPath, p.Name, p.Spec.CSI.VolumeAttributes); err != nil {
		return status.Errorf(codes.Internal, "dealPodVolumeMount globalMount mount volume %q to path %q failed: %v", p.Name, globalMountPath, err)
	}

	glog.Infof("dealPodVolumeMount volume %q to global mount path %q succeed.", p.Name, globalMountPath)
	// bind globalmount to pods， mount --bind -o remount 可以解决mount --bind多次挂载问题，但不能解决挂载点损坏问题，需要umount掉再次mount
	for _, pod := range p.pods {
		podDir := filepath.Join(ns.KubeletRootDir, "/pods/", string(pod.UID))

		podMountPath := filepath.Join(podDir, fmt.Sprintf("/volumes/kubernetes.io~csi/%s/mount", p.Name))
		if err := forceUmount(podMountPath); err != nil {
			glog.Warningf("dealPodVolumeMount Unmount podMountPath %s err %v", podMountPath, err)
		}
		if err := bindMount(globalMountPath, podMountPath); err != nil {
			return status.Errorf(codes.Internal, "dealPodVolumeMount rebind damaged volume %q to path %q failed: %v\n", p.Name, podMountPath, err)
		}

		glog.Infof("dealPodVolumeMount rebind damaged volume %q to pod mount path %q succeed.", p.Name, podMountPath)

		// bind pod volume to subPath mount point
		for _, container := range pod.Spec.Containers {
			for i, volumeMount := range container.VolumeMounts {
				if volumeMount.SubPath == "" {
					continue
				}

				source := filepath.Join(podMountPath, volumeMount.SubPath)

				// ref: https://github.com/kubernetes/kubernetes/blob/v1.22.0/pkg/volume/util/subpath/subpath_linux.go#L158
				subMountPath := filepath.Join(podDir, "volume-subpaths", p.Name, container.Name, strconv.Itoa(i))
				glog.V(5).Infof("dealPodVolumeMount subMountPath stagingTargetPath  %s targetPath %s ", source, subMountPath)
				if err := forceUmount(subMountPath); err != nil {
					glog.Warningf("dealPodVolumeMount Unmount subMountPath %s err %v", subMountPath, err)
				}
				if err := bindMount(source, subMountPath); err != nil {
					return status.Errorf(codes.Internal, "dealPodVolumeMount rebind volume %q to sub mount path %q failed: %v\n", p.Name, subMountPath, err)
				}

				glog.Infof("dealPodVolumeMount rebind volume %q to sub mount path %q succeed.", p.Name, subMountPath)
			}
		}
	}
	return nil
}

func forceUmount(target string) error {
	command := exec.Command("umount", target)
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("forceUmount umount target err %v output %s", err, string(output))
	}
	return nil
}
