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
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	csicommon "github.com/cubefs/cubefs-csi/pkg/csi-common"
	"golang.org/x/sys/unix"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	v1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	"k8s.io/utils/mount"
)

type nodeServer struct {
	Config
	*csicommon.DefaultNodeServer
	mounter mount.Interface
	mutex   sync.RWMutex
}

// persistentVolumeWithPods 关联PV与使用该PV的Pod
type persistentVolumeWithPods struct {
	*v1.PersistentVolume
	pods []*v1.Pod
}

// buildPodSpec 构建Cubefs Client Pod规格
func (ns *nodeServer) buildPodSpec(ctx context.Context, targetPath, volName string, volumeContext map[string]string) (*v1.Pod, error) {
	pvcName := getParamWithDefault(volumeContext, KeyCSIPVCName, "")
	pvcNamespace := getParamWithDefault(volumeContext, KeyCSIPVCNamespace, "")
	clientImage := getParamWithDefault(volumeContext, KeyPodClientImage, "")
	cfsMasterStr := getParamWithDefault(volumeContext, KeyCfsMaster, "")

	required := map[string]string{
		"PVCName":       pvcName,
		"PVCNamespace":  pvcNamespace,
		"ClientImage":   clientImage,
		"Cubefs Master": cfsMasterStr,
	}
	for name, val := range required {
		if val == "" {
			return nil, fmt.Errorf("Pod mounting mode is missing required parameters, %s", name)
		}
	}

	cfsVolName := getParamWithDefault(volumeContext, KeyCfsVolName, volName)
	cfsServer, err := NewCfsServer(cfsVolName, volumeContext)
	if err != nil {
		return nil, fmt.Errorf("NewCfsServer fail, volumeName: %s, err: %v", cfsVolName, err)
	}
	if err := cfsServer.PersistClientConf(targetPath); err != nil {
		return nil, fmt.Errorf("cfsServer.PersistClientConf fail, volumeName: %s, err: %v", cfsVolName, err)
	}
	confFilePath := cfsServer.clientConfFile

	confBytes, err := os.ReadFile(confFilePath)
	if err != nil {
		return nil, fmt.Errorf("os.ReadFile fail, confFilePath: %s,  err: %v", confFilePath, err)
	}
	clientConfContent := string(confBytes)

	pvc, err := ns.Driver.ClientSet.CoreV1().PersistentVolumeClaims(pvcNamespace).Get(ctx, pvcName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("PVC query failed: %s/%s, err: %v", pvcNamespace, pvcName, err)
	}

	resourceReq, resourceLimit := ParsePodResource(volumeContext)
	runCmd := fmt.Sprintf(
		"mkdir -p %s %s && "+
			"echo '%s' > %s && "+
			"%s %s %s; "+
			"sleep 9999999d",
		DefaultClientConfPath, cfsServer.clientConf.LogDir,
		clientConfContent, confFilePath,
		CfsClientBin, CfsConfArg, confFilePath,
	)
	return &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("cfs-clientpod-%s", pvcName),
			Namespace: pvcNamespace,
			Labels: map[string]string{
				"app":            "cfs-clientpod",
				"csi-client-pod": "true",
				"vol-name":       cfsVolName,
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         "v1",
					Kind:               "PersistentVolumeClaim",
					Name:               pvcName,
					UID:                pvc.UID,
					Controller:         func() *bool { b := true; return &b }(),
					BlockOwnerDeletion: func() *bool { b := true; return &b }(),
				},
			},
			Annotations: map[string]string{
				"container.apparmor.security.beta.kubernetes.io/cfs-clientpod": "unconfined",
				"io.kubernetes.cri-o.userns-mode":                              "host",
				"kubernetes.io/psp":                                            "privileged",
			},
		},
		Spec: v1.PodSpec{
			NodeName:      ns.Driver.NodeID,
			RestartPolicy: v1.RestartPolicyAlways,
			Containers: []v1.Container{
				{
					Name:    "cfs-clientpod",
					Image:   clientImage,
					Command: []string{"bash", "-c", runCmd},
					Resources: v1.ResourceRequirements{
						Requests: resourceReq,
						Limits:   resourceLimit,
					},
					SecurityContext: &v1.SecurityContext{
						Privileged: func() *bool { b := true; return &b }(),
						Capabilities: &v1.Capabilities{
							Add: []v1.Capability{"SYS_ADMIN"},
						},
					},
					VolumeMounts: []v1.VolumeMount{
						{Name: "cfs-conf", MountPath: DefaultClientConfPath, ReadOnly: false},
						{Name: "cfs-logs", MountPath: DefaultLogDir, ReadOnly: false},
						{Name: "csi-target-mount", MountPath: targetPath, ReadOnly: false, MountPropagation: func() *v1.MountPropagationMode { m := v1.MountPropagationBidirectional; return &m }()},
					},
					LivenessProbe: &v1.Probe{
						Handler:             v1.Handler{Exec: &v1.ExecAction{Command: []string{"mountpoint", targetPath}}},
						InitialDelaySeconds: 30,
						TimeoutSeconds:      3,
						PeriodSeconds:       5,
						FailureThreshold:    3,
					},
					ReadinessProbe: &v1.Probe{
						Handler:             v1.Handler{Exec: &v1.ExecAction{Command: []string{"mountpoint", targetPath}}},
						InitialDelaySeconds: 40,
						TimeoutSeconds:      3,
						PeriodSeconds:       10,
						FailureThreshold:    5,
					},
				},
			},
			Volumes: []v1.Volume{
				{
					Name: "cfs-conf",
					VolumeSource: v1.VolumeSource{
						EmptyDir: &v1.EmptyDirVolumeSource{},
					},
				},
				{
					Name: "cfs-logs",
					VolumeSource: v1.VolumeSource{
						EmptyDir: &v1.EmptyDirVolumeSource{},
					},
				},
				{
					Name: "csi-target-mount",
					VolumeSource: v1.VolumeSource{
						HostPath: &v1.HostPathVolumeSource{
							Path: targetPath,
							Type: func() *v1.HostPathType { t := v1.HostPathDirectoryOrCreate; return &t }(),
						},
					},
				},
			},
			NodeSelector: map[string]string{"cubefs/support": "true"},
		},
	}, nil
}

