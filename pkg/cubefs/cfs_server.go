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
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	csicommon "github.com/cubefs/cubefs-csi/pkg/csi-common"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog/v2"
)

const (
	DefaultClientConfPath = "/cfs/conf"
	DefaultLogDir         = "/cfs/logs"
	CfsClientBin          = "/cfs/bin/cfs-client"

	CfsConfArg     = "-c"    // 配置文件参数标识
	JsonFileSuffix = ".json" // 配置文件后缀
)

const (
	DefaultCfsExporterPort int    = 9513
	DefaultCfsProfPort     int    = 10094
	DefaultCfsZoneName     string = "default"
	DefaultCfsLogLevel     string = "warn"
	DefaultCfsConsulAddr   string = "http://consul-service.cubefs.svc.cluster.local:8500"
	DefaultCfsVolType      string = "0"
)

const (
	// 错误码与消息
	ErrCodeVolNotExists = 7               // 卷不存在错误码
	ErrDuplicateVolMsg  = "duplicate vol" // 卷已存在消息
)

type cfsServerResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data string `json:"data,omitempty"`
}

type cfsExtraParams struct {
	MasterAddrList []string
	ZoneName       string
	CrossZone      bool
	EnableToken    bool
}

type cfsClientConf struct {
	MountPoint string `json:"mountPoint"`
	MasterAddr string `json:"masterAddr"`
	VolName    string `json:"volName"`
	Owner      string `json:"owner"`
	AccessKey  string `json:"accessKey"`
	SecretKey  string `json:"secretKey"`

	LogLevel string `json:"logLevel"`
	LogDir   string `json:"logDir"`
	Rdonly   bool   `json:"rdonly"`

	VolType      string `json:"volType"`
	ExporterPort int    `json:"exporterPort"`
	ProfPort     string `json:"profPort"`
	ConsulAddr   string `json:"consulAddr"`
}

type cfsServer struct {
	clientConfFile string
	extraParams    cfsExtraParams
	clientConf     cfsClientConf
}

func NewCfsServer(pvName string, param map[string]string) (*cfsServer, error) {
	masterAddrStr := param[KeyCfsMaster]
	volName := getParamWithDefault(param, KeyCfsVolName, pvName)
	if len(masterAddrStr) == 0 || len(volName) == 0 {
		return nil, fmt.Errorf("invalid argument for initializing cfsServer")
	}

	// masterAddrList
	masterAddrList := strings.Split(masterAddrStr, ",")
	for i, addr := range masterAddrList {
		masterAddrList[i] = strings.TrimSpace(addr)
		if masterAddrList[i] == "" {
			return nil, fmt.Errorf("invalid masterAddr: %s", masterAddrStr)
		}
	}

	return &cfsServer{
		clientConfFile: path.Join(DefaultClientConfPath, volName+JsonFileSuffix),
		extraParams: cfsExtraParams{
			MasterAddrList: masterAddrList,
			ZoneName:       getParamWithDefault(param, KeyCfsZoneName, DefaultCfsZoneName),
			CrossZone:      parseBool(param[KeyCfsCrossZone]),
			EnableToken:    parseBool(param[KeyCfsEnableToken]),
		},
		clientConf: cfsClientConf{
			MasterAddr: masterAddrStr,
			VolName:    volName,
			Owner:      getParamWithDefault(param, KeyCfsOwner, generateOwner()),
			AccessKey:  param[KeyCfsAccessKey],
			SecretKey:  param[KeyCfsSecretKey],
			LogLevel:   getParamWithDefault(param, KeyCfsLogLevel, DefaultCfsLogLevel),
			LogDir:     path.Join(DefaultLogDir, volName),
			Rdonly:     parseBool(param[KeyCfsReadOnly]),
			VolType:    getParamWithDefault(param, KeyCfsVolType, DefaultCfsVolType),
			ConsulAddr: getParamWithDefault(param, KeyCfsConsulAddr, DefaultCfsConsulAddr),
		},
	}, nil
}

// PersistClientConf 生成客户端配置文件
func (cs *cfsServer) PersistClientConf(mountPoint string) error {
	exporterPort, err := getFreePort(DefaultCfsExporterPort)
	if err != nil {
		klog.Warningf("failed to allocate exporterPort err: %v, set default: %d", err, DefaultCfsExporterPort)
		exporterPort = DefaultCfsExporterPort
	}
	profPort, err := getFreePort(DefaultCfsProfPort)
	if err != nil {
		klog.Warningf("failed to allocate profPort err: %v, set default: %d", err, DefaultCfsProfPort)
		profPort = DefaultCfsProfPort
	}
	cs.clientConf.MountPoint = mountPoint
	cs.clientConf.ExporterPort = exporterPort
	cs.clientConf.ProfPort = strconv.Itoa(profPort)

	// 创建配置目录与日志目录
	if err := os.MkdirAll(DefaultClientConfPath, 0755); err != nil {
		return fmt.Errorf("conf dir: %s, err: %v", DefaultClientConfPath, err)
	}
	if err := os.MkdirAll(cs.clientConf.LogDir, 0755); err != nil {
		return fmt.Errorf("log dir: %s, err: %v", cs.clientConf.LogDir, err)
	}

	// 生成JSON配置文件
	confBytes, err := json.MarshalIndent(cs.clientConf, "", "  ")
	if err != nil {
		return fmt.Errorf("json marshal err: %v", err)
	}
	if err := ioutil.WriteFile(cs.clientConfFile, confBytes, 0644); err != nil {
		return fmt.Errorf("write config file %s fail, err: %v", cs.clientConfFile, err)
	}

	klog.Infof("create config file success: %s", cs.clientConfFile)
	return nil
}

