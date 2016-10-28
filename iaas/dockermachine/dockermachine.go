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
	RegisterMachine(RegisterMachineOpts) (*Machine, error)
	List() ([]*Machine, error)
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

type RegisterMachineOpts struct {
	Base          *iaas.Machine
	DriverName    string
	SSHPrivateKey []byte
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
	client := libmachine.NewClient(storePath, certsPath)
	if _, err := os.Stat(client.GetMachinesDir()); os.IsNotExist(err) {
		err := os.MkdirAll(client.GetMachinesDir(), 0700)
		if err != nil {
			return nil, errors.WithMessage(err, "failed to create machines dir")
		}
	}
	return &DockerMachine{
		StorePath: storePath,
		CertsPath: certsPath,
		client:    client,
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
	return newMachine(h)
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

// RegisterMachine registers an iaas.Machine as an Machine and a host on
// the current running DockerMachine. It expects all data needed to Marshal
// the host/driver to be available on CustomData.
func (d *DockerMachine) RegisterMachine(opts RegisterMachineOpts) (*Machine, error) {
	if !d.temp {
		return nil, errors.New("register is only available without user defined StorePath")
	}
	if opts.Base.CustomData == nil {
		return nil, errors.New("custom data is required")
	}
	opts.Base.CustomData["SSHKeyPath"] = filepath.Join(d.client.GetMachinesDir(), opts.Base.Id, "id_rsa")
	rawDriver, err := json.Marshal(opts.Base.CustomData)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to marshal driver data")
	}
	h, err := d.client.NewHost(opts.DriverName, rawDriver)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	err = ioutil.WriteFile(h.Driver.GetSSHKeyPath(), opts.SSHPrivateKey, 0700)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	err = ioutil.WriteFile(h.AuthOptions().CaCertPath, opts.Base.CaCert, 0700)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	err = ioutil.WriteFile(h.AuthOptions().ClientCertPath, opts.Base.ClientCert, 0700)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	err = ioutil.WriteFile(h.AuthOptions().ClientKeyPath, opts.Base.ClientKey, 0700)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	err = d.client.Save(h)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	savedHost, err := d.client.Load(h.Name)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return &Machine{
		Base: opts.Base,
		Host: savedHost,
	}, nil
}

func (d *DockerMachine) List() ([]*Machine, error) {
	names, err := d.client.List()
	if err != nil {
		return nil, errors.WithStack(err)
	}
	var machines []*Machine
	for _, n := range names {
		h, err := d.client.Load(n)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		m, err := newMachine(h)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		machines = append(machines, m)
	}
	return machines, nil
}

func newMachine(h *host.Host) (*Machine, error) {
	rawDriver, err := json.Marshal(h.Driver)
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
	address, err := h.Driver.GetIP()
	if err != nil {
		return m, errors.Wrap(err, "failed to retrive host ip")
	}
	m.Base.Address = address
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
