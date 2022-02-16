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
	"os"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/cubefs/cubefs-csi/pkg/csi-common"
	"github.com/golang/glog"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/utils/mount"
)

const DriverName = "csi.cubefs.com"

type driver struct {
	*csicommon.CSIDriver
	ids *csicommon.DefaultIdentityServer
	cs  *controllerServer
	ns  *nodeServer
	Config
}

type Config struct {
	NodeID         string
	DriverName     string
	KubeConfig     string
	Version        string
	RemountDamaged bool
	KubeletRootDir string
}

func NewDriver(conf Config) (*driver, error) {
	glog.Infof("driverName:%v, version:%v, nodeID:%v", conf.DriverName, conf.Version, conf.NodeID)
	clientSet, err := initClientSet(conf.KubeConfig)
	if err != nil {
		glog.Errorf("init client-go Clientset fail. kubeconfig:%v, err:%v", conf.KubeConfig, err)
		return nil, err
	}

	csiDriver := csicommon.NewCSIDriver(conf.DriverName, conf.Version, conf.NodeID, clientSet)
	if csiDriver == nil {
		return nil, status.Error(codes.InvalidArgument, "csiDriver init fail")
	}

	csiDriver.AddControllerServiceCapabilities(
		[]csi.ControllerServiceCapability_RPC_Type{
			csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
			csi.ControllerServiceCapability_RPC_EXPAND_VOLUME,
		})
	csiDriver.AddVolumeCapabilityAccessModes(
		[]csi.VolumeCapability_AccessMode_Mode{
			csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
			csi.VolumeCapability_AccessMode_SINGLE_NODE_READER_ONLY,
			csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY,
			csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
		})

	return &driver{
		CSIDriver: csiDriver,
		Config:    conf,
	}, nil
}

func initClientSet(kubeconfig string) (*kubernetes.Clientset, error) {
	var config *rest.Config
	var err error
	if kubeconfig != "" {
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

func NewNodeServer(d *driver) *nodeServer {
	return &nodeServer{
		DefaultNodeServer: csicommon.NewDefaultNodeServer(d.CSIDriver),
		mounter:           mount.New(""),
		Config:            d.Config,
	}
}

func NewIdentityServer(d *driver) *identityServer {
	return &identityServer{
		DefaultIdentityServer: csicommon.NewDefaultIdentityServer(d.CSIDriver),
	}
}

func NewControllerServer(d *driver) *controllerServer {
	return &controllerServer{
		DefaultControllerServer: csicommon.NewDefaultControllerServer(d.CSIDriver),
		driver:                  d,
	}
}

func (d *driver) Run(endpoint string) {
	nodeServer := NewNodeServer(d)
	if nodeName := os.Getenv("KUBE_NODE_NAME"); d.RemountDamaged && nodeName != "" {
		nodeServer.remountDamagedVolumes(nodeName)
	}

	csicommon.RunControllerandNodePublishServer(endpoint, NewIdentityServer(d), NewControllerServer(d), NewNodeServer(d))
}

func (d *driver) queryPersistentVolumes(pvName string) (*v1.PersistentVolume, error) {
	persistentVolume, err := d.CSIDriver.ClientSet.CoreV1().PersistentVolumes().Get(pvName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	if persistentVolume == nil {
		return nil, status.Error(codes.Unknown, fmt.Sprintf("not found PersistentVolume[%v]", pvName))
	}

	return persistentVolume, nil
}
