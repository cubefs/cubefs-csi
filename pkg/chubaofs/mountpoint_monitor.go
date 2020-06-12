package chubaofs

import (
	"encoding/json"
	"fmt"
	csicommon "github.com/chubaofs/chubaofs-csi/pkg/csi-common"
	"github.com/golang/glog"
	"io/ioutil"
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
}

func IsCorruptedMnt(mp *CfsMountPoint) bool {
	_, err := os.Stat(mp.path)
	if err != nil {
		if mount.IsCorruptedMnt(err) {
			// error: "Transport endpoint is not connected"
			return true
		} else {
			// other unknown error
			glog.Errorf("os.Stat fail, path:%v error:%v", mp.path, err)
		}
	}

	return false
}

// check invalid mount point, restart cfs-client if pod is running, umount if pod terminated
func (mpMonitor *MountPointMonitor) checkInvalidMountPointPeriod() {
	glog.Info("invalid mount point monitor started")
	for {
		time.Sleep(10 * time.Second)
		//glog.Info("checking invalid mount point")
		mpMonitor.checkInvalidMountPoint()
	}
}

func (mpMonitor *MountPointMonitor) checkInvalidMountPoint() {
	cfsMountPointMap, err := GetCfsMountPointMap()
	if err != nil {
		glog.Errorf("GetCfsMountPointMap fail, err:%v", err)
		return
	}

	for _, v := range cfsMountPointMap {
		mpMonitor.mutex.Lock()
		mpMonitor.reMountInvalidVolume(v)
		mpMonitor.mutex.Unlock()
	}
}

// Only deal with the state that cfs-client is killed. Ignore user umount and stop cfs-client is not considered
func (mpMonitor *MountPointMonitor) reMountInvalidVolume(mountPointList []*CfsMountPoint) {
	if mountPointList == nil || len(mountPointList) == 0 {
		return
	}

	var globalMountPoint *CfsMountPoint
	var subMountPoints = make([]*CfsMountPoint, 0)
	var hasCorruptedMnt = false

	// check if globalMountPoint exists
	for _, mountPoint := range mountPointList {
		if mountPoint.isGlobalMountPoint {
			// contain globalMountPoint
			globalMountPoint = mountPoint
		} else {
			subMountPoints = append(subMountPoints, mountPoint)
		}

		if IsCorruptedMnt(mountPoint) {
			hasCorruptedMnt = true
		}
	}

	if !hasCorruptedMnt || len(subMountPoints) == 0 {
		// all MountPoint is OK, or no sub MountPoint
		return
	}

	if globalMountPoint == nil || IsCorruptedMnt(globalMountPoint) {
		glog.Info("globalMountPoint not work")
		for _, mountPoint := range mountPointList {
			mountPoint.UMountPoint()
		}

		configFilePath := fmt.Sprintf("/cfs/conf/%v.json", subMountPoints[0].volName)
		file, err := ioutil.ReadFile(configFilePath)
		if err != nil {
			glog.Errorf("config file[%s] not found, MountPoint[%v] cannot remount", configFilePath, subMountPoints)
			return
		}

		glog.Infof("restart cfs-client process and remount, configFilePath:%v", configFilePath)
		err = mountVolume(configFilePath)
		if err != nil {
			glog.Errorf("start cfs-client process fail, configFilePath:%v error: %v", configFilePath, err)
			return
		}

		glog.Infof("cfs-client process already started, configFilePath:%s", configFilePath)

		clientConf := &cfsClientConf{}
		err = json.Unmarshal(file, clientConf)
		for _, mountPoint := range subMountPoints {
			err := bindMount(clientConf.MountPoint, mountPoint.path)
			if err != nil {
				glog.Errorf("mount bind fail, stagingTargetPath:%v targetPath:%v error: %v", clientConf.MountPoint, mountPoint.path, err)
			} else {
				glog.Errorf("mount bind success. stagingTargetPath:%v targetPath:%v", clientConf.MountPoint, mountPoint.path)
			}
		}

	} else {
		glog.Info("globalMountPoint exists")
		for _, mountPoint := range subMountPoints {
			if !IsCorruptedMnt(mountPoint) {
				continue
			}

			// sub MountPoint remount
			mountPoint.UMountPoint()
			err := bindMount(globalMountPoint.path, mountPoint.path)
			if err != nil {
				glog.Errorf("mount bind fail, stagingTargetPath:%v targetPath:%v error: %v", globalMountPoint.path, mountPoint.path, err)
			} else {
				glog.Errorf("mount bind success. stagingTargetPath:%v targetPath:%v", globalMountPoint.path, mountPoint.path)
			}
		}
	}
}

func (mountPoint *CfsMountPoint) UMountPoint() {
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

			_ = CleanPath(path)
		} else {
			glog.Errorf("MountPoint stat error: %v", err)
		}
	}
}

func GetCfsMountPointMap() (map[string][]*CfsMountPoint, error) {
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
		}
	} else {
		return nil
	}
}
