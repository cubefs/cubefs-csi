package chubaofs

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type RequestType int

func (t RequestType) String() string {
	switch t {
	case createVolumeRequest:
		return "CreateVolume"
	case deleteVolumeRequest:
		return "DeleteVolume"
	default:
	}
	return "N/A"
}

const (
	createVolumeRequest RequestType = iota
	deleteVolumeRequest
)

type clusterInfoResponseData struct {
	LeaderAddr string `json:"LeaderAddr"`
}

type clusterInfoResponse struct {
	Code int                      `json:"code"`
	Msg  string                   `json:"msg"`
	Data *clusterInfoResponseData `json:"data"`
}

// Create and Delete Volume Response
type generalVolumeResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data string `json:"data"`
}

/*
 * This functions sends http request to the on-premise cluster to
 * get cluster info.
 */
func getClusterInfo(host string) (string, error) {
	url := "http://" + host + "/admin/getCluster"
	httpResp, err := http.Get(url)
	if err != nil {
		return "", status.Errorf(codes.Unavailable, "chubaofs: failed to get cluster info, url(%v) err(%v)", url, err)
	}
	defer httpResp.Body.Close()

	body, err := ioutil.ReadAll(httpResp.Body)
	if err != nil {
		return "", status.Errorf(codes.Unavailable, "chubaofs: getClusterInfo failed to read response, url(%v) err(%v)", url, err)
	}

	resp := &clusterInfoResponse{}
	if err = json.Unmarshal(body, resp); err != nil {
		return "", status.Errorf(codes.Unavailable, "chubaofs: getClusterInfo failed to unmarshal, url(%v) bodyLen(%v), err(%v)", url, len(body), err)
	}

	// TODO: check if response is exist error
	if resp.Code != 0 {
		return "", status.Errorf(codes.Unavailable, "chubaofs: getClusterInfo response code is NOK, url(%v) code(%v) msg(%v)", url, resp.Code, resp.Msg)
	}

	if resp.Data == nil {
		return "", status.Errorf(codes.Unavailable, "chubaofs: getClusterInfo gets nil response data, url(%v) msg(%v)", url, resp.Msg)
	}

	return resp.Data.LeaderAddr, nil
}

/*
 * This function sends http request to the on-premise cluster to create
 * or delete a volume according to request type.
 */
func createOrDeleteVolume(req RequestType, leader, name, owner string, size int64) error {
	var url string

	switch req {
	case createVolumeRequest:
		sizeInGB := size
		url = fmt.Sprintf("http://%s/admin/createVol?name=%s&capacity=%v&owner=%v", leader, name, sizeInGB, owner)
	case deleteVolumeRequest:
		key := md5.New()
		if _, err := key.Write([]byte(owner)); err != nil {
			return status.Errorf(codes.Internal, "chubaofs: deleteVolume failed to get md5 sum, err(%v)", err)
		}
		url = fmt.Sprintf("http://%s/vol/delete?name=%s&authKey=%v", leader, name, hex.EncodeToString(key.Sum(nil)))
	default:
		return status.Error(codes.InvalidArgument, "chubaofs: createOrDeleteVolume request type not recognized")
	}

	httpResp, err := http.Get(url)
	if err != nil {
		return status.Errorf(codes.Unavailable, "chubaofs: createOrDeleteVolume failed, url(%v) err(%v)", url, err)
	}
	defer httpResp.Body.Close()

	body, err := ioutil.ReadAll(httpResp.Body)
	if err != nil {
		return status.Errorf(codes.Unavailable, "chubaofs: createOrDeleteVolume failed to read http response body, url(%v) bodyLen(%v) err(%v)", url, len(body), err)
	}

	resp := &generalVolumeResponse{}
	if err := json.Unmarshal(body, resp); err != nil {
		return status.Errorf(codes.Unavailable, "chubaofs: createOrDeleteVolume failed to unmarshal, url(%v) msg(%v) err(%v)", url, resp.Msg, err)
	}

	if resp.Code != 0 {
		return status.Errorf(codes.Unavailable, "chubaofs: createOrDeleteVolume response code is NOK, url(%v) code(%v) msg(%v)", url, resp.Code, resp.Msg)
	}

	return nil
}

func doMount(cmdName string, configFile string) error {
	env := []string{
		fmt.Sprintf("PATH=%s", os.Getenv("PATH")),
	}

	cmd := exec.Command(cmdName, "-c", configFile)
	cmd.Env = append(cmd.Env, env...)
	if msg, err := cmd.CombinedOutput(); err != nil {
		return errors.New(fmt.Sprintf("chubaofs: failed to start client daemon, msg: %v , err: %v", string(msg), err))
	}
	return nil
}

func doUmount(mntPoint string) error {
	env := []string{
		fmt.Sprintf("PATH=%s", os.Getenv("PATH")),
	}

	cmd := exec.Command("umount", mntPoint)
	cmd.Env = append(cmd.Env, env...)
	msg, err := cmd.CombinedOutput()
	if err != nil {
		return errors.New(fmt.Sprintf("chubaofs: failed to umount, msg: %v , err: %v", msg, err))
	}
	return nil
}

/*
 * This function creates mount points according to the specified paths,
 * and returns the absolute paths.
 */
func createAbsMntPoints(locations []string) (mntPoints []string, err error) {
	mntPoints = make([]string, 0)
	for _, loc := range locations {
		mnt, e := filepath.Abs(loc)
		if e != nil {
			err = errors.New(fmt.Sprintf("chubaofs: failed to get absolute path of export locations, loc: %v , err: %v", loc, e))
			return
		}
		if e = os.MkdirAll(mnt, os.ModeDir); e != nil {
			err = errors.New(fmt.Sprintf("chubaofs: failed to create mount point dir, mnt: %v , err: %v", mnt, e))
			return
		}
		mntPoints = append(mntPoints, mnt)
	}
	return
}

/*
 * This function generates the target file with specified path, and writes data.
 */
func generateFile(filePath string, data []byte) (int, error) {
	os.MkdirAll(path.Dir(filePath), os.ModePerm)
	fw, err := os.Create(filePath)
	if err != nil {
		return 0, err
	}
	defer fw.Close()
	return fw.Write(data)
}
