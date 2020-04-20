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
	"github.com/chubaofs/chubaofs-csi/pkg/csi-common"
	"github.com/container-storage-interface/spec/lib/go/csi/v0"
	"github.com/golang/glog"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type driver struct {
	csiDriver *csicommon.CSIDriver
	ids       *csicommon.DefaultIdentityServer
	cs        *controllerServer
	ns        *nodeServer

	cap   []*csi.VolumeCapability_AccessMode
	cscap []*csi.ControllerServiceCapability

	clientSet *kubernetes.Clientset
}

func NewDriver(driverName, version, nodeID, kubeconfig string) (*driver, error) {
	glog.Infof("driverName:%v, version:%v, nodeID:%v", driverName, version, nodeID)

	csiDriver := csicommon.NewCSIDriver(driverName, version, nodeID)
	if csiDriver == nil {
		return nil, status.Error(codes.InvalidArgument, "csiDriver init fail")
	}

	csiDriver.AddControllerServiceCapabilities(
		[]csi.ControllerServiceCapability_RPC_Type{
			csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
		})
	csiDriver.AddVolumeCapabilityAccessModes(
		[]csi.VolumeCapability_AccessMode_Mode{
			csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
			csi.VolumeCapability_AccessMode_SINGLE_NODE_READER_ONLY,
			csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY,
			csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
		})

	clientSet, err := initClientSet(kubeconfig)
	if err != nil {
		glog.Errorf("init client-go Clientset fail. kubeconfig:%v, err:%v", kubeconfig, err)
		return nil, err
	}

	return &driver{
		csiDriver: csiDriver,
		clientSet: clientSet,
	}, nil
}

func initClientSet(kubeconfig string) (*kubernetes.Clientset, error) {
	var config *rest.Config
	var err error
	exists, _ := pathExists(kubeconfig)
	if exists {
		// creates the out-of-cluster config
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
	} else {
		// creates the in-cluster config
		config, err = rest.InClusterConfig()
	}

	if err != nil {
		return nil, err
	}

	return kubernetes.NewForConfig(config)
}

func NewControllerServer(d *driver) *controllerServer {
	return &controllerServer{
		DefaultControllerServer: csicommon.NewDefaultControllerServer(d.csiDriver),
		driver:                  d,
	}
}

func NewNodeServer(d *driver) *nodeServer {
	return &nodeServer{
		DefaultNodeServer: csicommon.NewDefaultNodeServer(d.csiDriver),
	}
}

func NewIdentityServer(d *driver) *identityServer {
	return &identityServer{
		DefaultIdentityServer: csicommon.NewDefaultIdentityServer(d.csiDriver),
	}
}

func (d *driver) Run(endpoint string) {
	csicommon.RunControllerandNodePublishServer(endpoint, NewIdentityServer(d), NewControllerServer(d), NewNodeServer(d))
}

func (d *driver) queryPersistentVolumes(pvName string) (*v1.PersistentVolume, error) {
	persistentVolume, err := d.clientSet.CoreV1().PersistentVolumes().Get(pvName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	if persistentVolume == nil {
		return nil, status.Error(codes.Unknown, fmt.Sprintf("not found PersistentVolume[%v]", pvName))
	}

	return persistentVolume, nil
}