func (ns *nodeServer) waitForClientPodReady(ctx context.Context, podName, podNamespace, targetPath string) (*v1.Pod, error) {
	timeout := 2 * time.Minute
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			pod, err := ns.Driver.ClientSet.CoreV1().Pods(podNamespace).Get(ctx, podName, metav1.GetOptions{})
			if err != nil {
				klog.Warningf("query Pod status fail, %s/%s, err: %v", podNamespace, podName, err)
				continue
			}

			// Pod status is Running
			if pod.Status.Phase != v1.PodRunning {
				continue
			}

			// cfs-clientpod containerReady
			containerReady := false
			for _, cs := range pod.Status.ContainerStatuses {
				if cs.Name == "cfs-clientpod" && cs.Ready {
					containerReady = true
					break
				}
			}
			if !containerReady {
				continue
			}

			// targetPath is a mountpoint
			isMount, err := isMountPoint(targetPath)
			if err != nil {
				klog.Warningf("isMountPoint fail, targetPath: %s, err: %v", targetPath, err)
				continue
			}
			if isMount {
				klog.Infof("Pod ready and successfully mounted, %s/%s, targetPath: %s", podNamespace, podName, targetPath)
				return pod, nil
			}

			klog.Infof("Pod started, but mountpoint not ready, targetPath: %s (waiting)", targetPath)
		}
	}
	return nil, fmt.Errorf("waiting for Pod to be ready or mounting timeout (%v), %s/%s", timeout, podNamespace, podName)
}

