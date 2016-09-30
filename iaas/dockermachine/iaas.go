// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dockermachine

import (
	"fmt"

	"github.com/tsuru/tsuru/iaas"
)

var (
	errCaPathNotSet = fmt.Errorf("ca-path configuration is mandatory")

	errDriverNotSet = fmt.Errorf("driver is mandatory")
)

func init() {
	iaas.RegisterIaasProvider("dockermachine", newDockerMachineIaaS)
}

type dockerMachineIaaS struct {
	base       iaas.NamedIaaS
	apiFactory func(DockerMachineConfig) (dockerMachineAPI, error)
}

func newDockerMachineIaaS(name string) iaas.IaaS {
	return &dockerMachineIaaS{
		base:       iaas.NamedIaaS{BaseIaaSName: "dockermachine", IaaSName: name},
		apiFactory: NewDockerMachine,
	}
}

func (i *dockerMachineIaaS) getParamOrConfigString(name string, params map[string]string) (string, error) {
	if v, ok := params[name]; ok {
		return v, nil
	}
	return i.base.GetConfigString(name)
}

func (i *dockerMachineIaaS) CreateMachine(params map[string]string) (*iaas.Machine, error) {
	caPath, err := i.base.GetConfigString("ca-path")
	if err != nil {
		return nil, errCaPathNotSet
	}
	driverName, ok := params["driver"]
	if !ok {
		name, errConf := i.base.GetConfigString("driver:name")
		if errConf != nil {
			return nil, errDriverNotSet
		}
		driverName = name
		params["driver"] = driverName
	}
	dockerEngineInstallURL, err := i.getParamOrConfigString("docker-install-url", params)
	if err != nil {
		dockerEngineInstallURL = ""
	}
	insecureRegistry, _ := i.base.GetConfigString("insecure-registry")
	machineName, ok := params["name"]
	if !ok {
		machines, errList := iaas.ListMachines()
		if errList != nil {
			return nil, errList
		}
		machineName = fmt.Sprintf("%s-%d", params["pool"], len(machines)+1)
	} else {
		delete(params, "name")
	}
	dockerMachine, err := i.apiFactory(DockerMachineConfig{
		CaPath:                 caPath,
		InsecureRegistry:       insecureRegistry,
		DockerEngineInstallURL: dockerEngineInstallURL,
	})
	if err != nil {
		return nil, err
	}
	defer dockerMachine.Close()
	driverOpts := i.buildDriverOpts(params)
	m, err := dockerMachine.CreateMachine(machineName, driverName, driverOpts)
	if err != nil {
		return nil, err
	}
	m.CreationParams = params
	return m, nil
}

func (i *dockerMachineIaaS) buildDriverOpts(params map[string]string) map[string]interface{} {
	driverOpts := make(map[string]interface{})
	config, _ := i.base.GetConfig("driver:options")
	if config != nil {
		for k, v := range config.(map[interface{}]interface{}) {
			switch k := k.(type) {
			case string:
				driverOpts[k] = v
			}
		}
	}
	for k, v := range params {
		driverOpts[k] = v
	}
	return driverOpts
}

func (i *dockerMachineIaaS) DeleteMachine(m *iaas.Machine) error {
	dockerMachine, err := i.apiFactory(DockerMachineConfig{})
	if err != nil {
		return err
	}
	defer dockerMachine.Close()
	return dockerMachine.DeleteMachine(m)
}

func (i *dockerMachineIaaS) Describe() string {
	return `DockerMachine IaaS required params:
  name: host name of the machine to be created
  driver: name of the docker machine driver to be used`
}
