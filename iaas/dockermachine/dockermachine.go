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
	"strconv"
	"strings"

	"github.com/docker/machine/libmachine"
	"github.com/docker/machine/libmachine/drivers"
	rpcdriver "github.com/docker/machine/libmachine/drivers/rpc"
	"github.com/docker/machine/libmachine/engine"
	"github.com/docker/machine/libmachine/host"
	"github.com/docker/machine/libmachine/log"
	"github.com/docker/machine/libmachine/mcnflag"
	"github.com/docker/machine/libmachine/mcnutils"
	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/iaas"
)

type DockerMachine struct {
	io.Closer
	client    libmachine.API
	StorePath string
	CertsPath string
	temp      bool
}

type DockerMachineConfig struct {
	CaPath    string
	StorePath string
	IsDebug   bool
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
	Name                      string
	DriverName                string
	Params                    map[string]interface{}
	InsecureRegistry          string
	DockerEngineInstallURL    string
	RegistryMirror            string
	DockerEngineStorageDriver string
	ArbitraryFlags            []string
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

func InitLogging(outWriter, errorWriter io.Writer, isDebug bool) {
	if outWriter != nil {
		log.SetOutWriter(outWriter)
	} else {
		log.SetOutWriter(ioutil.Discard)
	}
	if errorWriter != nil {
		log.SetOutWriter(errorWriter)
	} else {
		log.SetOutWriter(ioutil.Discard)
	}
	log.SetDebug(isDebug)
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
		err := mcnutils.CopyFile(filepath.Join(config.CaPath, "ca.pem"), filepath.Join(certsPath, "ca.pem"))
		if err != nil {
			return nil, errors.Wrap(err, "failed to copy ca file")
		}
		err = mcnutils.CopyFile(filepath.Join(config.CaPath, "ca-key.pem"), filepath.Join(certsPath, "ca-key.pem"))
		if err != nil {
			return nil, errors.Wrap(err, "failed to copy ca key file")
		}
	}
	client := libmachine.NewClient(storePath, certsPath)
	client.IsDebug = config.IsDebug
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
	if opts.DockerEngineStorageDriver != "" {
		engineOpts.StorageDriver = opts.DockerEngineStorageDriver
	}
	engineOpts.ArbitraryFlags = opts.ArbitraryFlags
	if h.AuthOptions() != nil {
		h.AuthOptions().StorePath = d.StorePath
	}
	errCreate := d.client.Create(h)
	machine, err := newMachine(h)
	if errCreate != nil {
		return machine, errors.Wrap(errCreate, "failed to create host")
	}
	return machine, errors.Wrap(err, "failed to create machine")
}

func (d *DockerMachine) DeleteMachine(m *iaas.Machine) error {
	host, err := d.hostFromCustomData(m.CreationParams["driver"], m.CustomData)
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
	h, err := d.hostFromCustomData(opts.DriverName, opts.Base.CustomData)
	if err != nil {
		return nil, err
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
			CreationParams: map[string]string{
				"driver": h.DriverName,
			},
		},
		Host: h,
	}
	address, err := h.Driver.GetIP()
	if err != nil || address == "" {
		return nil, errors.Wrap(err, "failed to retrieve host ip")
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
		val, ok := opts.Values[c.String()]
		if !ok {
			opts.Values[c.String()] = c.Default()
			if c.Default() == nil {
				opts.Values[c.String()] = false
			}
		} else {
			if strVal, ok := val.(string); ok {
				switch c.(type) {
				case *mcnflag.StringSliceFlag, mcnflag.StringSliceFlag:
					opts.Values[c.String()] = strings.Split(strVal, ",")
				case *mcnflag.IntFlag, mcnflag.IntFlag:
					v, err := strconv.Atoi(strVal)
					if err != nil {
						return errors.Wrapf(err, "failed to set %s flag: %s is not an int", c.String(), strVal)
					}
					opts.Values[c.String()] = v
				case *mcnflag.BoolFlag, mcnflag.BoolFlag:
					v, err := strconv.ParseBool(strVal)
					if err != nil {
						return errors.Wrapf(err, "failed to set %s flag: %s is not a bool", c.String(), strVal)
					}
					opts.Values[c.String()] = v
				}
			}
		}
	}
	err := driver.SetConfigFromFlags(opts)
	return errors.Wrap(err, "failed to set driver configuration")
}

func (d *DockerMachine) hostFromCustomData(driverName string, data map[string]interface{}) (*host.Host, error) {
	if driverName == "cloudstack" && data != nil {
		// In recent versions of cloudstack driver network is now a string
		// slice. Here we convert from the old representation to the new one.
		for _, field := range []string{"Network", "NetworkID"} {
			netData, hasField := data[field]
			if !hasField {
				continue
			}
			switch v := netData.(type) {
			case string:
				data[field] = []string{v}
			}
		}
	}
	rawDriver, err := json.Marshal(data)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to marshal driver data")
	}
	h, err := d.client.NewHost(driverName, rawDriver)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return h, nil
}
