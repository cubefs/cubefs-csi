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

package csicommon

import (
	"fmt"
	"path"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/glog"
	"github.com/kubernetes-csi/csi-lib-utils/protosanitizer"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

func ParseEndpoint(ep string) (string, string, error) {
	if strings.HasPrefix(strings.ToLower(ep), "unix://") || strings.HasPrefix(strings.ToLower(ep), "tcp://") {
		s := strings.SplitN(ep, "://", 2)
		if s[1] != "" {
			return s[0], s[1], nil
		}
	}
	return "", "", fmt.Errorf("Invalid endpoint: %v", ep)
}

func NewVolumeCapabilityAccessMode(mode csi.VolumeCapability_AccessMode_Mode) *csi.VolumeCapability_AccessMode {
	return &csi.VolumeCapability_AccessMode{Mode: mode}
}

func NewDefaultNodeServer(d *CSIDriver) *DefaultNodeServer {
	return &DefaultNodeServer{
		Driver: d,
	}
}

func NewDefaultIdentityServer(d *CSIDriver) *DefaultIdentityServer {
	return &DefaultIdentityServer{
		Driver: d,
	}
}

func NewDefaultControllerServer(d *CSIDriver) *DefaultControllerServer {
	return &DefaultControllerServer{
		Driver: d,
	}
}

func NewControllerServiceCapability(cap csi.ControllerServiceCapability_RPC_Type) *csi.ControllerServiceCapability {
	return &csi.ControllerServiceCapability{
		Type: &csi.ControllerServiceCapability_Rpc{
			Rpc: &csi.ControllerServiceCapability_RPC{
				Type: cap,
			},
		},
	}
}

func RunControllerandNodePublishServer(endpoint string, ids csi.IdentityServer, cs csi.ControllerServer, ns csi.NodeServer) {
	s := NewNonBlockingGRPCServer()
	s.Start(endpoint, ids, cs, ns)
	s.Wait()
}

var tooFrequentMethods = []string{"NodeGetVolumeStats", "NodeGetCapabilities"}

// some methods are too frequently, which will cause a full page of junk log
func needFullLog(method string) bool {
	method = path.Base(method)
	for _, m := range tooFrequentMethods {
		if m == method {
			return false
		}
	}

	return true
}

func logGRPC(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	needFullLog := needFullLog(info.FullMethod)
	if needFullLog {
		glog.V(5).Infof("GRPC request: %s body: %s", info.FullMethod, protosanitizer.StripSecrets(req))
	}

	resp, err := handler(ctx, req)
	if err != nil {
		glog.Errorf("GRPC error: %v", err)
	} else if needFullLog {
		glog.V(5).Infof("GRPC response: %s return: %s", info.FullMethod, protosanitizer.StripSecrets(resp))
	}

	return resp, err
}

// ShortenString returns the first N slice of a string.
func ShortenString(str string, n int) string {
	if len(str) <= n {
		return str
	}
	return str[:n]
}
