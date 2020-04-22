package chubaofs

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"k8s.io/utils/mount"
	"testing"
)

func Test_convertToCfsMountPoint1(t *testing.T) {
	mountPoint := mount.MountPoint{
		Device: "chubaofs-share_volume",
		Path:   "/var/lib/kubelet/plugins/kubernetes.io/csi/pv/pvc-3ce3451a-82b3-11ea-80b3-246e968d4b38/globalmount",
		Type:   "fuse",
	}

	mp := convertToCfsMountPoint(mountPoint)
	fmt.Println(fmt.Sprintf("%v", mp))
	assert.NotNil(t, mp, "convert to CfsMountPoint fail")
	assert.True(t, mp.isGlobalMountPoint, "mp.isGlobalMountPoint is not true")
	assert.Equal(t, "share_volume", mp.volName, "volName not equals")
	assert.Equal(t, "pvc-3ce3451a-82b3-11ea-80b3-246e968d4b38", mp.pvName, "pvName not equals")
}

func Test_convertToCfsMountPoint2(t *testing.T) {
	mountPoint := mount.MountPoint{
		Device: "chubaofs-share_volume",
		Path:   "/var/lib/kubelet/pods/4054c1bb-82b3-11ea-80b3-246e968d4b38/volumes/kubernetes.io~csi/pvc-3ce3451a-82b3-11ea-80b3-246e968d4b38/mount",
		Type:   "fuse",
	}

	mp := convertToCfsMountPoint(mountPoint)
	fmt.Println(fmt.Sprintf("%v", mp))
	assert.NotNil(t, mp, "convert to CfsMountPoint fail")
	assert.False(t, mp.isGlobalMountPoint, "mp.isGlobalMountPoint is not false")
	assert.Equal(t, "share_volume", mp.volName, "volName not equals")
	assert.Equal(t, "4054c1bb-82b3-11ea-80b3-246e968d4b38", mp.podUid, "Pod uid not equals")
	assert.Equal(t, "pvc-3ce3451a-82b3-11ea-80b3-246e968d4b38", mp.pvName, "pvName not equals")
}

func Test_convertToCfsMountPoint3(t *testing.T) {
	mountPoint := mount.MountPoint{
		Device: "chubaofs-pvc-9ae9405c-7fb3-11ea-80b3-246e968d4b38",
		Path:   "/var/lib/kubelet/plugins/kubernetes.io/csi/pv/pvc-9ae9405c-7fb3-11ea-80b3-246e968d4b38/globalmount",
		Type:   "fuse",
	}

	mp := convertToCfsMountPoint(mountPoint)
	fmt.Println(fmt.Sprintf("%v", mp))
	assert.NotNil(t, mp, "convert to CfsMountPoint fail")
	assert.True(t, mp.isGlobalMountPoint, "mp.isGlobalMountPoint is not true")
	assert.Equal(t, "pvc-9ae9405c-7fb3-11ea-80b3-246e968d4b38", mp.volName, "volName not equals")
	assert.Equal(t, "pvc-9ae9405c-7fb3-11ea-80b3-246e968d4b38", mp.pvName, "pvName not equals")
}

func Test_convertToCfsMountPoint4(t *testing.T) {
	mountPoint := mount.MountPoint{
		Device: "chubaofs-pvc-9ae9405c-7fb3-11ea-80b3-246e968d4b38",
		Path:   "/var/lib/kubelet/pods/68affac4-82b3-11ea-80b3-246e968d4b38/volumes/kubernetes.io~csi/pvc-9ae9405c-7fb3-11ea-80b3-246e968d4b38/mount",
		Type:   "fuse",
	}

	mp := convertToCfsMountPoint(mountPoint)
	fmt.Println(fmt.Sprintf("%v", mp))
	assert.NotNil(t, mp, "convert to CfsMountPoint fail")
	assert.False(t, mp.isGlobalMountPoint, "mp.isGlobalMountPoint is not false")
	assert.Equal(t, "pvc-9ae9405c-7fb3-11ea-80b3-246e968d4b38", mp.volName, "volName not equals")
	assert.Equal(t, "68affac4-82b3-11ea-80b3-246e968d4b38", mp.podUid, "Pod uid not equals")
	assert.Equal(t, "pvc-9ae9405c-7fb3-11ea-80b3-246e968d4b38", mp.pvName, "pvName not equals")
}

func Test_convertToCfsMountPointMap(t *testing.T) {
	list := []mount.MountPoint{
		{
			Device: "chubaofs-share_volume",
			Path:   "/var/lib/kubelet/pods/4054c1bb-82b3-11ea-80b3-246e968d4b38/volumes/kubernetes.io~csi/pvc-3ce3451a-82b3-11ea-80b3-246e968d4b38/mount",
			Type:   "fuse",
		},
		{
			Device: "chubaofs-share_volume",
			Path:   "/var/lib/kubelet/pods/4054c1bb-82b3-11ea-80b3-246e968d4b38/volumes/kubernetes.io~csi/pvc-3ce3451a-82b3-11ea-80b3-246e968d4b38/mount",
			Type:   "fuse",
		},
		{
			Device: "chubaofs-share_volume",
			Path:   "/var/lib/kubelet/pods/4054c1bb-82b3-11ea-80b3-246e968d4b38/volumes/kubernetes.io~csi/pvc-3ce3451a-82b3-11ea-80b3-246e968d4b38/mount",
			Type:   "fuse",
		},
		{
			Device: "chubaofs-pvc-9ae9405c-7fb3-11ea-80b3-246e968d4b38",
			Path:   "/var/lib/kubelet/plugins/kubernetes.io/csi/pv/pvc-9ae9405c-7fb3-11ea-80b3-246e968d4b38/globalmount",
			Type:   "fuse",
		},
		{
			Device: "chubaofs-pvc-9ae9405c-7fb3-11ea-80b3-246e968d4b38",
			Path:   "/var/lib/kubelet/pods/68affac4-82b3-11ea-80b3-246e968d4b38/volumes/kubernetes.io~csi/pvc-9ae9405c-7fb3-11ea-80b3-246e968d4b38/mount",
			Type:   "fuse",
		},
		{
			Device: "chubaofs-pvc-9ae9405c-7fb3-11ea-80b3-246e968d4b38",
			Path:   "/var/lib/kubelet/pods/fc93a903-82dd-11ea-80b3-246e968d4b38/volumes/kubernetes.io~csi/pvc-9ae9405c-7fb3-11ea-80b3-246e968d4b38/mount",
			Type:   "fuse",
		},
	}
	mountPointMap, err := convertToCfsMountPointMap(list)
	assert.Nil(t, err, "convertToCfsMountPointMap fail")
	assert.True(t, len(mountPointMap) == 2, "convertToCfsMountPointMap count not rigth")
}