// mountViaPod 通过ClientPod挂载CubeFS
func (ns *nodeServer) mountViaPod(ctx context.Context, targetPath, volumeName string, volumeContext map[string]string) (retErr error) {
	defer func() {
		if retErr != nil {
			pvcName := getParamWithDefault(volumeContext, KeyCSIPVCName, "")
			pvcNamespace := getParamWithDefault(volumeContext, KeyCSIPVCNamespace, "")
			if pvcName != "" && pvcNamespace != "" {
				podName := fmt.Sprintf("cfs-clientpod-%s", pvcName)
				klog.Errorf("Pod mounting failed, clean Pod. %s/%s, volumeName: %s", pvcNamespace, podName, volumeName)
				if delErr := ns.Driver.ClientSet.CoreV1().Pods(pvcNamespace).Delete(ctx, podName, metav1.DeleteOptions{}); delErr != nil && !k8serrors.IsNotFound(delErr) {
					klog.Warningf("delete Pod fail, %s/%s, err: %v", pvcNamespace, podName, delErr)
				}
			}
			if cleanErr := mount.CleanupMountPoint(targetPath, ns.mounter, false); cleanErr != nil {
				klog.Warningf("mount.CleanupMountPoint fail, targetPath: %s, err: %v", targetPath, cleanErr)
			}
		}
	}()

	if err := mount.CleanupMountPoint(targetPath, ns.mounter, false); err != nil {
		return fmt.Errorf("mount.CleanupMountPoint fail, targetPath: %s, volumeName: %s, err: %v", targetPath, volumeName, err)
	}
	if err := createMountPoint(targetPath); err != nil {
		return fmt.Errorf("createMountPoint fail, targetPath: %s, volumeName: %s, err: %v", targetPath, volumeName, err)
	}

	podSpec, err := ns.buildPodSpec(ctx, targetPath, volumeName, volumeContext)
	if err != nil {
		return fmt.Errorf("build Pod spec fail, volumeName: %s, err: %v", volumeName, err)
	}
	podName := podSpec.Name
	podNamespace := podSpec.Namespace

	existingPod, err := ns.Driver.ClientSet.CoreV1().Pods(podNamespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			if _, err := ns.Driver.ClientSet.CoreV1().Pods(podNamespace).Create(ctx, podSpec, metav1.CreateOptions{}); err != nil {
				return fmt.Errorf("create Pod fail, %s/%s. volumeName: %s, err: %v", podNamespace, podName, volumeName, err)
			}
			klog.Infof("create Pod success, %s/%s. volumeName: %s", podNamespace, podName, volumeName)
		} else {
			return fmt.Errorf("query Pod fail, %s/%s. volumeName: %s, err: %v", podNamespace, podName, volumeName, err)
		}
	} else if existingPod.Spec.NodeName != ns.Driver.NodeID { // 调度到错误节点，删除重建
		if err := ns.Driver.ClientSet.CoreV1().Pods(podNamespace).Delete(ctx, podName, metav1.DeleteOptions{}); err != nil {
			return fmt.Errorf("delete incorrect node Pod, %s/%s. err: %v", podNamespace, podName, err)
		}
		if _, err := ns.Driver.ClientSet.CoreV1().Pods(podNamespace).Create(ctx, podSpec, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("rebuilding Pod fail, %s/%s. node: %s, err: %v", podNamespace, podName, ns.Driver.NodeID, err)
		}
	}

	_, err = ns.waitForClientPodReady(ctx, podName, podNamespace, targetPath)
	if err != nil {
		return fmt.Errorf("waitForClientPodReady fail, volumeName: %s, err: %v", volumeName, err)
	}

	isMount, err := isMountPoint(targetPath)
	if err != nil || !isMount {
		return fmt.Errorf("Pod mounting failed, targetPath: %s, volumeName: %s, err: %v", targetPath, volumeName, err)
	}

	klog.Infof("Pod mounting success, volumeName: %s, targetPath: %s", volumeName, targetPath)
	return nil
}

