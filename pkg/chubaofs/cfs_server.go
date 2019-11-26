package chubaofs

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"io/ioutil"
	"k8s.io/klog"
	"net/http"
	"os"
	"os/exec"
)

const (
	KVolumeName   = "volName"
	KMasterAddr   = "masterAddr"
	KLogDir       = "logDir"
	KWarnLogDir   = "warnLogDir"
	KLogLevel     = "logLevel"
	KOwner        = "owner"
	KProfPort     = "profPort"
	KConsulAddr   = "consulAddr"
	KExporterPort = "exporterPort"
)

const (
	defaultOwner              = "csi-user"
	defaultClientConfPath     = "/cfs/conf/"
	defaultLogDir             = "/cfs/logs/"
	defaultExporterPort   int = 9513
	defaultProfPort       int = 10094
	defaultLogLevel           = "info"
	CfsClientBin              = "/cfs/bin/cfs-client"
)

type cfsServer struct {
	masterAddr     string
	volName        string
	owner          string
	clientConfFile string
	clientConf     *cfsClientConf
}

type cfsClientConf struct {
	MasterAddr   string `json:"masterAddr"`
	VolName      string `json:"volName"`
	Owner        string `json:"owner"`
	LogDir       string `json:"logDir"`
	MountPoint   string `json:"mountPoint"`
	LogLevel     string `json:"logLevel"`
	ConsulAddr   string `json:"consulAddr"`
	ExporterPort int    `json:"exporterPort"`
	ProfPort     int    `json:"profPort"`
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

	clientConfFile := defaultClientConfPath + volName
	logDir := defaultLogDir + volName
	owner := param[KOwner]
	if len(owner) == 0 {
		owner = defaultOwner
	}

	logLevel := param[KLogLevel]
	if len(logLevel) == 0 {
		logLevel = defaultLogLevel
	}

	consulAddr := param[KConsulAddr]
	if len(consulAddr) == 0 {
		consulAddr = ""
	}

	return &cfsServer{
		masterAddr:     masterAddr,
		volName:        volName,
		owner:          owner,
		clientConfFile: clientConfFile,
		clientConf: &cfsClientConf{
			MasterAddr: masterAddr,
			VolName:    volName,
			Owner:      owner,
			LogLevel:   logLevel,
			ConsulAddr: consulAddr,
			LogDir:     logDir,
		},
	}, err
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

	klog.V(0).Infof("create client config file success, volumeId:%v", cs.volName)
	return nil
}

func (cs *cfsServer) createVolume(capacityGB int64) error {
	url := fmt.Sprintf("http://%s/admin/createVol?name=%s&capacity=%v&owner=%v", cs.masterAddr, cs.volName, capacityGB, cs.owner)
	klog.Infof("createVol url: %v", url)
	resp, err := cs.executeRequest(url)
	if err != nil {
		return err
	}

	if resp.Code != 0 {
		if resp.Code == 1 {
			klog.Warning("duplicate to create volume. url(%v) code=1 msg:%v", url, resp.Msg)
		} else {
			klog.Errorf("create volume is failed. url(%v) code=(%v), msg:%v", url, resp.Code, resp.Msg)
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
	klog.Infof("deleteVol url: %v", url)
	resp, err := cs.executeRequest(url)
	if err != nil {
		return err
	}

	if resp.Code != 0 {
		if resp.Code == 7 {
			klog.Warning("volume not exists, assuming the volume has already been deleted. code:%v, msg:%v", resp.Code, resp.Msg)
		} else {
			klog.Errorf("delete volume is failed. code:%v, msg:%v", resp.Code, resp.Msg)
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
	cmd := exec.Command(CfsClientBin, "-c", cs.clientConfFile)
	msg, err := cmd.CombinedOutput()
	if err != nil {
		return err
	}

	klog.V(5).Info("cfs-client execute output:%v", msg)
	return nil
}
