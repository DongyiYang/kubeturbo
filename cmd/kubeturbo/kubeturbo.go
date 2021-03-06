/*
Copyright 2014 The Kubernetes Authors All rights reserved.

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
	"runtime"

	"k8s.io/apiserver/pkg/util/flag"
	"k8s.io/kubernetes/pkg/util/logs"

	"github.com/turbonomic/kubeturbo/cmd/kubeturbo/app"

	"github.com/golang/glog"
	"github.com/spf13/pflag"
)

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())
	glog.V(2).Infof("*** Run Kubeturbo service ***")

	s := app.NewVMTServer()
	s.AddFlags(pflag.CommandLine)
	flag.InitFlags()

	logs.InitLogs()
	defer logs.FlushLogs()

	s.Run(pflag.CommandLine.Args())
}