// mountDirectly 宿主直接挂载CubeFS
func (ns *nodeServer) mountDirectly(ctx context.Context, targetPath, volumeName string, volumeContext map[string]string) (retErr error) {
	var confFilePath string
	defer func() {
		if retErr != nil {
			klog.Errorf("mountDirectly fail, volumeName: %s, err: %v", volumeName, retErr)
			if confFilePath != "" {
				if err := os.Remove(confFilePath); err != nil && !os.IsNotExist(err) {
					klog.Warningf("delete confFilePath fail, confFilePath: %s, err: %v", confFilePath, err)
				}
			}
			if err := mount.CleanupMountPoint(targetPath, ns.mounter, false); err != nil {
				klog.Warningf("mount.CleanupMountPoint fail, targetPath: %s, err: %v", targetPath, err)
			}
		}
	}()

	cfsServer, err := NewCfsServer(volumeName, volumeContext)
	if err != nil {
		return fmt.Errorf("NewCfsServer fail, volumeName: %s, err: %v", volumeName, err)
	}
	if err := cfsServer.PersistClientConf(targetPath); err != nil {
		return fmt.Errorf("cfsServer.PersistClientConf fail, volumeName: %s, err: %v", volumeName, err)
	}
	confFilePath = cfsServer.clientConfFile

	if err := mount.CleanupMountPoint(targetPath, ns.mounter, false); err != nil {
		return fmt.Errorf("mount.CleanupMountPoint fail, targetPath: %s, volumeName: %s, err: %v", targetPath, volumeName, err)
	}
	if err := createMountPoint(targetPath); err != nil {
		return fmt.Errorf("createMountPoint fail, targetPath: %s, volumeName: %s, err: %v", targetPath, volumeName, err)
	}

	runCmd := exec.CommandContext(
		ctx,
		CfsClientBin,
		CfsConfArg,
		confFilePath,
	)
	output, err := runCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("cfs-client mount fail, volumeName: %s, cmd: %s, output: %s, err: %v",
			volumeName, strings.Join(runCmd.Args, " "), string(output), err)
	}

	isMount, err := isMountPoint(targetPath)
	if err != nil || !isMount {
		return fmt.Errorf("isMountPoint fail, volumeName: %s, targetPath: %s, err: %v", volumeName, targetPath, err)
	}

	klog.Infof("mountDirectly success, volumeName: %s, targetPath: %s", volumeName, targetPath)
	return nil
}

// mount 选择 Pod/直接 挂载
func (ns *nodeServer) mount(ctx context.Context, targetPath, volumeName string, volumeContext map[string]string) error {
	enablePodMount := parseBool(getParamWithDefault(volumeContext, KeyPodMountEnable, "false"))
	if enablePodMount {
		return ns.mountViaPod(ctx, targetPath, volumeName, volumeContext)
	}
	return ns.mountDirectly(ctx, targetPath, volumeName, volumeContext)
}

func (ns *nodeServer) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	ns.mutex.Lock()
	defer ns.mutex.Unlock()

	start := time.Now()
	stagingPath := req.GetStagingTargetPath()
	volumeID := req.GetVolumeId()
	volumeContext := PickupVolumeContext(req.GetVolumeContext())

	if volumeID == "" || stagingPath == "" {
		return nil, status.Errorf(codes.InvalidArgument, "NodeStageVolume missing parameter, volumeID: %s, stagingPath: %s", volumeID, stagingPath)
	}

	if err := ns.mount(ctx, stagingPath, volumeID, volumeContext); err != nil {
		return nil, status.Errorf(codes.Internal, "NodeStageVolume fail, volumeID: %s, stagingPath: %s, err: %v", volumeID, stagingPath, err)
	}

	klog.Infof("NodeStageVolume success, volumeID: %s, stagingPath: %s, cost: %v", volumeID, stagingPath, time.Since(start))
	return &csi.NodeStageVolumeResponse{}, nil
}

