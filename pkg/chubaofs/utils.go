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
	"fmt"
	"k8s.io/utils/mount"
	"net"
	"os"
	"os/exec"
	"strings"
)

const (
	CfsClientBin = "/cfs/bin/cfs-client"
)

func parseEndpoint(ep string) (string, string, error) {
	if strings.HasPrefix(strings.ToLower(ep), "unix://") || strings.HasPrefix(strings.ToLower(ep), "tcp://") {
		s := strings.SplitN(ep, "://", 2)
		if s[1] != "" {
			return s[0], s[1], nil
		}
	}
	return "", "", fmt.Errorf("invalid endpoint: %v", ep)
}

func getFreePort(defaultPort int) (int, error) {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		return defaultPort, err
	}

	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return defaultPort, err
	}

	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func createMountPoint(root string) error {
	return os.MkdirAll(root, 0750)
}

// return true if mount
func isMountPoint(targetPath string) (bool, error) {
	notMnt, err := mount.New("").IsLikelyNotMountPoint(targetPath)
	// notMnt is false if the target path has been mount
	return !notMnt, err
}

func bindMount(stagingTargetPath string, targetPath string) error {
	if _, err := execCommand("mount", "--bind", stagingTargetPath, targetPath); err != nil {
		return fmt.Errorf("mount --bind %s to %s fail: %v", stagingTargetPath, targetPath, err)
	}
	return nil
}

func listMount() ([]mount.MountPoint, error) {
	return mount.New("").List()
}

func mountVolume(configFilePath string) error {
	_, err := execCommand(CfsClientBin, "-c", configFilePath)
	return err
}

func umountVolume(path string) error {
	if _, err := execCommand("umount", path); err != nil {
		return fmt.Errorf("umount %s fail: %v", path, err)
	}
	return nil
}

func execCommand(command string, args ...string) ([]byte, error) {
	cmd := exec.Command(command, args...)
	return cmd.CombinedOutput()
}

func substr(s string, pos, length int) string {
	runes := []rune(s)
	l := pos + length
	if l > len(runes) {
		l = len(runes)
	}
	return string(runes[pos:l])
}

func getParentDirectory(dirctory string) string {
	return substr(dirctory, 0, strings.LastIndex(dirctory, "/"))
}

func pathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}
