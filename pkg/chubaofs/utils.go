package chubaofs

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/golang/glog"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

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

// Create and Delete Volume Response
type generalVolumeResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data string `json:"data"`
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
		switch req {
		case createVolumeRequest:
			if resp.Code == 1 {
				glog.Warning("chubaofs: duplicate to create volume. msg:%v", resp.Msg)
			} else {
				glog.Errorf("CFS: create volume is failed. code:%v, msg:%v", resp.Code, resp.Msg)
				return fmt.Errorf("create volume is failed")
			}
		case deleteVolumeRequest:
			if resp.Code == 7 {
				glog.Warning("CFS: volume not exists, assuming the volume has already been deleted. code:%v, msg:%v", resp.Code, resp.Msg)
			} else {
				glog.Errorf("CFS: delete volume is failed. code:%v, msg:%v", resp.Code, resp.Msg)
				return fmt.Errorf("delete volume is failed")
			}
		}

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
func parseEndpoint(ep string) (string, string, error) {
	if strings.HasPrefix(strings.ToLower(ep), "unix://") || strings.HasPrefix(strings.ToLower(ep), "tcp://") {
		s := strings.SplitN(ep, "://", 2)
		if s[1] != "" {
			return s[0], s[1], nil
		}
	}
	return "", "", fmt.Errorf("invalid endpoint: %v", ep)
}