func (ns *nodeServer) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	ns.mutex.Lock()
	defer ns.mutex.Unlock()

	start := time.Now()
	stagingPath := req.GetStagingTargetPath()
	targetPath := req.GetTargetPath()
	volumeID := req.GetVolumeId()
	readOnly := req.GetReadonly()

	if volumeID == "" || stagingPath == "" || targetPath == "" {
		return nil, status.Errorf(codes.InvalidArgument, "NodePublishVolume missing parameter, volumeID: %s, stagingPath: %s, targetPath: %s", volumeID, stagingPath, targetPath)
	}

	isStagingMount, err := isMountPoint(stagingPath)
	if err != nil || !isStagingMount {
		return nil, status.Errorf(codes.FailedPrecondition, "stagingPath not mounted, volumeID: %s, stagingPath: %s, err: %v", volumeID, stagingPath, err)
	}

	if err := mount.CleanupMountPoint(targetPath, ns.mounter, false); err != nil {
		return nil, status.Errorf(codes.Internal, "mount.CleanupMountPoint fail, volumeID: %s, targetPath: %s, err: %v", volumeID, targetPath, err)
	}
	if err := createMountPoint(targetPath); err != nil {
		return nil, status.Errorf(codes.Internal, "createMountPoint fail, volumeID: %s, targetPath: %s, err: %v", volumeID, targetPath, err)
	}

	if err := bindMount(stagingPath, targetPath, readOnly); err != nil {
		return nil, status.Errorf(codes.Internal, "bindMount fail: volumeID: %s, stagingPath: %s -> targetPath: %s, err: %v", volumeID, stagingPath, targetPath, err)
	}

	klog.Infof("NodePublishVolume success, volumeID: %s, targetPath: %s, cost: %v", volumeID, targetPath, time.Since(start))
	return &csi.NodePublishVolumeResponse{}, nil
}

func (ns *nodeServer) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	ns.mutex.Lock()
	defer ns.mutex.Unlock()

	stagingPath := req.GetStagingTargetPath()
	volumeID := req.GetVolumeId()
	volumeContext := make(map[string]string)
	var targetPV *v1.PersistentVolume

	if volumeID == "" || stagingPath == "" {
		return nil, status.Errorf(codes.InvalidArgument, "NodeUnstageVolume missing parameter, volumeID: %s, stagingPath: %s", volumeID, stagingPath)
	}

	// VolumeAttachment -> PV提取volumeContext
	vaList, err := ns.Driver.ClientSet.StorageV1().VolumeAttachments().List(ctx, metav1.ListOptions{})
	if err != nil {
		klog.Warningf("VolumeAttachments List fail, volumeID: %s, err: %v", volumeID, err)
	} else {
		var candidatePVNames []string
		for _, va := range vaList.Items {
			if va.Spec.NodeName == ns.Driver.NodeID && va.Status.Attached && va.Spec.Source.PersistentVolumeName != nil {
				candidatePVNames = append(candidatePVNames, *va.Spec.Source.PersistentVolumeName)
			}
		}

		for _, pvName := range candidatePVNames {
			pv, err := ns.Driver.ClientSet.CoreV1().PersistentVolumes().Get(ctx, pvName, metav1.GetOptions{})
			if err != nil {
				klog.Warningf("PV query failed, pvName: %s, volumeID: %s, err: %v", pvName, volumeID, err)
				continue
			}
			if pv.Spec.CSI != nil && pv.Spec.CSI.VolumeHandle == volumeID {
				targetPV = pv
				volumeContext = pv.Spec.CSI.VolumeAttributes
				break
			}
		}
	}

	if err := mount.CleanupMountPoint(stagingPath, ns.mounter, false); err != nil {
		return nil, status.Errorf(codes.Internal, "NodeUnstageVolume fail, stagingPath: %s, volumeID: %s, err: %v", stagingPath, volumeID, err)
	}

	enablePodMount := parseBool(getParamWithDefault(volumeContext, KeyPodMountEnable, "false"))
	if enablePodMount && targetPV != nil && targetPV.Spec.ClaimRef != nil {
		pvcName := targetPV.Spec.ClaimRef.Name
		pvcNamespace := targetPV.Spec.ClaimRef.Namespace
		podName := fmt.Sprintf("cfs-clientpod-%s", pvcName)
		cfsVolName := getParamWithDefault(volumeContext, KeyCfsVolName, volumeID)
		confFilePath := filepath.Join(DefaultClientConfPath, cfsVolName+JsonFileSuffix)

		if err := ns.Driver.ClientSet.CoreV1().Pods(pvcNamespace).Delete(ctx, podName, metav1.DeleteOptions{}); err != nil && !k8serrors.IsNotFound(err) {
			klog.Warningf("delete Pod fail, %s/%s, volumeID: %s, err: %v", pvcNamespace, podName, volumeID, err)
		}

		if err := os.Remove(confFilePath); err != nil && !os.IsNotExist(err) {
			klog.Warningf("delete confFilePath fail, confFilePath: %s, volumeID: %s, err: %v", confFilePath, volumeID, err)
		}
	}

	klog.Infof("NodeUnstageVolume success, volumeID: %s, stagingPath: %s", volumeID, stagingPath)
	return &csi.NodeUnstageVolumeResponse{}, nil
}

