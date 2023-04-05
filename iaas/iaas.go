// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package iaas provides interfaces that need to be satisfied in order to
// implement a new iaas on tsuru.
package iaas

import (
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/pkg/errors"
	"github.com/tsuru/config"
	tsuruNet "github.com/tsuru/tsuru/net"
)

const defaultUserData = `#!/bin/bash
curl -sL https://raw.github.com/tsuru/now/master/run.bash | bash -s -- --docker-only
`

var ErrNoDefaultIaaS = errors.New("no default iaas configured")

// Every Tsuru IaaS must implement this interface.
type IaaS interface {
	// Called when tsuru is creating a Machine.
	CreateMachine(params map[string]string) (*Machine, error)

	// Called when tsuru is destroying a Machine.
	DeleteMachine(m *Machine) error
}

type Describer interface {
	Describe() string
}

type HealthChecker interface {
	HealthCheck() error
}

type InitializableIaaS interface {
	Initialize() error
}

type NamedIaaS struct {
	BaseIaaSName string
	IaaSName     string
}

type UserDataIaaS struct {
	NamedIaaS
}

func (i *UserDataIaaS) ReadUserData(params map[string]string) (string, error) {
	if userData, ok := params["user-data"]; ok {
		return userData, nil
	}
	userDataURL, ok := params["user-data-url"]
	var err error
	if !ok {
		userDataURL, err = i.NamedIaaS.GetConfigString("user-data")
		if err != nil {
			return defaultUserData, nil
		}
	}
	if userDataURL == "" {
		return "", nil
	}
	resp, err := tsuruNet.Dial15Full60ClientNoKeepAlive.Get(userDataURL)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", errors.Errorf("Invalid user-data status code: %d", resp.StatusCode)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func (i *NamedIaaS) GetConfigString(name string) (string, error) {
	val, err := i.GetConfig(name)
	if err != nil || val == nil {
		return "", err
	}
	return fmt.Sprintf("%v", val), nil
}

func (i *NamedIaaS) GetConfig(name string) (interface{}, error) {
	val, err := config.Get(fmt.Sprintf("iaas:custom:%s:%s", i.IaaSName, name))
	if err != nil {
		val, err = config.Get(fmt.Sprintf("iaas:%s:%s", i.BaseIaaSName, name))
	}
	return val, err
}

type iaasFactory func(string) IaaS

var iaasProviders = make(map[string]iaasFactory)
var iaasInstances = make(map[string]IaaS)
var iaasLock sync.Mutex

func RegisterIaasProvider(name string, factory iaasFactory) {
	iaasProviders[name] = factory
}

func getIaasProvider(name string) (IaaS, error) {
	iaasLock.Lock()
	defer iaasLock.Unlock()
	instance, ok := iaasInstances[name]
	if !ok {
		providerName, err := config.GetString(fmt.Sprintf("iaas:custom:%s:provider", name))
		if err != nil {
			providerName = name
		}
		providerFactory, ok := iaasProviders[providerName]
		if !ok {
			return nil, errors.Errorf("IaaS provider %q based on %q not registered", name, providerName)
		}
		instance = providerFactory(name)
		if init, ok := instance.(InitializableIaaS); ok {
			err := init.Initialize()
			if err != nil {
				return nil, err
			}
		}
		iaasInstances[name] = instance
	}
	return instance, nil
}

func Describe(iaasName ...string) (string, error) {
	if len(iaasName) == 0 || iaasName[0] == "" {
		defaultIaaS, err := getDefaultIaasName()
		if err != nil {
			return "", err
		}
		iaasName = []string{defaultIaaS}
	}
	iaas, err := getIaasProvider(iaasName[0])
	if err != nil {
		return "", err
	}
	desc, ok := iaas.(Describer)
	if !ok {
		return "", nil
	}
	return desc.Describe(), nil
}

func getDefaultIaasName() (string, error) {
	defaultIaaS, err := config.GetString("iaas:default")
	if err == nil {
		return defaultIaaS, nil
	}
	ec2ProviderName := "ec2"
	ec2Configured := false
	var configuredIaases []string
	for provider := range iaasProviders {
		if _, err = config.Get(fmt.Sprintf("iaas:%s", provider)); err == nil {
			configuredIaases = append(configuredIaases, provider)
			if provider == ec2ProviderName {
				ec2Configured = true
			}
		}
	}
	c, err := config.Get("iaas:custom")
	if err == nil {
		if v, ok := c.(map[interface{}]interface{}); ok {
			for provider := range v {
				configuredIaases = append(configuredIaases, provider.(string))
			}
		}
	}
	if len(configuredIaases) == 1 {
		return configuredIaases[0], nil
	}
	if ec2Configured {
		return ec2ProviderName, nil
	}
	return "", ErrNoDefaultIaaS
}

func ResetAll() {
	iaasLock.Lock()
	defer iaasLock.Unlock()
	iaasProviders = make(map[string]iaasFactory)
	iaasInstances = make(map[string]IaaS)
}
