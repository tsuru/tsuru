// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dockermachine

import (
	"bytes"
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/iaas"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/provision"
)

var errDriverNotSet = errors.Errorf("driver is mandatory")
var invalidHostnameChars = regexp.MustCompile(`[^a-z0-9-]`)

func init() {
	iaas.RegisterIaasProvider("dockermachine", newDockerMachineIaaS)
}

type dockerMachineIaaS struct {
	base       iaas.UserDataIaaS
	apiFactory func(DockerMachineConfig) (DockerMachineAPI, error)
}

func newDockerMachineIaaS(name string) iaas.IaaS {
	return &dockerMachineIaaS{
		base:       iaas.UserDataIaaS{NamedIaaS: iaas.NamedIaaS{BaseIaaSName: "dockermachine", IaaSName: name}},
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
	caPath, _ := i.base.GetConfigString("ca-path")
	driverName, ok := params["driver"]
	if !ok {
		name, errConf := i.base.GetConfigString("driver:name")
		if errConf != nil {
			return nil, errDriverNotSet
		}
		driverName = name
		params["driver"] = driverName
	}
	dockerEngineInstallURL, _ := i.getParamOrConfigString("docker-install-url", params)
	dockerEngineStorageDriver, _ := i.getParamOrConfigString("docker-storage-driver", params)
	insecureRegistry, _ := i.getParamOrConfigString("insecure-registry", params)
	var engineFlags []string
	if f, err := i.getParamOrConfigString("docker-flags", params); err == nil {
		engineFlags = strings.Split(f, ",")
	}
	machineName, ok := params["name"]
	if !ok {
		name, err := generateMachineName(params[provision.PoolMetadataName])
		if err != nil {
			return nil, err
		}
		machineName = name
	} else {
		delete(params, "name")
	}
	userDataFileParam, err := i.getParamOrConfigString("user-data-file-param", params)
	if err != nil {
		userDataFileParam, err = i.base.GetConfigString("driver:user-data-file-param")
	}
	if err == nil {
		f, errTemp := ioutil.TempFile("", "")
		if errTemp != nil {
			return nil, errors.Wrap(errTemp, "failed to create userdata file")
		}
		defer os.RemoveAll(f.Name())
		userData, errData := i.base.ReadUserData(params)
		if errData != nil {
			return nil, errors.WithMessage(errData, "failed to read userdata")
		}
		_, errWrite := f.WriteString(userData)
		if errWrite != nil {
			return nil, errors.Wrap(errWrite, "failed to write local userdata file")
		}
		params[userDataFileParam] = f.Name()
	}
	driverOpts := i.buildDriverOpts(driverName, params)
	if userDataFileParam != "" {
		delete(params, userDataFileParam)
	}
	buf := &bytes.Buffer{}
	debugConf, _ := i.base.GetConfigString("debug")
	if debugConf == "" {
		debugConf = "false"
	}
	isDebug, err := strconv.ParseBool(debugConf)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse debug config")
	}
	dockerMachine, err := i.apiFactory(DockerMachineConfig{
		CaPath:    caPath,
		OutWriter: buf,
		ErrWriter: buf,
		IsDebug:   isDebug,
	})
	if err != nil {
		return nil, err
	}
	defer func() {
		dockerMachine.Close()
		log.Debug(buf.String())
	}()
	m, err := dockerMachine.CreateMachine(CreateMachineOpts{
		Name:                      machineName,
		DriverName:                driverName,
		Params:                    driverOpts,
		InsecureRegistry:          insecureRegistry,
		DockerEngineInstallURL:    dockerEngineInstallURL,
		DockerEngineStorageDriver: dockerEngineStorageDriver,
		ArbitraryFlags:            engineFlags,
	})
	if err != nil {
		if m != nil {
			errRem := dockerMachine.DeleteMachine(m.Base)
			if errRem != nil {
				err = tsuruErrors.NewMultiError(err, errors.WithMessage(errRem, "failed to remove machine after error"))
			}
		}
		return nil, err
	}
	m.Base.CreationParams = params
	return m.Base, nil
}

func (i *dockerMachineIaaS) buildDriverOpts(driverName string, params map[string]string) map[string]interface{} {
	driverOpts := DefaultParamsForDriver(driverName)
	config, _ := i.base.GetConfig("driver:options")
	if config != nil {
		for k, v := range config.(map[interface{}]interface{}) {
			switch k := k.(type) {
			case string:
				value, _ := i.base.GetConfig(fmt.Sprintf("driver:options:%s", k))
				switch value := value.(type) {
				case string:
					driverOpts[k] = value
				default:
					driverOpts[k] = v
				}
			}
		}
	}
	for k, v := range params {
		driverOpts[k] = v
	}
	return driverOpts
}

func (i *dockerMachineIaaS) DeleteMachine(m *iaas.Machine) error {
	buf := &bytes.Buffer{}
	debugConf, _ := i.base.GetConfigString("debug")
	if debugConf == "" {
		debugConf = "false"
	}
	isDebug, err := strconv.ParseBool(debugConf)
	if err != nil {
		return errors.Wrap(err, "failed to parse debug config")
	}
	dockerMachine, err := i.apiFactory(DockerMachineConfig{
		OutWriter: buf,
		ErrWriter: buf,
		IsDebug:   isDebug,
	})
	if err != nil {
		return err
	}
	defer func() {
		dockerMachine.Close()
		log.Debug(buf.String())
	}()
	return dockerMachine.DeleteMachine(m)
}

func generateMachineName(prefix string) (string, error) {
	r := strings.NewReplacer("_", "-", " ", "-")
	prefix = r.Replace(prefix)
	prefix = strings.TrimPrefix(prefix, "-")
	prefix = invalidHostnameChars.ReplaceAllString(prefix, "")
	name, err := generateRandomID()
	if err != nil {
		return "", err
	}
	if prefix != "" {
		name = fmt.Sprintf("%s-%s", prefix, name)
	}
	if len(name) > 63 {
		name = name[:63]
	}
	return name, nil
}

func generateRandomID() (string, error) {
	id := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, id); err != nil {
		return "", errors.Wrap(err, "failed to generate random id")
	}
	str := string(id[0]%('z'-'a') + 'a')
	encoded := base32.StdEncoding.EncodeToString(id[1:])
	return str + strings.ToLower(strings.TrimRight(encoded, "=")), nil
}

func (i *dockerMachineIaaS) Describe() string {
	return `DockerMachine IaaS required params:
  driver=<driver>                         Driver to be used by docker machine. Can be set on the IaaS configuration.

Optional params:
  name=<name>                                  Hostname for the created machine.
  docker-install-url=<docker-install-url>      Remote script to be used for docker installation. Defaults to: http://get.docker.com. Can be set on the IaaS configuration.
  insecure-registry=<insecure-registry>        Registry to be added as insecure-registry to the docker engine. Can be set on the IaaS configuration.
  docker-flags=<flag1,flag2>                   Arbitrary docker engine flags. Can be set on the IaaS configuration.
  docker-storage-driver<=docker-storage-driver Docker engine storage driver.
  user-data-file-param                         Name of the userdata driver parameter.
`
}