func (ns *nodeServer) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	ns.mutex.Lock()
	defer ns.mutex.Unlock()

	targetPath := req.GetTargetPath()
	volumeID := req.GetVolumeId()

	if volumeID == "" || targetPath == "" {
		return nil, status.Errorf(codes.InvalidArgument, "NodeUnpublishVolume missing parameter, volumeID: %s, targetPath: %s", volumeID, targetPath)
	}

	if err := mount.CleanupMountPoint(targetPath, ns.mounter, false); err != nil {
		return nil, status.Errorf(codes.Internal, "NodeUnpublishVolume fail, targetPath: %s, volumeID: %s, err: %v", targetPath, volumeID, err)
	}

	klog.Infof("NodeUnpublishVolume success, volumeID: %s, targetPath: %s", volumeID, targetPath)
	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (ns *nodeServer) NodeGetInfo(ctx context.Context, req *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	if ns.Driver.NodeID == "" {
		return nil, status.Errorf(codes.Internal, "NodeID not initialized")
	}
	return &csi.NodeGetInfoResponse{NodeId: ns.Driver.NodeID}, nil
}

func (ns *nodeServer) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	return &csi.NodeGetCapabilitiesResponse{
		Capabilities: []*csi.NodeServiceCapability{
			{Type: &csi.NodeServiceCapability_Rpc{Rpc: &csi.NodeServiceCapability_RPC{Type: csi.NodeServiceCapability_RPC_STAGE_UNSTAGE_VOLUME}}},
			{Type: &csi.NodeServiceCapability_Rpc{Rpc: &csi.NodeServiceCapability_RPC{Type: csi.NodeServiceCapability_RPC_GET_VOLUME_STATS}}},
		},
	}, nil
}

