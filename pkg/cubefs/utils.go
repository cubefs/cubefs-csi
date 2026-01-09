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
	"fmt"
	"net"
	"os"
	"os/exec"
	"path"
	"strings"

	"k8s.io/utils/mount"
)

func parseEndpoint(ep string) (string, string, error) {
	lowerEp := strings.ToLower(ep)
	if strings.HasPrefix(lowerEp, "unix://") || strings.HasPrefix(lowerEp, "tcp://") {
		parts := strings.SplitN(ep, "://", 2)
		if parts[1] != "" {
			return parts[0], parts[1], nil
		}
	}
	return "", "", fmt.Errorf("invalid endpoint: %v (must start with unix://or tcp://)", ep)
}

func getFreePort(defaultPort int) (int, error) {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		return defaultPort, fmt.Errorf("net.ResolveTCPAddr error: %v", err)
	}

	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return defaultPort, fmt.Errorf("net.ListenTCP error: %v", err)
	}
	defer l.Close()

	tcpAddr, ok := l.Addr().(*net.TCPAddr)
	if !ok {
		return defaultPort, fmt.Errorf("unable convert to net.TCPAddr")
	}
	return tcpAddr.Port, nil
}

func createMountPoint(path string) error {
	if err := os.MkdirAll(path, 0755); err != nil {
		if !os.IsExist(err) {
			return fmt.Errorf("create mountPoint: %s, err: %v", path, err)
		}
	}
	return nil
}

func isMountPoint(path string) (bool, error) {
	if _, err := os.Stat(path); err != nil {
		return false, fmt.Errorf("path %s does not exist, err: %v", path, err)
	}
	isNotMount, err := mount.New("").IsLikelyNotMountPoint(path)
	if err != nil {
		return false, fmt.Errorf("failed to determine the mounting point, path: %s, err: %v", path, err)
	}
	return !isNotMount, nil
}

func bindMount(source, target string, readOnly bool) error {
	options := []string{"bind"}
	if readOnly {
		options = append(options, "ro")
	}
	mounter := mount.New("")
	if err := mounter.Mount(source, target, "", options); err != nil {
		return fmt.Errorf("bind mount fail. %s->%s, opts=%v, err: %v", source, target, options, err)
	}
	return nil
}

func listMount() ([]mount.MountPoint, error) {
	return mount.New("").List()
}

func umountVolume(path string) error {
	output, err := execCommand("umount", path)
	if err != nil {
		return fmt.Errorf("umount fail: %s, output: %s, err: %v", path, string(output), err)
	}
	return nil
}

func execCommand(command string, args ...string) ([]byte, error) {
	cmd := exec.Command(command, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return output, fmt.Errorf("exec.Command fail: %s %s, err: %v", command, strings.Join(args, " "), err)
	}
	return output, nil
}

func CleanPath(targetPath string) error {
	parentDir := path.Dir(targetPath)
	if err := os.RemoveAll(parentDir); err != nil {
		return fmt.Errorf("CleanPath: %s, err: %v", parentDir, err)
	}
	return nil
}

func pathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, fmt.Errorf("path exists: %s, err: %v", path, err)
}
