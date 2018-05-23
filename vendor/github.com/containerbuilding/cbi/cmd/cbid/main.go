/*
Copyright The CBI Authors.
Copyright 2017 The Kubernetes Authors.
https://github.com/kubernetes/sample-controller/tree/4d47428cc1926e6cc47f4a5cf4441077ca1b605f

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
	"context"
	"flag"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/golang/glog"
	"google.golang.org/grpc"
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	// Uncomment the following line to load the gcp plugin (only required to authenticate against GKE clusters).
	// _ "k8s.io/client-go/plugin/pkg/client/auth/gcp"

	"github.com/containerbuilding/cbi/pkg/cbid/controller"
	"github.com/containerbuilding/cbi/pkg/cbid/pluginselector"
	"github.com/containerbuilding/cbi/pkg/cbid/pluginselector/generic"
	clientset "github.com/containerbuilding/cbi/pkg/client/clientset/versioned"
	informers "github.com/containerbuilding/cbi/pkg/client/informers/externalversions"
	"github.com/containerbuilding/cbi/pkg/plugin"
	"github.com/containerbuilding/cbi/pkg/signals"
)

var (
	masterURL  string
	kubeconfig string
	pluginsStr string
)

func main() {
	flag.Parse()

	cbiPlugins, err := parsePluginsStr(pluginsStr)
	if err != nil {
		glog.Fatal(err)
	}
	if len(cbiPlugins) == 0 {
		glog.Fatalf("no CBI plugin specified")
	}

	var cbiPluginConns []*grpc.ClientConn
	for _, s := range cbiPlugins {
		c, err := grpc.Dial(s, grpc.WithInsecure())
		if err != nil {
			glog.Fatal(err)
		}
		cbiPluginConns = append(cbiPluginConns, c)
	}
	ps := pluginselector.NewPluginSelector(generic.SelectPlugin, cbiPluginConns...)
	// FIXME: keep them latest
	if err := ps.UpdateCachedInfo(context.TODO()); err != nil {
		glog.Fatal(err)
	}

	// set up signals so we handle the first shutdown signal gracefully
	stopCh := signals.SetupSignalHandler()

	cfg, err := clientcmd.BuildConfigFromFlags(masterURL, kubeconfig)
	if err != nil {
		glog.Fatalf("Error building kubeconfig: %s", err.Error())
	}

	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		glog.Fatalf("Error building kubernetes clientset: %s", err.Error())
	}

	cbiClient, err := clientset.NewForConfig(cfg)
	if err != nil {
		glog.Fatalf("Error building CBI clientset: %s", err.Error())
	}

	kubeInformerFactory := kubeinformers.NewSharedInformerFactory(kubeClient, time.Second*30)
	cbiInformerFactory := informers.NewSharedInformerFactory(cbiClient, time.Second*30)

	controller := controller.New(
		kubeClient,
		cbiClient,
		kubeInformerFactory,
		cbiInformerFactory,
		ps)

	go kubeInformerFactory.Start(stopCh)
	go cbiInformerFactory.Start(stopCh)

	if err = controller.Run(2, stopCh); err != nil {
		glog.Fatalf("Error running controller: %s", err.Error())
	}
	for _, c := range cbiPluginConns {
		if err := c.Close(); err != nil {
			glog.Fatal(err)
		}
	}
}

func parsePluginsStr(s string) ([]string, error) {
	fields := strings.FieldsFunc(s, func(c rune) bool { return c == ',' || unicode.IsSpace(c) })
	var res []string
	for _, f := range fields {
		if strings.Contains(f, "://") {
			return nil, fmt.Errorf("bad plugin: extra scheme: %q", f)
		}
		if !strings.Contains(f, ":") {
			f = fmt.Sprintf("%s:%d", f, plugin.DefaultPort)
		}
		res = append(res, f)
	}
	return res, nil
}

func init() {
	flag.StringVar(&kubeconfig, "kubeconfig", "", "Path to a kubeconfig. Only required if out-of-cluster.")
	flag.StringVar(&masterURL, "master", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
	flag.StringVar(&pluginsStr, "cbi-plugins", "", "Comma-separated list of CBI plugin hostname[:port]")
}
