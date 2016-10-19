// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dockermachine

import (
	"encoding/json"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/docker/machine/libmachine"
	"github.com/docker/machine/libmachine/drivers"
	"github.com/docker/machine/libmachine/drivers/rpc"
	"github.com/docker/machine/libmachine/engine"
	"github.com/docker/machine/libmachine/host"
	"github.com/docker/machine/libmachine/log"
	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/iaas"
)

var defaultWriter = ioutil.Discard

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
	OutWriter              io.Writer
	ErrWriter              io.Writer
}

type dockerMachineAPI interface {
	io.Closer
	CreateMachine(string, string, map[string]interface{}) (*iaas.Machine, error)
	DeleteMachine(*iaas.Machine) error
}

func NewDockerMachine(config DockerMachineConfig) (dockerMachineAPI, error) {
	path, err := ioutil.TempDir("", "")
	if err != nil {
		return nil, errors.Wrap(err, "failed to create temp dir")
	}
	if config.CaPath != "" {
		err = copy(filepath.Join(config.CaPath, "ca.pem"), filepath.Join(path, "ca.pem"))
		if err != nil {
			return nil, errors.WithMessage(err, "failed to copy ca file")
		}
		err = copy(filepath.Join(config.CaPath, "ca-key.pem"), filepath.Join(path, "ca-key.pem"))
		if err != nil {
			return nil, errors.WithMessage(err, "failed to copy ca key file")
		}
	}
	if config.OutWriter != nil {
		log.SetOutWriter(config.OutWriter)
	} else {
		log.SetOutWriter(defaultWriter)
	}
	if config.ErrWriter != nil {
		log.SetOutWriter(config.ErrWriter)
	} else {
		log.SetOutWriter(defaultWriter)
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
	host, err := d.CreateHost(CreateHostOpts{
		Name:       name,
		DriverName: driver,
		Params:     params,
	})
	if err != nil {
		return nil, err
	}
	rawDriver, err := json.Marshal(host.Driver)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal host driver")
	}
	var driverData map[string]interface{}
	err = json.Unmarshal(rawDriver, &driverData)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal host driver")
	}
	m := &iaas.Machine{
		Id:         host.Name,
		Port:       engine.DefaultPort,
		Protocol:   "https",
		CustomData: driverData,
	}
	m.Address, err = host.Driver.GetIP()
	if err != nil {
		return m, errors.Wrap(err, "failed to retrive host ip")
	}
	if host.AuthOptions() != nil {
		m.CaCert, err = ioutil.ReadFile(host.AuthOptions().CaCertPath)
		if err != nil {
			return m, errors.Wrap(err, "failed to read host ca cert")
		}
		m.ClientCert, err = ioutil.ReadFile(host.AuthOptions().ClientCertPath)
		if err != nil {
			return m, errors.Wrap(err, "failed to read host client cert")
		}
		m.ClientKey, err = ioutil.ReadFile(host.AuthOptions().ClientKeyPath)
		if err != nil {
			return m, errors.Wrap(err, "failed to read host client key")
		}
	}
	return m, nil
}

type CreateHostOpts struct {
	Name       string
	DriverName string
	Params     map[string]interface{}
}

func (d *DockerMachine) CreateHost(opts CreateHostOpts) (*host.Host, error) {
	rawDriver, err := json.Marshal(&drivers.BaseDriver{
		MachineName: opts.Name,
		StorePath:   d.path,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal base driver")
	}
	host, err := d.client.NewHost(opts.DriverName, rawDriver)
	if err != nil {
		return nil, errors.Wrap(err, "failed to initialize host")
	}
	err = configureDriver(host.Driver, opts.Params)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to configure driver")
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
		return nil, errors.Wrap(err, "failed to create host")
	}
	return host, nil
}

func (d *DockerMachine) DeleteMachine(m *iaas.Machine) error {
	rawDriver, err := json.Marshal(m.CustomData)
	if err != nil {
		return errors.Wrap(err, "failed to marshal machine data")
	}
	host, err := d.client.NewHost(m.CreationParams["driver"], rawDriver)
	if err != nil {
		return errors.Wrap(err, "failed to initialize host")
	}
	err = host.Driver.Remove()
	if err != nil {
		return errors.Wrap(err, "failed to remove host")
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
	err := driver.SetConfigFromFlags(opts)
	return errors.Wrap(err, "failed to set driver configuration")
}

func copy(src, dst string) error {
	fileSrc, err := ioutil.ReadFile(src)
	if err != nil {
		return errors.Wrapf(err, "failed to read %s", src)
	}
	err = ioutil.WriteFile(dst, fileSrc, 0644)
	return errors.Wrapf(err, "failed to write %s", dst)
}
