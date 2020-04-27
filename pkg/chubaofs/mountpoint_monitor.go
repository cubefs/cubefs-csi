package chubaofs

import (
	"fmt"
	csicommon "github.com/chubaofs/chubaofs-csi/pkg/csi-common"
	"github.com/golang/glog"
	"k8s.io/utils/mount"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	MountPointRegexpDefault       = `^.*/([a-z0-9-]+)/volumes/kubernetes.io~csi/([a-z0-9-]+)/mount$`
	GlobalMountPointRegexpDefault = `^.*/csi/pv/([a-z0-9-]+)/globalmount$`
	FSNamePrefix                  = "chubaofs-"
)

var (
	globalMountPointRegexp = regexp.MustCompile(GlobalMountPointRegexpDefault)
	mountPointRegexp       = regexp.MustCompile(MountPointRegexpDefault)
)

type MountPointMonitor struct {
	driver *csicommon.CSIDriver
	mutex  *sync.RWMutex
}

func NewMountPointerMonitor(driver *csicommon.CSIDriver, mutex *sync.RWMutex) *MountPointMonitor {
	return &MountPointMonitor{
		driver: driver,
		mutex:  mutex,
	}
}

type CfsMountPoint struct {
	device             string
	volName            string
	path               string
	pvName             string
	podUid             string
	isGlobalMountPoint bool
	isCorruptedMnt     bool
}

func (mp *CfsMountPoint) checkMountPoint() {
	_, err := os.Stat(mp.path)
	if err != nil {
		if mount.IsCorruptedMnt(err) {
			// error: "Transport endpoint is not connected"
			mp.isCorruptedMnt = true
		} else {
			// other unknown error
			glog.Errorf("os.Stat fail, path:%v error:%v", mp.path, err)
		}
	}
}

// check invalid mount point, restart cfs-client if pod is running, umount if pod terminated
func (mpMonitor *MountPointMonitor) checkInvalidMountPointPeriod() {
	time.Sleep(60 * time.Second)
	glog.Info("invalid mount point monitor started")
	for true {
		//glog.Info("checking invalid mount point")
		mpMonitor.checkInvalidMountPoint()
		time.Sleep(10 * time.Second)
	}
}

func (mpMonitor *MountPointMonitor) checkInvalidMountPoint() {
	cfsMountPointMap, err := getCfsMountPointMap()
	if err != nil {
		glog.Errorf("getCfsMountPointMap fail, err:%v", err)
		return
	}

	for _, v := range cfsMountPointMap {
		mpMonitor.mutex.Lock()
		mpMonitor.checkAndUMountInvalidCfsMountPointList(v)
		mpMonitor.mutex.Unlock()
	}
}

// Only deal with the state that cfs-client is killed. Ignore user umount and stop cfs-client is not considered
func (mpMonitor *MountPointMonitor) checkAndUMountInvalidCfsMountPointList(mountPointList []*CfsMountPoint) {
	var globalMountPoint *CfsMountPoint
	var invalidMountPointList []*CfsMountPoint
	for _, mountPoint := range mountPointList {
		if mountPoint.isGlobalMountPoint {
			mountPoint.checkMountPoint()
			if mountPoint.isCorruptedMnt {
				globalMountPoint = mountPoint
			}
		} else {
			invalidMountPointList = append(invalidMountPointList, mountPoint)
		}
	}

	if globalMountPoint != nil {
		for _, mountPoint := range mountPointList {
			mpMonitor.uMountPoint(mountPoint)
		}

		configFilePath := fmt.Sprintf("/cfs/conf/%v.json", globalMountPoint.volName)
		glog.Infof("GlobalMountPoint:%v is invalid mount point, restart cfs-client process and remount, configFilePath:%v", globalMountPoint, configFilePath)
		err := mountVolume(configFilePath)
		if err != nil {
			glog.Errorf("start cfs-client process fail, GlobalMountPoint:%v configFilePath:%v error: %v", globalMountPoint, configFilePath, err)
			return
		}

		glog.Infof("cfs-client process already started, configFilePath:%v", configFilePath)
		for _, mountPoint := range invalidMountPointList {
			err := bindMount(globalMountPoint.path, mountPoint.path)
			if err != nil {
				glog.Errorf("mount bind fail, stagingTargetPath:%v targetPath:%v error: %v", globalMountPoint.path, mountPoint.path, err)
			} else {
				glog.Errorf("mount bind success. stagingTargetPath:%v targetPath:%v", globalMountPoint.path, mountPoint.path)
			}
		}
	}
}

