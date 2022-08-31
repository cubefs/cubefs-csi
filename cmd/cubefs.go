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

package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/cubefs/cubefs-csi/pkg/cubefs"
	"github.com/golang/glog"

	"github.com/spf13/cobra"
)

var (
	endpoint string
	version  = "1.0.0"
	conf     cubefs.Config
)

// injected while compile
var (
	CommitID  = ""
	BuildTime = ""
	Branch    = ""
)

func init() {
	_ = flag.Set("logtostderr", "true")
}

func main() {
	fmt.Printf("System build info: BuildTime: [%s], Branch [%s], CommitID [%s]\n", BuildTime, Branch, CommitID)

	cmd := &cobra.Command{
		Use:   "cfs-csi-driver --endpoint=<endpoint> --nodeid=<nodeid> --drivername=<drivername> --version=<version>",
		Short: "CSI based CFS driver",
		Run: func(cmd *cobra.Command, args []string) {
			registerInterceptedSignal()
			handle()
		},
	}

	cmd.Flags().AddGoFlagSet(flag.CommandLine)
	cmd.PersistentFlags().StringVar(&conf.NodeID, "nodeid", "", "This node's ID")
	cmd.PersistentFlags().StringVar(&endpoint, "endpoint", "unix:///csi/csi.sock", "CSI endpoint, must be a UNIX socket")
	cmd.PersistentFlags().StringVar(&conf.DriverName, "drivername", cubefs.DriverName, "name of the driver (Kubernetes: `provisioner` field in StorageClass must correspond to this value)")
	cmd.PersistentFlags().StringVar(&conf.Version, "version", version, "Driver version")
	cmd.PersistentFlags().StringVar(&conf.KubeConfig, "kubeconfig", "", "Kubernetes config")
	cmd.PersistentFlags().BoolVar(&conf.RemountDamaged, "remountdamaged", false,
		"Try to remount all the volumes damaged during csi-node restart or upgrade, set mountPropagation of pod to HostToContainer to use this feature")
	cmd.PersistentFlags().StringVar(&conf.KubeletRootDir, "kubeletrootdir", "/var/lib/kubelet", "The path of your kubelet root dir, set it if you customized it")

	if err := cmd.Execute(); err != nil {
		glog.Errorf("cmd.Execute error:%v\n", err)
		os.Exit(1)
	}

	os.Exit(0)
}

func handle() {
	d, err := cubefs.NewDriver(conf)
	if err != nil {
		glog.Errorf("cubefs.NewDriver error:%v\n", err)
		os.Exit(1)
	}

	d.Run(endpoint)
}

func registerInterceptedSignal() {
	sigC := make(chan os.Signal, 1)
	signal.Notify(sigC, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigC
		glog.Errorf("Killed due to a received signal (%v)\n", sig)
		os.Exit(1)
	}()
}
