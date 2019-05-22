package cfs

import (
	"github.com/container-storage-interface/spec/lib/go/csi/v0"
	"github.com/golang/glog"

	"github.com/chubaofs/chubaofs-csi/pkg/csi-common"
)

type driver struct {
	csiDriver   *csicommon.CSIDriver
	endpoint    string
	cloudconfig string

	ids *csicommon.DefaultIdentityServer
	cs  *controllerServer
	ns  *nodeServer

	cap   []*csi.VolumeCapability_AccessMode
	cscap []*csi.ControllerServiceCapability
}

const (
	driverName = "csi-cfsplugin"
)

var (
	version = "0.3.0"
)

func NewDriver(nodeID, endpoint string) *driver {
	glog.Infof("Driver: %v version: %v", driverName, version)

	d := &driver{}

	d.endpoint = endpoint

	csiDriver := csicommon.NewCSIDriver(driverName, version, nodeID)
	csiDriver.AddControllerServiceCapabilities(
		[]csi.ControllerServiceCapability_RPC_Type{
			csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
		})
	csiDriver.AddVolumeCapabilityAccessModes([]csi.VolumeCapability_AccessMode_Mode{csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER})

	d.csiDriver = csiDriver

	return d
}

func NewControllerServer(d *driver) *controllerServer {
	return &controllerServer{
		DefaultControllerServer: csicommon.NewDefaultControllerServer(d.csiDriver),
		cfsMasterHosts:          make(map[string][]string),
	}
}

func NewNodeServer(d *driver) *nodeServer {
	return &nodeServer{
		DefaultNodeServer: csicommon.NewDefaultNodeServer(d.csiDriver),
	}
}

func (d *driver) Run() {

	csicommon.RunControllerandNodePublishServer(d.endpoint, d.csiDriver, NewControllerServer(d), NewNodeServer(d))
}
