// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dockermachine

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/docker/machine/libmachine"
	"github.com/docker/machine/libmachine/drivers"
	"github.com/docker/machine/libmachine/drivers/rpc"
	"github.com/tsuru/tsuru/iaas"
)

type DockerMachine struct {
	io.Closer
	client libmachine.API
	config *DockerMachineConfig
	path   string
}

type DockerMachineConfig struct {
	CaPath                 string
	InsecureRegistry       string
	DockerEngineInstallURL string
}

type dockerMachineAPI interface {
	io.Closer
	CreateMachine(string, string, map[string]interface{}) (*iaas.Machine, error)
	DeleteMachine(*iaas.Machine) error
}

func NewDockerMachine(config DockerMachineConfig) (dockerMachineAPI, error) {
	path, err := ioutil.TempDir("", "")
	if err != nil {
		return nil, err
	}
	if config.CaPath != "" {
		err = copy(filepath.Join(config.CaPath, "ca.pem"), filepath.Join(path, "ca.pem"))
		if err != nil {
			return nil, err
		}
		err = copy(filepath.Join(config.CaPath, "ca-key.pem"), filepath.Join(path, "ca-key.pem"))
		if err != nil {
			return nil, err
		}
	}
	return &DockerMachine{
		path:   path,
		client: libmachine.NewClient(path, path),
		config: &config,
	}, nil
}

func (d *DockerMachine) Close() error {
	os.RemoveAll(d.path)
	return d.client.Close()
}

func (d *DockerMachine) CreateMachine(name, driver string, params map[string]interface{}) (*iaas.Machine, error) {
	rawDriver, err := json.Marshal(&drivers.BaseDriver{
		MachineName: name,
		StorePath:   d.path,
	})
	if err != nil {
		return nil, err
	}
	host, err := d.client.NewHost(driver, rawDriver)
	if err != nil {
		return nil, err
	}
	err = configureDriver(host.Driver, params)
	if err != nil {
		return nil, err
	}
	engineOpts := host.HostOptions.EngineOptions
	if d.config.InsecureRegistry != "" {
		engineOpts.InsecureRegistry = []string{d.config.InsecureRegistry}
	}
	if d.config.DockerEngineInstallURL != "" {
		engineOpts.InstallURL = d.config.DockerEngineInstallURL
	}
	err = d.client.Create(host)
	if err != nil {
		return nil, err
	}
	ip, err := host.Driver.GetIP()
	if err != nil {
		return nil, err
	}
	rawDriver, err = json.Marshal(host.Driver)
	if err != nil {
		return nil, err
	}
	var driverData map[string]interface{}
	err = json.Unmarshal(rawDriver, &driverData)
	if err != nil {
		return nil, err
	}
	m := &iaas.Machine{
		Id:         host.Name,
		Address:    ip,
		Port:       2376,
		Protocol:   "https",
		CustomData: driverData,
	}
	if host.AuthOptions() != nil {
		m.CaCert, err = ioutil.ReadFile(host.AuthOptions().CaCertPath)
		if err != nil {
			return nil, err
		}
		m.ClientCert, err = ioutil.ReadFile(host.AuthOptions().ClientCertPath)
		if err != nil {
			return nil, err
		}
		m.ClientKey, err = ioutil.ReadFile(host.AuthOptions().ClientKeyPath)
		if err != nil {
			return nil, err
		}
	}
	return m, nil
}

func (d *DockerMachine) DeleteMachine(m *iaas.Machine) error {
	rawDriver, err := json.Marshal(m.CustomData)
	if err != nil {
		return err
	}
	host, err := d.client.NewHost(m.CreationParams["driver"], rawDriver)
	if err != nil {
		return err
	}
	err = host.Driver.Remove()
	if err != nil {
		return err
	}
	return d.client.Remove(m.Id)
}

func configureDriver(driver drivers.Driver, driverOpts map[string]interface{}) error {
	opts := &rpcdriver.RPCFlags{Values: driverOpts}
	for _, c := range driver.GetCreateFlags() {
		_, ok := opts.Values[c.String()]
		if !ok {
			opts.Values[c.String()] = c.Default()
			if c.Default() == nil {
				opts.Values[c.String()] = false
			}
		}
	}
	if err := driver.SetConfigFromFlags(opts); err != nil {
		return fmt.Errorf("Error setting driver configurations: %s", err)
	}
	return nil
}

func copy(src, dst string) error {
	fileSrc, err := ioutil.ReadFile(src)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(dst, fileSrc, 0644)
}
