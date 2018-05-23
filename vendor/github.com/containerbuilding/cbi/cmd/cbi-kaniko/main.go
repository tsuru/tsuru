/*
Copyright The CBI Authors.

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
	"os"

	"github.com/golang/glog"

	"github.com/containerbuilding/cbi/pkg/plugin/backends/kaniko"
	"github.com/containerbuilding/cbi/pkg/plugin/base"
	"github.com/containerbuilding/cbi/pkg/plugin/base/cbipluginhelper"
	"github.com/containerbuilding/cbi/pkg/plugin/base/cmd"
)

func main() {
	o := cmd.Opts{
		// glog installs itself to flag.CommandLine via init().
		// flag.CommandLine is associated with flag.ExitOnError.
		FlagSet: flag.CommandLine,
		Args:    os.Args[1:],
	}
	var (
		helperImage string
		image       string
	)
	o.FlagSet.StringVar(&helperImage, "helper-image", "", "cbipluginhelper image")
	o.FlagSet.StringVar(&image, "kaniko-image", "", "kaniko image")
	o.CreateBackend = func() (base.Backend, error) {
		if helperImage == "" {
			glog.Fatal("no helper-image provided")
		}
		if image == "" {
			glog.Fatal("no kaniko-image provided")
		}
		b := &kaniko.Kaniko{
			Helper: cbipluginhelper.Helper{
				Image:   helperImage,
				HomeDir: "/root",
			},
			Image: image,
		}
		return b, nil
	}
	if err := cmd.Main(o); err != nil {
		glog.Fatal(err)
	}
}
