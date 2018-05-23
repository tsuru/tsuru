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

package cmd

import (
	"flag"
	"fmt"

	"github.com/containerbuilding/cbi/pkg/plugin"
	"github.com/containerbuilding/cbi/pkg/plugin/base"
	"github.com/containerbuilding/cbi/pkg/plugin/base/service"
)

type Opts struct {
	FlagSet *flag.FlagSet
	Args    []string
	// CreateBackend is called after calling o.FlagSet.Parse(o.Args).
	CreateBackend func() (base.Backend, error)
}

func Main(o Opts) error {
	var (
		port int
	)
	o.FlagSet.IntVar(&port, "cbi-plugin-port", plugin.DefaultPort, "Port for listening CBI Plugin gRPC API")
	if err := o.FlagSet.Parse(o.Args); err != nil {
		return err
	}
	b, err := o.CreateBackend()
	if err != nil {
		return err
	}
	s := &service.Service{
		Backend: b,
	}
	if err := service.ServeTCP(s, port); err != nil {
		return fmt.Errorf("Error serving CBI plugin API: %s", err.Error())
	}
	return nil
}
