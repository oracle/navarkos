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
	goflag "flag"
	"fmt"
	"github.com/golang/glog"
	clustercontroller "github.com/kubernetes-incubator/navarkos/pkg/controller/cluster"
	dpmtcontroller "github.com/kubernetes-incubator/navarkos/pkg/controller/deployment"
	opt "github.com/kubernetes-incubator/navarkos/pkg/options"
	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apiserver/pkg/util/flag"
	"k8s.io/apiserver/pkg/util/logs"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"os"
	"time"
)

// Version holds the version information injected during build
var Version string

func main() {
	version := goflag.Bool("version", false, "Prints version and exit")

	options := opt.NewNO()
	options.AddFlags(pflag.CommandLine)

	flag.InitFlags()
	// Convinces goflags that we have called Parse() to avoid noisy logs.
	// OSS Issue: kubernetes/kubernetes#17162.
	goflag.CommandLine.Parse([]string{})
	logs.InitLogs()
	defer logs.FlushLogs()

	glog.Infof("Version: %s", Version)
	if *version {
		os.Exit(0)
	}

	if err := Run(options); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

}

//Run initializes config based on environment and starts controller
func Run(options *opt.NavarkoOptions) error {

	var restClientCfg *restclient.Config
	var err error
	if fedKubeConfigLocation, ok := os.LookupEnv("LOCAL_KUBECONFIG"); ok {
		restClientCfg, err = clientcmd.BuildConfigFromFlags("", fedKubeConfigLocation)
	} else {
		restClientCfg, err = clientcmd.BuildConfigFromFlags(options.FkubeApiServer, "/etc/federation/controller-manager/kubeconfig")
	}

	if err != nil || restClientCfg == nil {
		glog.V(2).Infof("Couldn't build the rest client config from flags: %v", err)
		return err
	}

	// Override restClientCfg qps/burst settings from flags
	restClientCfg.QPS = 20.0
	restClientCfg.Burst = 30
	run := func() {
		err := StartControllers(restClientCfg, options)
		glog.Fatalf("error running controllers: %v", err)
		panic("unreachable")
	}
	run()

	return nil
}

//StartControllers launches Navarkos specific cluster and deployment controller
func StartControllers(restClientCfg *restclient.Config, options *opt.NavarkoOptions) error {
	glog.V(1).Infof("Initializing Cluster controller...")
	stopChan := wait.NeverStop
	cController := clustercontroller.StartClusterController(restClientCfg, stopChan, time.Second*10)
	dpmtcontroller.StartDeploymentController(cController, restClientCfg, stopChan, true)
	select {}
}
