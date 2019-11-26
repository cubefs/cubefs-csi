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
package main

import (
	"flag"
	"fmt"
	cfs "github.com/chubaofs/chubaofs-csi/pkg/chubaofs"
	"github.com/spf13/cobra"
	"k8s.io/klog"
	"os"
)

var (
	endpoint   string
	nodeID     string
	driverName string
)

func init() {
	_ = flag.Set("logtostderr", "true")
}

func main() {
	klog.InitFlags(nil)
	_ = flag.CommandLine.Parse([]string{})
	cmd := &cobra.Command{
		Use:   "cfs-csi --endpoint <endpoint> --nodeid <nodeid>",
		Short: "ChubaoFS CSI driver",
		Run: func(cmd *cobra.Command, args []string) {
			handle()
		},
	}

	cmd.Flags().AddGoFlagSet(flag.CommandLine)
	cmd.PersistentFlags().StringVar(&nodeID, "nodeid", "", "node id")
	_ = cmd.MarkPersistentFlagRequired("nodeid")
	cmd.PersistentFlags().StringVar(&endpoint, "endpoint", "", "CSI endpoint")
	_ = cmd.MarkPersistentFlagRequired("endpoint")
	cmd.PersistentFlags().StringVar(&driverName, "drivername", "csi.chubaofs.com", "ChubaoFS driver name")
	if err := cmd.Execute(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "%v", err)
		os.Exit(1)
	}

	os.Exit(0)
}

func handle() {
	d, err := cfs.NewDriver(nodeID, endpoint, driverName)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "%v", err)
		os.Exit(1)
	}

	d.Run()
}
