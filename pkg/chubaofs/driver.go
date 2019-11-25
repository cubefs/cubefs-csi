// Copyright 2019 The Chubao Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or
// implied. See the License for the specific language governing
// permissions and limitations under the License.
package chubaofs

import (
	"fmt"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/kubernetes-csi/csi-lib-utils/protosanitizer"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog"
	"net"
	"os"
)

type driver struct {
	name     string
	version  string
	nodeID   string
	endpoint string

	clientSet *kubernetes.Clientset
}

func NewDriver(nodeID, endpoint string, driverName string) (*driver, error) {
	newDriver := &driver{
		name:     driverName,
		version:  "1.0.0",
		nodeID:   nodeID,
		endpoint: endpoint,
	}

	err := newDriver.initClientSet()
	if err != nil {
		return nil, err
	}

	return newDriver, nil
}

func (d *driver) initClientSet() error {
	// creates the in-cluster config
	clusterConfig, err := rest.InClusterConfig()
	if err != nil {
		return err
	}

	// creates the clientSet
	d.clientSet, err = kubernetes.NewForConfig(clusterConfig)
	if err != nil {
		return err
	}

	return nil
}

func (d *driver) Run() {
	ids := NewIdentityServer(d)
	cs := NewControllerServer(d)
	ns := NewNodeServer(d)
	d.serve(ids, cs, ns)
}

func (d *driver) serve(ids csi.IdentityServer, cs csi.ControllerServer, ns csi.NodeServer) {
	proto, addr, err := parseEndpoint(d.endpoint)
	if err != nil {
		klog.Fatal(err.Error())
	}

	if proto == "unix" {
		addr = "/" + addr
		if e := os.Remove(addr); e != nil && !os.IsNotExist(e) {
			klog.Fatalf("Failed to remove %s, error: %s", addr, e.Error())
		}
	}

	listener, err := net.Listen(proto, addr)
	if err != nil {
		klog.Fatalf("Failed to listen: %v", err)
	}

	middleWare := []grpc.UnaryServerInterceptor{logGRPC}
	opts := []grpc.ServerOption{grpc.UnaryInterceptor(middleWare[0])}
	server := grpc.NewServer(opts...)
	csi.RegisterIdentityServer(server, ids)
	csi.RegisterControllerServer(server, cs)
	csi.RegisterNodeServer(server, ns)
	klog.Infof("Listening for connections on address: %#v protocol: %v", addr, proto)
	err = server.Serve(listener)
	if err != nil {
		klog.Fatalf("server serve fail, error:%v", err)
	}
}

func logGRPC(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	klog.V(5).Infof("GRPC call: %s, parameters: %+v", info.FullMethod, protosanitizer.StripSecrets(req))
	resp, err := handler(ctx, req)
	if err != nil {
		klog.Errorf("GRPC error: %v", err)
	} else {
		klog.V(5).Infof("GRPC response: %+v", protosanitizer.StripSecrets(resp))
	}
	return resp, err
}

func (d *driver) queryPersistentVolumes(pvName string) (*v1.PersistentVolume, error) {
	persistentVolume, err := d.clientSet.CoreV1().PersistentVolumes().Get(pvName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	if persistentVolume == nil {
		return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("not found PersistentVolume[%v]", pvName))
	}

	return persistentVolume, nil
}
