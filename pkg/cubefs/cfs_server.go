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
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	csicommon "github.com/cubefs/cubefs-csi/pkg/csi-common"
	"github.com/golang/glog"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	KVolumeName       = "volName"
	KMasterAddr       = "masterAddr"
	KLogLevel         = "logLevel"
	KLogDir           = "logDir"
	KOwner            = "owner"
	KMountPoint       = "mountPoint"
	KExporterPort     = "exporterPort"
	KProfPort         = "profPort"
	KCrossZone        = "crossZone"
	KEnableToken      = "enableToken"
	KZoneName         = "zoneName"
	KConsulAddr       = "consulAddr"
	KVolType          = "volType"
	KMpCount          = "mpCount"
	KDpCount          = "dpCount"
	KDpSize           = "dpSize"
	KReplicaNum       = "replicaNum"
	KEnablePosixAcl   = "enablePosixAcl"
	KFollowerRead     = "followerRead"
	KNormalZonesFirst = "normalZonesFirst"
	KCacheRuleKey     = "cacheRuleKey"
	KEbsBlkSize       = "ebsBlkSize"
	KCacheCap         = "cacheCap"
	KCacheAction      = "cacheAction"
	KCacheThreshold   = "cacheThreshold"
	KCacheTTL         = "cacheTTL"
	KCacheHighWater   = "cacheHighWater"
	KCacheLowWater    = "cacheLowWater"
	KCacheLRUInterval = "cacheLRUInterval"
)

const (
	defaultClientConfPath     = "/cfs/conf/"
	defaultLogDir             = "/cfs/logs/"
	defaultExporterPort   int = 9513
	defaultProfPort       int = 10094
	defaultLogLevel           = "info"
	jsonFileSuffix            = ".json"
)

const (
	ErrCodeVolNotExists = 7

	ErrDuplicateVolMsg = "duplicate vol"
)

var createVolParams = [...]string{
	KOwner,
	KCrossZone,
	KEnableToken,
	KZoneName,
	KVolType,
	KMpCount,
	KDpCount,
	KDpSize,
	KReplicaNum,
	KEnablePosixAcl,
	KFollowerRead,
	KNormalZonesFirst,
	KCacheRuleKey,
	KEbsBlkSize,
	KCacheCap,
	KCacheAction,
	KCacheThreshold,
	KCacheTTL,
	KCacheHighWater,
	KCacheLowWater,
	KCacheLRUInterval,
}

type cfsServer struct {
	clientConfFile string
	masterAddrs    []string
	clientConf     map[string]string
}

// Create and Delete Volume Response
type cfsServerResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data string `json:"data,omitempty"`
}

func newCfsServer(volName string, param map[string]string) (cs *cfsServer, err error) {
	if len(volName) == 0 {
		return nil, fmt.Errorf("invalid argument value in volName.")
	}
	if !hasValue(param, KMasterAddr) {
		return nil, fmt.Errorf("master address(es) must be configured")
	}

	finalVolName := getValueWithDefault(param, KVolumeName, volName)
	generatedOwner := csicommon.ShortenString(fmt.Sprintf("csi_%d", time.Now().UnixNano()), 20)
	param[KVolumeName] = finalVolName
	param[KOwner] = getValueWithDefault(param, KOwner, generatedOwner)
	param[KLogLevel] = getValueWithDefault(param, KLogLevel, defaultLogLevel)
	param[KLogDir] = defaultLogDir + finalVolName
	return &cfsServer{
		clientConfFile: defaultClientConfPath + finalVolName + jsonFileSuffix,
		masterAddrs:    strings.Split(param[KMasterAddr], ","),
		clientConf:     param,
	}, err
}

func hasValue(param map[string]string, key string) bool {
	return len(param[key]) > 0
}

func getValueWithDefault(param map[string]string, key string, defaultValue string) string {
	if hasValue(param, key) {
		return param[key]
	}
	return defaultValue
}

func (cs *cfsServer) persistClientConf(mountPoint string) error {
	exporterPort, _ := getFreePort(defaultExporterPort)
	profPort, _ := getFreePort(defaultProfPort)
	cs.clientConf[KMountPoint] = mountPoint
	cs.clientConf[KExporterPort] = strconv.Itoa(exporterPort)
	cs.clientConf[KProfPort] = strconv.Itoa(profPort)
	_ = os.Mkdir(cs.clientConf[KLogDir], 0777)
	clientConfBytes, _ := json.Marshal(cs.clientConf)
	err := os.WriteFile(cs.clientConfFile, clientConfBytes, 0444)
	if err != nil {
		return status.Errorf(codes.Internal, "create client config file fail. err(%s)", err.Error())
	}

	glog.V(0).Infof("create client config file success. volumeName(%s)", cs.clientConf[KVolumeName])
	return nil
}

