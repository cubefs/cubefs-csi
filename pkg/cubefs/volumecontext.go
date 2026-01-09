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
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/klog/v2"
)

const (
	KeyCfsMaster      = "csi.cubefs/master"       // Cubefs Master地址(必填)
	KeyCfsVolName     = "csi.cubefs/volume-name"  // Cubefs卷名(可选, 默认PVC名称)
	KeyCfsOwner       = "csi.cubefs/owner"        // Cubefs卷所有者(必填)
	KeyCfsAccessKey   = "csi.cubefs/access-key"   // Cubefs AK
	KeyCfsSecretKey   = "csi.cubefs/secret-key"   // Cubefs SK
	KeyCfsLogLevel    = "csi.cubefs/log-level"    // 日志级别(默认warn)
	KeyCfsReadOnly    = "csi.cubefs/read-only"    // 只读模式开关(默认false)
	KeyCfsConsulAddr  = "csi.cubefs/consul-addr"  // Consul地址配置
	KeyCfsVolType     = "csi.cubefs/vol-type"     // 卷类型(默认0)
	KeyCfsZoneName    = "csi.cubefs/zone-name"    // Zone分区(默认default)
	KeyCfsCrossZone   = "csi.cubefs/cross-zone"   // 跨Zone配置(默认false)
	KeyCfsEnableToken = "csi.cubefs/enable-token" // Token认证开关(默认false)
)

// 由provisioner启用--extra-create-metadata后自动注入
const (
	KeyCSIPVCName      = "csi.storage.k8s.io/pvc/name"      // PVC名称
	KeyCSIPVCNamespace = "csi.storage.k8s.io/pvc/namespace" // PVC命名空间
	KeyCSIPVName       = "csi.storage.k8s.io/pv/name"       // PV名称
)

const (
	KeyPodMountEnable = "csi.client-pod/enabled"     // Pod挂载模式开关(true/false)
	KeyPodClientImage = "csi.client-pod/image"       // Pod模式客户端镜像(Pod挂载时必填)
	KeyPodCPURequest  = "csi.client-pod/cpu-request" // Client Pod CPU 请求
	KeyPodCPULimit    = "csi.client-pod/cpu-limit"   // Client Pod CPU 限制
	KeyPodMemRequest  = "csi.client-pod/mem-request" // Client Pod 内存请求
	KeyPodMemLimit    = "csi.client-pod/mem-limit"   // Client Pod 内存限制
)

const (
	DefaultPodCPURequest = "2"   // 默认CPU请求
	DefaultPodCPULimit   = "4"   // 默认CPU限制
	DefaultPodMemRequest = "2Gi" // 默认内存请求
	DefaultPodMemLimit   = "4Gi" // 默认内存限制
)

type DefaultVolumeContextItem struct {
	DefaultValue string
	BoolString   bool
}

var DefaultVolumeContext map[string]DefaultVolumeContextItem = map[string]DefaultVolumeContextItem{
	KeyCfsMaster:       {"", false},
	KeyCfsVolName:      {"", false},
	KeyCfsOwner:        {"", false},
	KeyCfsAccessKey:    {"", false},
	KeyCfsSecretKey:    {"", false},
	KeyCfsLogLevel:     {"warn", false},
	KeyCfsReadOnly:     {"false", true},
	KeyCfsConsulAddr:   {"", false},
	KeyCfsVolType:      {"0", false},
	KeyCfsZoneName:     {"default", false},
	KeyCfsCrossZone:    {"false", true},
	KeyCfsEnableToken:  {"false", true},
	KeyCSIPVCName:      {"", false},
	KeyCSIPVCNamespace: {"default", false},
	KeyCSIPVName:       {"", false},
	KeyPodMountEnable:  {"false", true},
	KeyPodClientImage:  {"", false},
	KeyPodCPURequest:   {DefaultPodCPURequest, false},
	KeyPodCPULimit:     {DefaultPodCPULimit, false},
	KeyPodMemRequest:   {DefaultPodMemRequest, false},
	KeyPodMemLimit:     {DefaultPodMemLimit, false},
}

func SetDefaultVolumeName(volumeName string) {
	item := DefaultVolumeContext[KeyCfsVolName]
	item.DefaultValue = volumeName
	DefaultVolumeContext[KeyCfsVolName] = item
}

func PickupVolumeContext(volumeContext map[string]string) map[string]string {
	pickedMap := make(map[string]string)
	for key, item := range DefaultVolumeContext {
		value := getParamWithDefault(volumeContext, key, item.DefaultValue)
		if item.BoolString {
			value = parseBoolToString(value, item.DefaultValue)
		}
		pickedMap[key] = value
	}
	return pickedMap
}

func getParamWithDefault(volumeContext map[string]string, key, defaultValue string) string {
	val, ok := volumeContext[key]
	if !ok {
		return defaultValue
	}
	val = strings.TrimSpace(val)
	if val == "" {
		return defaultValue
	}
	return val
}

func parseBoolToString(val string, defaultVal string) string {
	val = strings.TrimSpace(strings.ToLower(val))
	if val == "true" || val == "false" {
		return val
	}
	return defaultVal
}

// 解析Pod资源配置
func ParsePodResource(volumeContext map[string]string) (req corev1.ResourceList, limit corev1.ResourceList) {
	// 解析CPU配置
	cpuReq := getParamWithDefault(volumeContext, KeyPodCPURequest, DefaultPodCPURequest)
	cpuLimit := getParamWithDefault(volumeContext, KeyPodCPULimit, DefaultPodCPULimit)
	// 解析内存配置
	memReq := getParamWithDefault(volumeContext, KeyPodMemRequest, DefaultPodMemRequest)
	memLimit := getParamWithDefault(volumeContext, KeyPodMemLimit, DefaultPodMemLimit)

	// 构建资源列表（处理解析错误）
	req = corev1.ResourceList{}
	if cpu, err := resource.ParseQuantity(cpuReq); err == nil {
		req[corev1.ResourceCPU] = cpu
	} else {
		klog.Warningf("invalid CPU Request: %s, using default value: %s", cpuReq, DefaultPodCPURequest)
		req[corev1.ResourceCPU] = resource.MustParse(DefaultPodCPURequest)
	}
	if mem, err := resource.ParseQuantity(memReq); err == nil {
		req[corev1.ResourceMemory] = mem
	} else {
		klog.Warningf("invalid Memory Request: %s, using default value: %s", memReq, DefaultPodMemRequest)
		req[corev1.ResourceMemory] = resource.MustParse(DefaultPodMemRequest)
	}

	limit = corev1.ResourceList{}
	if cpu, err := resource.ParseQuantity(cpuLimit); err == nil {
		limit[corev1.ResourceCPU] = cpu
	} else {
		klog.Warningf("invalid CPU Limit: %s, using default value: %s", cpuLimit, DefaultPodCPULimit)
		limit[corev1.ResourceCPU] = resource.MustParse(DefaultPodCPULimit)
	}
	if mem, err := resource.ParseQuantity(memLimit); err == nil {
		limit[corev1.ResourceMemory] = mem
	} else {
		klog.Warningf("invalid Memory Limit: %s, using default value: %s", memLimit, DefaultPodMemLimit)
		limit[corev1.ResourceMemory] = resource.MustParse(DefaultPodMemLimit)
	}

	return req, limit
}
