package cfs

import (
	"encoding/json"
	"fmt"
	"github.com/golang/glog"
	"io/ioutil"
	"net/http"
)


type CreateDeleteVolumeResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data string `json:"data"`
}

func CreateVolume(host string, volumeName string, volSizeGB int) error {
	createVolUrl := fmt.Sprintf("http://%s/admin/createVol?name=%s&capacity=%v&owner=cfs", host, volumeName, volSizeGB)
	glog.V(2).Infof("CFS: CreateVol url:%v", createVolUrl)

	resp, err := http.Get(createVolUrl)
	if err != nil {
		glog.Errorf("CreateVol cfs failed, error:%v", err)
		return err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		glog.Errorf("Read response of createVol is failed. err:%v", err)
		return err
	}

	var cfsCreateVolumeResp = &CreateDeleteVolumeResponse{}
	if err := json.Unmarshal(body, cfsCreateVolumeResp); err != nil {
		glog.Errorf("Cannot unmarshal response of createVol. bodyLen:%d, err:%v", len(body), err)
		return err
	}
	glog.V(2).Infof("CFS: createVol response:%v", cfsCreateVolumeResp)

	if cfsCreateVolumeResp.Code != 0 {
		if cfsCreateVolumeResp.Code == 1 {
			glog.Warning("CFS: duplicate to create volume. msg:%v", cfsCreateVolumeResp.Msg)
		} else {
			glog.Errorf("CFS: create volume is failed. code:%v, msg:%v", cfsCreateVolumeResp.Code, cfsCreateVolumeResp.Msg)
			return fmt.Errorf("create volume is failed")
		}
	}
	return nil
}

func DeleteVolume(host string, volumeName string) error {
	deleteVolUrl := "http://" + host + "/vol/delete?name=" + volumeName + "&authKey=7b2f1bf38b87d32470c4557c7ff02e75"
	resp, err := http.Get(deleteVolUrl)
	if err != nil {
		glog.Errorf("DeleteVol cfs failed, error:%v", err)
		return err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		glog.Errorf("Read response of deleteVol is failed. err:%v", err)
		return err
	}

	var cfsDeleteVolumeResp = &CreateDeleteVolumeResponse{}
	if err := json.Unmarshal(body, cfsDeleteVolumeResp); err != nil {
		glog.Errorf("Cannot unmarshal response of deleteVol. bodyLen:%d, err:%v", len(body), err)
		return err
	}
	glog.V(2).Infof("CFS: deleteVol response:%v", cfsDeleteVolumeResp)

	if cfsDeleteVolumeResp.Code != 0 {
		if cfsDeleteVolumeResp.Code == 7{
			glog.Warning("CFS: volume not exists, assuming the volume has already been deleted. code:%v, msg:%v", cfsDeleteVolumeResp.Code, cfsDeleteVolumeResp.Msg)
			return nil
		}
		glog.Errorf("CFS: delete volume is failed. code:%v, msg:%v", cfsDeleteVolumeResp.Code, cfsDeleteVolumeResp.Msg)
		return fmt.Errorf("delete volume is failed")
	}
	return nil
}
