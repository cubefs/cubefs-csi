package chubaofs

import (
	"fmt"

	"github.com/golang/glog"
)

type driver struct {
	name     string
	nodeID   string
	endpoint string

	ids *identityServer
	cs  *controllerServer
	ns  *nodeServer
}

var (
	version = "1.0.0"
)

func NewDriver(driverName, nodeID, endpoint string) (*driver, error) {
	if driverName == "" {
		return nil, fmt.Errorf("No driver name provided")
	}

	if nodeID == "" {
		return nil, fmt.Errorf("No node id provided")
	}

	if endpoint == "" {
		return nil, fmt.Errorf("No driver endpoint provided")
	}

	glog.Infof("Driver: %v Version: %v", driverName, version)

	return &driver{
		name:     driverName,
		nodeID:   nodeID,
		endpoint: endpoint,
	}, nil
}

func (d *driver) Run() {
	d.ids = NewIdentityServer(d.name, version)
	d.cs = NewControllerServer()
	d.ns = NewNodeServer(d.nodeID)

	// TODO:
	s := NewServer()
	s.Start(d.endpoint, d.ids, d.cs, d.ns)
	s.Wait()
}