func (ns *nodeServer) NodeGetVolumeStats(ctx context.Context, req *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	volumeID := req.GetVolumeId()
	volumePath := req.GetVolumePath()

	if volumeID == "" || volumePath == "" {
		return nil, status.Errorf(codes.InvalidArgument, "NodeGetVolumeStats missing parameter, volumeID: %s, volumePath: %s", volumeID, volumePath)
	}

	isMount, err := isMountPoint(volumePath)
	if err != nil || !isMount {
		return nil, status.Errorf(codes.InvalidArgument, "volumePath is not a mountpoint, volumeID: %s, volumePath: %s, err: %v", volumeID, volumePath, err)
	}

	statfs := &unix.Statfs_t{}
	if err := unix.Statfs(volumePath, statfs); err != nil {
		return nil, status.Errorf(codes.Internal, "unix.Statfs fail, volumeID: %s, err: %v", volumeID, err)
	}

	totalBytes := int64(statfs.Blocks) * int64(statfs.Bsize)
	availableBytes := int64(statfs.Bavail) * int64(statfs.Bsize)
	usedBytes := (int64(statfs.Blocks) - int64(statfs.Bfree)) * int64(statfs.Bsize)

	totalInodes := int64(statfs.Files)
	availableInodes := int64(statfs.Ffree)
	usedInodes := totalInodes - availableInodes

	return &csi.NodeGetVolumeStatsResponse{
		Usage: []*csi.VolumeUsage{
			{Unit: csi.VolumeUsage_BYTES, Total: totalBytes, Used: usedBytes, Available: availableBytes},
			{Unit: csi.VolumeUsage_INODES, Total: totalInodes, Used: usedInodes, Available: availableInodes},
		},
	}, nil
}

// ------------------------------ 受损卷重挂载 ------------------------------
// appendPodUnique 给PV添加唯一Pod
func (p *persistentVolumeWithPods) appendPodUnique(newPod *v1.Pod) {
	for _, pod := range p.pods {
		if pod.UID == newPod.UID {
			return
		}
	}
	p.pods = append(p.pods, newPod)
}

// getAttachedPVWithPods 获取节点上已挂载PV及关联Pod
func (ns *nodeServer) getAttachedPVWithPods() ([]*persistentVolumeWithPods, error) {
	vaList, err := ns.Driver.ClientSet.StorageV1().VolumeAttachments().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("query VolumeAttachments fail, err: %v", err)
	}

	nodePVNames := make(map[string]struct{})
	for _, va := range vaList.Items {
		if va.Spec.NodeName == ns.Driver.NodeID && va.Spec.Attacher == "cubefs.csi.k8s.io" && va.Status.Attached && va.Spec.Source.PersistentVolumeName != nil {
			nodePVNames[*va.Spec.Source.PersistentVolumeName] = struct{}{}
		}
	}

	pvList, err := ns.Driver.ClientSet.CoreV1().PersistentVolumes().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("query PV fail, err: %v", err)
	}

	pvcToPV := make(map[string]*persistentVolumeWithPods)
	for i := range pvList.Items {
		pv := &pvList.Items[i]
		if _, exists := nodePVNames[pv.Name]; exists && pv.Spec.ClaimRef != nil {
			pvcKey := fmt.Sprintf("%s/%s", pv.Spec.ClaimRef.Namespace, pv.Spec.ClaimRef.Name)
			pvcToPV[pvcKey] = &persistentVolumeWithPods{PersistentVolume: pv}
		}
	}

	pods, err := ns.Driver.ClientSet.CoreV1().Pods("").List(context.Background(), metav1.ListOptions{
		FieldSelector: fmt.Sprintf("spec.nodeName=%s", ns.Driver.NodeID),
	})
	if err != nil {
		return nil, fmt.Errorf("query node Pod fail, node: %s, err: %v", ns.Driver.NodeID, err)
	}
	for i := range pods.Items {
		pod := &pods.Items[i]
		if pod.Status.Phase != v1.PodRunning {
			continue
		}
		for _, vol := range pod.Spec.Volumes {
			if vol.PersistentVolumeClaim == nil {
				continue
			}
			pvcKey := fmt.Sprintf("%s/%s", pod.Namespace, vol.PersistentVolumeClaim.ClaimName)
			if pvWithPods, exists := pvcToPV[pvcKey]; exists {
				pvWithPods.appendPodUnique(pod)
			}
		}
	}

	result := make([]*persistentVolumeWithPods, 0, len(pvcToPV))
	for _, pvWithPods := range pvcToPV {
		if len(pvWithPods.pods) > 0 {
			result = append(result, pvWithPods)
		}
	}

	return result, nil
}