func (cs *cfsServer) createVolume(capacityGB int64) (err error) {
	return cs.forEachMasterAddr("CreateVolume", func(addr string) error {
		volName := cs.clientConf[KVolumeName]
		params := url.Values{}
		params.Add("name", volName)
		params.Add("capacity", strconv.FormatInt(capacityGB, 10))
		for _, param := range createVolParams {
			if hasValue(cs.clientConf, param) {
				params.Add(param, cs.clientConf[param])
			}
		}
		resp, err := cs.executeRequest(addr, "admin/createVol", params)
		if err != nil {
			return err
		}

		if resp.Code != 0 {
			if strings.Contains(resp.Msg, ErrDuplicateVolMsg) {
				glog.Warningf("duplicate create volume[%s]. msg(%s)", volName, resp.Msg)
				return nil
			}

			return fmt.Errorf("create volume[%s] failed. code(%d), msg(%s)", volName, resp.Code, resp.Msg)
		}

		return nil
	})
}

func (cs *cfsServer) forEachMasterAddr(stage string, f func(addr string) error) (err error) {
	for _, addr := range cs.masterAddrs {
		if err = f(addr); err == nil {
			break
		}

		glog.Warningf("try %s with master[%s] failed. err(%v)", stage, addr, err)
	}

	if err != nil {
		glog.Errorf("%s failed with all masters. err(%v)", stage, err)
		return err
	}

	return nil
}

func (cs *cfsServer) deleteVolume() (err error) {
	ownerMd5, err := cs.getOwnerMd5()
	if err != nil {
		return err
	}

	volName := cs.clientConf[KVolumeName]
	return cs.forEachMasterAddr("DeleteVolume", func(addr string) error {
		params := url.Values{}
		params.Add("name", volName)
		params.Add("authKey", ownerMd5)
		resp, err := cs.executeRequest(addr, "vol/delete", params)
		if err != nil {
			return err
		}

		if resp.Code != 0 {
			if resp.Code == ErrCodeVolNotExists {
				glog.Warningf("volume[%s] does not exist, assuming the volume has already been deleted. code(%d), msg(%s)",
					volName, resp.Code, resp.Msg)
				return nil
			}
			return fmt.Errorf("delete volume[%s] failed. code(%d), msg(%s)", volName, resp.Code, resp.Msg)
		}

		return nil
	})
}

func (cs *cfsServer) executeRequest(host string, path string, params url.Values) (*cfsServerResponse, error) {
	url := fmt.Sprintf("http://%s/%s?%s", host, path, params.Encode())
	glog.Infof("request url: %v", url)
	httpResp, err := http.Get(url)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "request to url[%s] failed. err(%v)", url, err)
	}

	defer httpResp.Body.Close()
	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "reading http response body from url[%s] failed. bodyLen(%d), err(%v)", url, len(body), err)
	}

	resp := &cfsServerResponse{}
	if err := json.Unmarshal(body, resp); err != nil {
		return nil, status.Errorf(codes.Unavailable, "unmarshalling http response body from url[%s] failed. msg(%s), err(%v)", url, resp.Msg, err)
	}
	return resp, nil
}

func (cs *cfsServer) runClient() error {
	return mountVolume(cs.clientConfFile)
}

func (cs *cfsServer) expandVolume(capacityGB int64) (err error) {
	ownerMd5, err := cs.getOwnerMd5()
	if err != nil {
		return err
	}

	volName := cs.clientConf[KVolumeName]

	return cs.forEachMasterAddr("ExpandVolume", func(addr string) error {
		params := url.Values{}
		params.Add("name", volName)
		params.Add("authKey", ownerMd5)
		params.Add("capacity", strconv.FormatInt(capacityGB, 10))
		resp, err := cs.executeRequest(addr, "vol/expand", params)
		if err != nil {
			return err
		}

		if resp.Code != 0 {
			return fmt.Errorf("expand volume[%v] failed. code(%d), msg(%s)", volName, resp.Code, resp.Msg)
		}

		return nil
	})
}

func (cs *cfsServer) getOwnerMd5() (string, error) {
	owner := cs.clientConf[KOwner]
	key := md5.New()
	if _, err := key.Write([]byte(owner)); err != nil {
		return "", status.Errorf(codes.Internal, "calculating owner[%s] md5 failed. err(%v)", owner, err)
	}

	return hex.EncodeToString(key.Sum(nil)), nil
}