func (mpMonitor *MountPointMonitor) uMountPoint(mountPoint *CfsMountPoint) {
	path := mountPoint.path
	glog.Warningf("umount path:%v", path)
	err := umountVolume(path)
	if err != nil {
		glog.Errorf("umount path[%v] fail, error:%v", path, err)
		return
	}
}

// mount point path:
// /var/lib/kubelet/pods/9cda187a-7fb3-11ea-80b3-246e968d4b38/volumes/kubernetes.io~csi/pvc-9ae9405c-7fb3-11ea-80b3-246e968d4b38/mount
// /var/lib/kubelet/plugins/kubernetes.io/csi/pv/pvc-9ae9405c-7fb3-11ea-80b3-246e968d4b38/globalmount
func (mpMonitor *MountPointMonitor) checkAndUMountInvalidCfsMountPoint(mountPoint *CfsMountPoint) {
	path := mountPoint.path
	_, err := os.Stat(path)
	if err != nil {
		if mount.IsCorruptedMnt(err) {
			glog.Warningf("MountPoint is invalid, umount path:%v", path)
			err := umountVolume(path)
			if err != nil {
				glog.Errorf("umount path[%v] fail, error:%v", path, err)
				return
			}

			_ = os.Remove(getParentDirectory(path))
		} else {
			glog.Errorf("MountPoint stat error: %v", err)
		}
	}
}

func getCfsMountPointMap() (map[string][]*CfsMountPoint, error) {
	mountList, err := listMount()
	if err != nil {
		return nil, err
	}

	return convertToCfsMountPointMap(mountList)
}

func convertToCfsMountPointMap(mountList []mount.MountPoint) (map[string][]*CfsMountPoint, error) {
	var mountPointMap = make(map[string][]*CfsMountPoint)
	for _, mountPoint := range mountList {
		device := mountPoint.Device
		hasPrefix := strings.HasPrefix(device, FSNamePrefix)
		if !hasPrefix {
			continue
		}

		mp := convertToCfsMountPoint(mountPoint)
		if mp == nil {
			glog.Infof("convertToCfsMountPoint fail,  mountPoint:%v", mountPoint)
			continue
		}

		mountPointMap[mp.volName] = append(mountPointMap[mp.volName], mp)
	}

	return mountPointMap, nil
}

func convertToCfsMountPoint(mountPoint mount.MountPoint) *CfsMountPoint {
	device := mountPoint.Device
	volumeName := device[len(FSNamePrefix):len(device)]
	path := mountPoint.Path
	// /var/lib/kubelet/plugins/kubernetes.io/csi/pv/pvc-9ae9405c-7fb3-11ea-80b3-246e968d4b38/globalmount
	if strings.HasSuffix(path, "/globalmount") {
		params := globalMountPointRegexp.FindStringSubmatch(path)
		if len(params) != 2 {
			return nil
		}

		return &CfsMountPoint{
			device:             device,
			volName:            volumeName,
			path:               path,
			pvName:             params[1],
			podUid:             "",
			isGlobalMountPoint: true,
			isCorruptedMnt:     false,
		}
	} else if strings.HasSuffix(path, "/mount") {
		// /var/lib/kubelet/pods/9cda187a-7fb3-11ea-80b3-246e968d4b38/volumes/kubernetes.io~csi/pvc-9ae9405c-7fb3-11ea-80b3-246e968d4b38/mount
		params := mountPointRegexp.FindStringSubmatch(path)
		if len(params) != 3 {
			return nil
		}

		return &CfsMountPoint{
			device:             device,
			volName:            volumeName,
			path:               path,
			pvName:             params[2],
			podUid:             params[1],
			isGlobalMountPoint: false,
			isCorruptedMnt:     false,
		}
	} else {
		return nil
	}
}
