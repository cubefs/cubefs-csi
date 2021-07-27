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
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	csicommon "github.com/chubaofs/chubaofs-csi/pkg/csi-common"
	"github.com/golang/glog"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	KVolumeName    = "volName"
	KMasterAddr    = "masterAddr"
	KLogLevel      = "logLevel"
	KLogDir        = "logDir"
	KOwner         = "owner"
	KMountPoint    = "mountPoint"
	KExporterPort  = "exporterPort"
	KProfPort      = "profPort"
	KCrossZone     = "crossZone"
	KEnableToken   = "enableToken"
	KZoneName      = "zoneName"
	KConsulAddr    = "consulAddr"
)

const (
	defaultClientConfPath     = "/cfs/conf/"
	defaultLogDir             = "/cfs/logs/"
	defaultExporterPort   int = 9513
	defaultProfPort       int = 10094
	defaultLogLevel           = "info"
	jsonFileSuffix            = ".json"
	defaultConsulAddr         = "http://consul-service.chubaofs.svc.cluster.local:8500"
)

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
	masterAddr := param[KMasterAddr]
	if len(volName) == 0 || len(masterAddr) == 0 {
		return nil, fmt.Errorf("invalid argument for initializing cfsServer")
	}

	newVolName := getValueWithDefault(param, KVolumeName, volName)
	clientConfFile := defaultClientConfPath + newVolName + jsonFileSuffix
	newOwner := csicommon.ShortenString(fmt.Sprintf("csi_%d", time.Now().UnixNano()), 20)
	param[KMasterAddr] = masterAddr
	param[KVolumeName] = newVolName
	param[KOwner] = getValueWithDefault(param, KOwner, newOwner)
	param[KLogLevel] = getValueWithDefault(param, KLogLevel, defaultLogLevel)
	param[KLogDir] = defaultLogDir + newVolName
	param[KConsulAddr] = getValueWithDefault(param, KConsulAddr, defaultConsulAddr)
	return &cfsServer{
		clientConfFile: clientConfFile,
		masterAddrs:    strings.Split(masterAddr, ","),
		clientConf:     param,
	}, err
}

func getValueWithDefault(param map[string]string, key string, defaultValue string) string {
	value := param[key]
	if len(value) == 0 {
		value = defaultValue
	}

	return value
}

func (cs *cfsServer) persistClientConf(mountPoint string) error {
	exporterPort, _ := getFreePort(defaultExporterPort)
	profPort, _ := getFreePort(defaultProfPort)
	cs.clientConf[KMountPoint] = mountPoint
	cs.clientConf[KExporterPort] = strconv.Itoa(exporterPort)
	cs.clientConf[KProfPort] = strconv.Itoa(profPort)
	_ = os.Mkdir(cs.clientConf[KLogDir], 0777)
	clientConfBytes, _ := json.Marshal(cs.clientConf)
	err := ioutil.WriteFile(cs.clientConfFile, clientConfBytes, 0444)
	if err != nil {
		return status.Errorf(codes.Internal, "create client config file fail. err: %v", err.Error())
	}

	glog.V(0).Infof("create client config file success, volumeId:%v", cs.clientConf[KVolumeName])
	return nil
}

func (cs *cfsServer) createVolume(capacityGB int64) (err error) {
	valName := cs.clientConf[KVolumeName]
	owner := cs.clientConf[KOwner]
	crossZone := cs.clientConf[KCrossZone]
	token := cs.clientConf[KEnableToken]
	zone := cs.clientConf[KZoneName]
	for _, addr := range cs.masterAddrs {
		url := fmt.Sprintf("http://%s/admin/createVol?name=%s&capacity=%v&owner=%v&crossZone=%v&enableToken=%v&zoneName=%v",
			addr, valName, capacityGB, owner, crossZone, token, zone)
		glog.Infof("createVol url: %v", url)
		resp, err := cs.executeRequest(url)
		if err != nil {
			continue
		}

		if resp.Code != 0 {
			if resp.Code == 1 {
				glog.Warningf("duplicate to create volume. url(%v) code=1 msg:%v", url, resp.Msg)
				err = nil
				break
			} else {
				glog.Errorf("create volume is failed. url(%v) code=(%v), msg:%v", url, resp.Code, resp.Msg)
				err = fmt.Errorf("create volume is failed")
				continue
			}
		} else {
			err = nil
			break
		}
	}

	return
}

func (cs *cfsServer) deleteVolume() (err error) {
	ownerMd5, err := cs.getOwnerMd5()
	if err != nil {
		return err
	}

	valName := cs.clientConf[KVolumeName]
	for _, addr := range cs.masterAddrs {
		url := fmt.Sprintf("http://%s/vol/delete?name=%s&authKey=%v", addr, valName, ownerMd5)
		glog.Infof("deleteVol url: %v", url)
		resp, err := cs.executeRequest(url)
		if err != nil {
			glog.Fatalf("delete volume fail. url:%v error:%v", url, err)
			continue
		}

		if resp.Code != 0 {
			if resp.Code == 7 {
				glog.Warningf("volume[%s] not exists, assuming the volume has already been deleted. code:%v, msg:%v",
					valName, resp.Code, resp.Msg)
				err = nil
				break
			} else {
				glog.Errorf("delete volume[%s] is failed. code:%v, msg:%v", valName, resp.Code, resp.Msg)
				err = fmt.Errorf("delete volume is failed")
				continue
			}
		} else {
			err = nil
			break
		}
	}

	return
}

func (cs *cfsServer) executeRequest(url string) (*cfsServerResponse, error) {
	httpResp, err := http.Get(url)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "request url failed, url(%v) err(%v)", url, err)
	}

	defer httpResp.Body.Close()
	body, err := ioutil.ReadAll(httpResp.Body)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "read http response body, url(%v) bodyLen(%v) err(%v)", url, len(body), err)
	}

	resp := &cfsServerResponse{}
	if err := json.Unmarshal(body, resp); err != nil {
		return nil, status.Errorf(codes.Unavailable, "unmarshal http response body, url(%v) msg(%v) err(%v)", url, resp.Msg, err)
	}
	return resp, nil
}

func (cs *cfsServer) runClient() error {
	return mountVolume(cs.clientConfFile)
}

func (cs *cfsServer) expendVolume(capacityGB int64) (err error) {
	ownerMd5, err := cs.getOwnerMd5()
	if err != nil {
		return err
	}

	volName := cs.clientConf[KVolumeName]
	for _, addr := range cs.masterAddrs {
		url := fmt.Sprintf("http://%s/vol/expand?name=%s&authKey=%v&capacity=%v", addr, volName, ownerMd5, capacityGB)
		glog.Infof("expendVolume url: %v", url)
		resp, err := cs.executeRequest(url)
		if err != nil {
			glog.Fatalf("delete volume[%v] fail. url:%v error:%v", volName, url, err)
			continue
		}

		if resp.Code != 0 {
			glog.Errorf("expend volume[%v] fail. code:%v, msg:%v", volName, resp.Code, resp.Msg)
			err = fmt.Errorf("expend volume[%v] fail", volName)
			continue
		} else {
			break
		}
	}

	return
}

func (cs *cfsServer) getOwnerMd5() (string, error) {
	owner := cs.clientConf[KOwner]
	key := md5.New()
	if _, err := key.Write([]byte(owner)); err != nil {
		return "", status.Errorf(codes.Internal, "calc owner[%v] md5 fail. err(%v)", owner, err)
	}

	return hex.EncodeToString(key.Sum(nil)), nil
}
