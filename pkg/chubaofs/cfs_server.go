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
	KAuthenticate  = "authenticate"
	KTicketHosts   = "ticketHost"
	KEnableHTTPS   = "enableHTTPS"
	KAccessKey     = "accessKey"
	KSecretKey     = "secretKey"
	KLookupValid   = "lookupValid"
	KAttrValid     = "attrValid"
	KIcacheTimeout = "icacheTimeout"
	KEnSyncWrite   = "enSyncWrite"
	KAutoInvalData = "autoInvalData"
	KRdonly        = "rdonly"
	KWritecache    = "writecache"
	KKeepcache     = "keepcache"
)

const (
	defaultOwner              = "csi-user"
	defaultClientConfPath     = "/cfs/conf/"
	defaultLogDir             = "/cfs/logs/"
	defaultExporterPort   int = 9513
	defaultProfPort       int = 10094
	defaultLogLevel           = "info"
	jsonFileSuffix            = ".json"
)

type cfsServer struct {
	clientConfFile string
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
	clientConf := make(map[string]string)
	clientConf[KMasterAddr] = masterAddr
	clientConf[KVolumeName] = newVolName
	clientConf[KOwner] = getValueWithDefault(param, KOwner, newOwner)
	clientConf[KLogLevel] = getValueWithDefault(param, KLogLevel, defaultLogLevel)
	clientConf[KLogDir] = defaultLogDir + newVolName
	clientConf[KZoneName] = getValue(param, KZoneName)
	clientConf[KCrossZone] = getValueWithDefault(param, KCrossZone, "false")
	clientConf[KConsulAddr] = getValue(param, KConsulAddr)
	clientConf[KEnableToken] = getValueWithDefault(param, KEnableToken, "false")
	clientConf[KLookupValid] = getValue(param, KLookupValid)
	clientConf[KAttrValid] = getValue(param, KAttrValid)
	clientConf[KIcacheTimeout] = getValue(param, KIcacheTimeout)
	clientConf[KEnSyncWrite] = getValue(param, KEnSyncWrite)
	clientConf[KAutoInvalData] = getValue(param, KAutoInvalData)
	clientConf[KRdonly] = getValueWithDefault(param, KRdonly, "false")
	clientConf[KWritecache] = getValue(param, KWritecache)
	clientConf[KKeepcache] = getValue(param, KKeepcache)
	clientConf[KAuthenticate] = getValueWithDefault(param, KAuthenticate, "false")
	clientConf[KTicketHosts] = getValue(param, KTicketHosts)
	clientConf[KEnableHTTPS] = getValueWithDefault(param, KEnableHTTPS, "false")
	clientConf[KAccessKey] = getValue(param, KAccessKey)
	clientConf[KSecretKey] = getValue(param, KSecretKey)

	return &cfsServer{
		clientConfFile: clientConfFile,
		clientConf:     clientConf,
	}, err
}

func getValue(param map[string]string, key string) string {
	return getValueWithDefault(param, key, "")
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

func (cs *cfsServer) createVolume(capacityGB int64) error {
	masterAddr := strings.Split(cs.clientConf[KMasterAddr], ",")[0]
	url := fmt.Sprintf("http://%s/admin/createVol?name=%s&capacity=%v&owner=%v&crossZone=%v&enableToken=%v&zoneName=%v",
		masterAddr, cs.clientConf[KVolumeName], capacityGB, cs.clientConf[KOwner], cs.clientConf[KCrossZone], cs.clientConf[KEnableToken], cs.clientConf[KZoneName])
	glog.Infof("createVol url: %v", url)
	resp, err := cs.executeRequest(url)
	if err != nil {
		return err
	}

	if resp.Code != 0 {
		if resp.Code == 1 {
			glog.Warningf("duplicate to create volume. url(%v) code=1 msg:%v", url, resp.Msg)
		} else {
			glog.Errorf("create volume is failed. url(%v) code=(%v), msg:%v", url, resp.Code, resp.Msg)
			return fmt.Errorf("create volume is failed")
		}
	}

	return nil
}

func (cs *cfsServer) deleteVolume() error {
	key := md5.New()
	if _, err := key.Write([]byte(cs.clientConf[KOwner])); err != nil {
		return status.Errorf(codes.Internal, "deleteVolume failed to get md5 sum, err(%v)", err)
	}

	url := fmt.Sprintf("http://%s/vol/delete?name=%s&authKey=%v", cs.clientConf[KMasterAddr], cs.clientConf[KVolumeName], hex.EncodeToString(key.Sum(nil)))
	glog.Infof("deleteVol url: %v", url)
	resp, err := cs.executeRequest(url)
	if err != nil {
		glog.Fatalf("delete volume fail. url:%v error:%v", url, err)
		return err
	}

	if resp.Code != 0 {
		if resp.Code == 7 {
			glog.Warningf("volume not exists, assuming the volume has already been deleted. code:%v, msg:%v", resp.Code, resp.Msg)
		} else {
			glog.Errorf("delete volume is failed. code:%v, msg:%v", resp.Code, resp.Msg)
			return fmt.Errorf("delete volume is failed")
		}
	}

	return nil
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