func (cs *cfsServer) CreateVolume(capacityGB int64) error {
	volName := cs.clientConf.VolName
	owner := cs.clientConf.Owner
	crossZone := cs.extraParams.CrossZone
	enableToken := cs.extraParams.EnableToken
	zoneName := cs.extraParams.ZoneName
	volType := cs.clientConf.VolType

	return cs.forEachMasterAddr("CreateVolume", func(addr string) error {
		url := fmt.Sprintf(
			"http://%s/admin/createVol?name=%s&capacity=%d&owner=%s&crossZone=%t&enableToken=%t&zoneName=%s&volType=%s",
			addr, volName, capacityGB, owner, crossZone, enableToken, zoneName, volType,
		)
		klog.Infof("CreateVolume API: %s", url)

		resp, err := cs.executeRequest(url)
		if err != nil {
			return err
		}

		if resp.Code != 0 {
			if strings.Contains(resp.Msg, ErrDuplicateVolMsg) {
				klog.Warningf("duplicate to create volume %s", volName)
				return nil
			}
			return fmt.Errorf("create volume failed, volName: %s, code: %d, msg: %s", volName, resp.Code, resp.Msg)
		}

		klog.Infof("CreateVolume success: %s", volName)
		return nil
	})
}

func (cs *cfsServer) DeleteVolume() error {
	volName := cs.clientConf.VolName
	ownerMd5, err := cs.getOwnerMd5()
	if err != nil {
		return fmt.Errorf("calculate md5(owner) failed, volName: %s, err: %v", volName, err)
	}

	return cs.forEachMasterAddr("DeleteVolume", func(addr string) error {
		url := fmt.Sprintf("http://%s/vol/delete?name=%s&authKey=%s", addr, volName, ownerMd5)
		klog.Infof("DeleteVolume API: %s", url)

		resp, err := cs.executeRequest(url)
		if err != nil {
			return err
		}

		if resp.Code != 0 {
			if resp.Code == ErrCodeVolNotExists {
				klog.Warningf("volume does not exist: %s, skip deletion", volName)
				return nil
			}
			return fmt.Errorf("delete volume failed, volName: %s, code: %d, msg: %s", volName, resp.Code, resp.Msg)
		}

		klog.Infof("DeleteVolume success: %s", volName)
		return nil
	})
}

func (cs *cfsServer) ExpandVolume(capacityGB int64) error {
	volName := cs.clientConf.VolName
	ownerMd5, err := cs.getOwnerMd5()
	if err != nil {
		return fmt.Errorf("calculate md5(owner) failed, volName: %s, err: %v", volName, err)
	}

	return cs.forEachMasterAddr("ExpandVolume", func(addr string) error {
		url := fmt.Sprintf("http://%s/vol/expand?name=%s&authKey=%s&capacity=%d", addr, volName, ownerMd5, capacityGB)
		klog.Infof("ExpandVolume API: %s", url)

		resp, err := cs.executeRequest(url)
		if err != nil {
			return err
		}

		if resp.Code != 0 {
			return fmt.Errorf("expand volume failed, volName: %s, code: %d, msg: %s", volName, resp.Code, resp.Msg)
		}

		klog.Infof("ExpandVolume success: %s, capacity: %dGB", volName, capacityGB)
		return nil
	})
}

func parseBool(val string) bool {
	ok, _ := strconv.ParseBool(val)
	return ok
}

func generateOwner() string {
	return csicommon.ShortenString(fmt.Sprintf("csi_%d", time.Now().UnixNano()), 20)
}

func (cs *cfsServer) forEachMasterAddr(stage string, f func(addr string) error) error {
	var lastErr error
	for _, addr := range cs.extraParams.MasterAddrList {
		var err error
		if err = f(addr); err == nil {
			return nil
		}
		lastErr = err
		klog.Warningf("try %s with master %q failed: %v", stage, addr, err)
	}
	return fmt.Errorf("%s failed with all masters: %v", stage, lastErr)
}

func (cs *cfsServer) executeRequest(url string) (*cfsServerResponse, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "request url failed, url(%s) err(%v)", url, err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "read http response body, url(%s) bodyLen(%d) err(%v)", url, len(body), err)
	}

	var serverResp cfsServerResponse
	if err := json.Unmarshal(body, &serverResp); err != nil {
		return nil, status.Errorf(codes.Unavailable, "unmarshal http response body, url(%s) msg(%s) err(%v)", url, string(body), err)
	}

	return &serverResp, nil
}

func (cs *cfsServer) getOwnerMd5() (string, error) {
	owner := cs.clientConf.Owner
	if owner == "" {
		return "", fmt.Errorf("empty owner")
	}

	h := md5.New()
	if _, err := h.Write([]byte(owner)); err != nil {
		return "", status.Errorf(codes.Internal, "calc owner[%v] md5 fail. err(%v)", owner, err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