// remountDamagedVolumes 重挂载受损卷
func (ns *nodeServer) remountDamagedVolumes() {
	start := time.Now()
	klog.Infof("remountDamagedVolumes. node: %s", ns.Driver.NodeID)

	pvWithPodsList, err := ns.getAttachedPVWithPods()
	if err != nil {
		klog.Warningf("getAttachedPVWithPods fail, err: %v", err)
		return
	}
	if len(pvWithPodsList) == 0 {
		klog.Infof("PV without associated Pod, no need to remount. node: %s", ns.Driver.NodeID)
		return
	}

	// 并发重挂载
	var wg sync.WaitGroup
	wg.Add(len(pvWithPodsList))
	for _, pvWithPods := range pvWithPodsList {
		go func(pvp *persistentVolumeWithPods) {
			defer wg.Done()

			pv := pvp.PersistentVolume
			if pv.Spec.CSI == nil {
				klog.Warningf("PV without CSI configuration: %s, skip", pv.Name)
				return
			}
			volumeID := pv.Spec.CSI.VolumeHandle
			volumeContext := pv.Spec.CSI.VolumeAttributes
			if pv.Spec.ClaimRef != nil {
				volumeContext[KeyCSIPVCName] = pv.Spec.ClaimRef.Name
				volumeContext[KeyCSIPVCNamespace] = pv.Spec.ClaimRef.Namespace
			}

			// 重挂载Global Mount
			globalMountPath := filepath.Join(ns.KubeletRootDir, fmt.Sprintf("plugins/kubernetes.io/csi/pv/%s/globalmount", pv.Name))
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			if err := mount.CleanupMountPoint(globalMountPath, ns.mounter, false); err != nil {
				klog.Warningf("mount.CleanupMountPoint fail, globalMountPath: %s, PV: %s, err: %v", globalMountPath, pv.Name, err)
			}
			if err := ns.mount(ctx, globalMountPath, volumeID, volumeContext); err != nil {
				klog.Warningf("remount fail, globalMountPath: %s, PV: %s, err: %v", globalMountPath, pv.Name, err)
				return
			}

			// 重挂载Pod挂载点
			readOnly := parseBool(getParamWithDefault(volumeContext, KeyCfsReadOnly, "false"))
			for _, pod := range pvp.pods {
				podMountPath := filepath.Join(ns.KubeletRootDir, "pods", string(pod.UID), fmt.Sprintf("volumes/kubernetes.io~csi/%s/mount", pv.Name))
				if err := mount.CleanupMountPoint(podMountPath, ns.mounter, false); err != nil {
					klog.Warningf("mount.CleanupMountPoint fail, podMountPath: %s, Pod: %s/%s, err: %v", podMountPath, pod.Namespace, pod.Name, err)
					continue
				}
				if err := createMountPoint(podMountPath); err != nil {
					klog.Warningf("createMountPoint fail, podMountPath: %s, Pod: %s/%s, err: %v", podMountPath, pod.Namespace, pod.Name, err)
					continue
				}
				if err := bindMount(globalMountPath, podMountPath, readOnly); err != nil {
					klog.Warningf("bind mount fail, %s->%s, Pod: %s/%s, err: %v", globalMountPath, podMountPath, pod.Namespace, pod.Name, err)
					continue
				}
				klog.Infof("remount Pod mountpoint success, podMountPath: %s, Pod: %s/%s", podMountPath, pod.Namespace, pod.Name)
			}
		}(pvWithPods)
	}

	wg.Wait()
	klog.Infof("remountDamagedVolumes success. node: %s, len(PV): %d, cost: %v", ns.Driver.NodeID, len(pvWithPodsList), time.Since(start))
}
