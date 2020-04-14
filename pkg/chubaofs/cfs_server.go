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
	"github.com/golang/glog"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"io/ioutil"
	"net/http"
	"os"
)

const (
	KVolumeName    = "volName"
	KMasterAddr    = "masterAddr"
	KlogLevel      = "logLevel"
	KOwner         = "owner"
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
	masterAddr     string
	volName        string
	owner          string
	clientConfFile string
	clientConf     *cfsClientConf
}

type cfsClientConf struct {
	MasterAddr    string `json:"masterAddr"`
	VolName       string `json:"volName"`
	Owner         string `json:"owner"`
	LogDir        string `json:"logDir"`
	MountPoint    string `json:"mountPoint"`
	LogLevel      string `json:"logLevel"`
	ConsulAddr    string `json:"consulAddr,omitempty"`
	ExporterPort  int    `json:"exporterPort"`
	ProfPort      int    `json:"profPort"`
	LookupValid   string `json:"lookupValid,omitempty"`
	AttrValid     string `json:"attrValid,omitempty"`
	IcacheTimeout string `json:"icacheTimeout,omitempty"`
	EnSyncWrite   string `json:"enSyncWrite,omitempty"`
	AutoInvalData string `json:"autoInvalData,omitempty"`
	Rdonly        string `json:"rdonly,omitempty"`
	Writecache    string `json:"writecache,omitempty"`
	Keepcache     string `json:"keepcache,omitempty"`
	Authenticate  string `json:"authenticate,omitempty"`
	TicketHosts   string `json:"ticketHost,omitempty"`
	EnableHTTPS   string `json:"enableHTTPS,omitempty"`
	AccessKey     string `json:"accessKey,omitempty"`
	SecretKey     string `json:"secretKey,omitempty"`
}

// Create and Delete Volume Response
type cfsServerResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data string `json:"data"`
}

func newCfsServer(volName string, param map[string]string) (cs *cfsServer, err error) {
	masterAddr := param[KMasterAddr]
	if len(volName) == 0 || len(masterAddr) == 0 {
		return nil, fmt.Errorf("invalid argument for initializing cfsServer")
	}

	newVolName := getValueWithDefault(param, KVolumeName, volName)
	clientConfFile := defaultClientConfPath + newVolName + jsonFileSuffix
	logDir := defaultLogDir + newVolName
	owner := getValueWithDefault(param, KOwner, defaultOwner)
	logLevel := getValueWithDefault(param, KlogLevel, defaultLogLevel)
	consulAddr := getValue(param, KConsulAddr)
	lookupValid := getValue(param, KLookupValid)
	attrValid := getValue(param, KAttrValid)
	icacheTimeout := getValue(param, KIcacheTimeout)
	enSyncWrite := getValue(param, KEnSyncWrite)
	autoInvalData := getValue(param, KAutoInvalData)
	rdonly := getValueWithDefault(param, KRdonly, "false")
	writecache := getValue(param, KWritecache)
	keepcache := getValue(param, KKeepcache)
	authenticate := getValueWithDefault(param, KAuthenticate, "false")
	ticketHost := getValue(param, KTicketHosts)
	enableHTTPS := getValueWithDefault(param, KEnableHTTPS, "false")
	accessKey := getValue(param, KAccessKey)
	secretKey := getValue(param, KSecretKey)

	return &cfsServer{
		masterAddr:     masterAddr,
		volName:        newVolName,
		owner:          owner,
		clientConfFile: clientConfFile,
		clientConf: &cfsClientConf{
			MasterAddr:    masterAddr,
			VolName:       newVolName,
			Owner:         owner,
			LogLevel:      logLevel,
			ConsulAddr:    consulAddr,
			LogDir:        logDir,
			LookupValid:   lookupValid,
			AttrValid:     attrValid,
			IcacheTimeout: icacheTimeout,
			EnSyncWrite:   enSyncWrite,
			AutoInvalData: autoInvalData,
			Rdonly:        rdonly,
			Writecache:    writecache,
			Keepcache:     keepcache,
			Authenticate:  authenticate,
			TicketHosts:   ticketHost,
			EnableHTTPS:   enableHTTPS,
			AccessKey:     accessKey,
			SecretKey:     secretKey,
		},
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
	cs.clientConf.MountPoint = mountPoint
	cs.clientConf.ExporterPort, _ = getFreePort(defaultExporterPort)
	cs.clientConf.ProfPort, _ = getFreePort(defaultProfPort)
	_ = os.Mkdir(cs.clientConf.LogDir, 0777)
	clientConfBytes, _ := json.Marshal(cs.clientConf)
	err := ioutil.WriteFile(cs.clientConfFile, clientConfBytes, 0444)
	if err != nil {
		return status.Errorf(codes.Internal, "create client config file fail. err: %v", err.Error())
	}

	glog.V(0).Infof("create client config file success, volumeId:%v", cs.volName)
	return nil
}

func (cs *cfsServer) createVolume(capacityGB int64) error {
	url := fmt.Sprintf("http://%s/admin/createVol?name=%s&capacity=%v&owner=%v", cs.masterAddr, cs.volName, capacityGB, cs.owner)
	glog.Infof("createVol url: %v", url)
	resp, err := cs.executeRequest(url)
	if err != nil {
		return err
	}

	if resp.Code != 0 {
		if resp.Code == 1 {
			glog.Warning("duplicate to create volume. url(%v) code=1 msg:%v", url, resp.Msg)
		} else {
			glog.Errorf("create volume is failed. url(%v) code=(%v), msg:%v", url, resp.Code, resp.Msg)
			return fmt.Errorf("create volume is failed")
		}
	}

	return nil
}

func (cs *cfsServer) deleteVolume() error {
	key := md5.New()
	if _, err := key.Write([]byte(cs.owner)); err != nil {
		return status.Errorf(codes.Internal, "deleteVolume failed to get md5 sum, err(%v)", err)
	}

	url := fmt.Sprintf("http://%s/vol/delete?name=%s&authKey=%v", cs.masterAddr, cs.volName, hex.EncodeToString(key.Sum(nil)))
	glog.Infof("deleteVol url: %v", url)
	resp, err := cs.executeRequest(url)
	if err != nil {
		return err
	}

	if resp.Code != 0 {
		if resp.Code == 7 {
			glog.Warning("volume not exists, assuming the volume has already been deleted. code:%v, msg:%v", resp.Code, resp.Msg)
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
