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
	client    libmachine.API
	StorePath string
	CertsPath string
	temp      bool
}

type DockerMachineConfig struct {
	CaPath    string
	OutWriter io.Writer
	ErrWriter io.Writer
	StorePath string
}

type DockerMachineAPI interface {
	io.Closer
	CreateMachine(CreateMachineOpts) (*Machine, error)
	DeleteMachine(*iaas.Machine) error
	DeleteAll() error
}

type CreateMachineOpts struct {
	Name                   string
	DriverName             string
	Params                 map[string]interface{}
	InsecureRegistry       string
	DockerEngineInstallURL string
	RegistryMirror         string
}

type Machine struct {
	Base *iaas.Machine
	Host *host.Host
}

func NewDockerMachine(config DockerMachineConfig) (DockerMachineAPI, error) {
	storePath := config.StorePath
	temp := false
	if storePath == "" {
		tempPath, err := ioutil.TempDir("", "")
		if err != nil {
			return nil, errors.Wrap(err, "failed to create temp dir")
		}
		storePath = tempPath
		temp = true
	}
	certsPath := filepath.Join(storePath, "certs")
	if _, err := os.Stat(certsPath); os.IsNotExist(err) {
		err := os.MkdirAll(certsPath, 0700)
		if err != nil {
			return nil, errors.WithMessage(err, "failed to create certs dir")
		}
	}
	if config.CaPath != "" {
		err := copy(filepath.Join(config.CaPath, "ca.pem"), filepath.Join(certsPath, "ca.pem"))
		if err != nil {
			return nil, errors.WithMessage(err, "failed to copy ca file")
		}
		err = copy(filepath.Join(config.CaPath, "ca-key.pem"), filepath.Join(certsPath, "ca-key.pem"))
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
		StorePath: storePath,
		CertsPath: certsPath,
		client:    libmachine.NewClient(storePath, certsPath),
		temp:      temp,
	}, nil
}

func (d *DockerMachine) Close() error {
	if d.temp {
		os.RemoveAll(d.StorePath)
	}
	return d.client.Close()
}

func (d *DockerMachine) CreateMachine(opts CreateMachineOpts) (*Machine, error) {
	rawDriver, err := json.Marshal(&drivers.BaseDriver{
		MachineName: opts.Name,
		StorePath:   d.StorePath,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal base driver")
	}
	h, err := d.client.NewHost(opts.DriverName, rawDriver)
	if err != nil {
		return nil, errors.Wrap(err, "failed to initialize host")
	}
	err = configureDriver(h.Driver, opts.Params)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to configure driver")
	}
	engineOpts := h.HostOptions.EngineOptions
	if opts.InsecureRegistry != "" {
		engineOpts.InsecureRegistry = []string{opts.InsecureRegistry}
	}
	if opts.DockerEngineInstallURL != "" {
		engineOpts.InstallURL = opts.DockerEngineInstallURL
	}
	if opts.RegistryMirror != "" {
		engineOpts.RegistryMirror = []string{opts.RegistryMirror}
	}
	err = d.client.Create(h)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create host")
	}
	rawDriver, err = json.Marshal(h.Driver)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal host driver")
	}
	var driverData map[string]interface{}
	err = json.Unmarshal(rawDriver, &driverData)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal host driver")
	}
	m := &Machine{
		Base: &iaas.Machine{
			Id:         h.Name,
			Port:       engine.DefaultPort,
			Protocol:   "https",
			CustomData: driverData,
		},
		Host: h,
	}
	m.Base.Address, err = h.Driver.GetIP()
	if err != nil {
		return m, errors.Wrap(err, "failed to retrive host ip")
	}
	if h.AuthOptions() != nil {
		m.Base.CaCert, err = ioutil.ReadFile(h.AuthOptions().CaCertPath)
		if err != nil {
			return m, errors.Wrap(err, "failed to read host ca cert")
		}
		m.Base.ClientCert, err = ioutil.ReadFile(h.AuthOptions().ClientCertPath)
		if err != nil {
			return m, errors.Wrap(err, "failed to read host client cert")
		}
		m.Base.ClientKey, err = ioutil.ReadFile(h.AuthOptions().ClientKeyPath)
		if err != nil {
			return m, errors.Wrap(err, "failed to read host client key")
		}
	}
	return m, nil
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

func (d *DockerMachine) DeleteAll() error {
	hosts, err := d.client.List()
	if err != nil {
		return err
	}
	for _, n := range hosts {
		h, errLoad := d.client.Load(n)
		if errLoad != nil {
			return errLoad
		}
		err = h.Driver.Remove()
		if err != nil {
			return err
		}
	}
	return os.RemoveAll(d.StorePath)
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
