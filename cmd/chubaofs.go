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
	"github.com/golang/glog"
	"os"
	"os/signal"
	"syscall"

	"github.com/chubaofs/chubaofs-csi/pkg/chubaofs"
	"github.com/spf13/cobra"
)

var (
	endpoint   string
	nodeID     string
	driverName string
	version    = "1.0.0"
	kubeconfig string
)

func init() {
	_ = flag.Set("logtostderr", "true")
}

func main() {
	_ = flag.CommandLine.Parse([]string{})
	cmd := &cobra.Command{
		Use:   "cfs-csi-driver --endpoint=<endpoint> --nodeid=<nodeid> --drivername=<drivername> --version=<version>",
		Short: "CSI based CFS driver",
		Run: func(cmd *cobra.Command, args []string) {
			registerInterceptedSignal()
			handle()
		},
	}

	cmd.Flags().AddGoFlagSet(flag.CommandLine)
	cmd.PersistentFlags().StringVar(&nodeID, "nodeid", "", "This node's ID")
	cmd.PersistentFlags().StringVar(&endpoint, "endpoint", "unix:///csi/csi.sock", "CSI endpoint, must be a UNIX socket")
	cmd.PersistentFlags().StringVar(&driverName, "drivername", "csi.chubaofs.com", "name of the driver (Kubernetes: `provisioner` field in StorageClass must correspond to this value)")
	cmd.PersistentFlags().StringVar(&version, "version", version, "Driver version")
	cmd.PersistentFlags().StringVar(&kubeconfig, "kubeconfig", "", "Kubernetes config")

	if err := cmd.Execute(); err != nil {
		glog.Errorf("cmd.Execute error:%v\n", err)
		os.Exit(1)
	}

	os.Exit(0)
}

func handle() {
	d, err := chubaofs.NewDriver(driverName, version, nodeID, kubeconfig)
	if err != nil {
		glog.Errorf("chubaofs.NewDriver error:%v\n", err)
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
