// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package iaas provides interfaces that need to be satisfied in order to
// implement a new iaas on tsuru.
package iaas

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"sync"

	"github.com/tsuru/config"
)

const (
	defaultUserData = `#!/bin/bash
curl -sL https://raw.github.com/tsuru/now/master/run.bash | bash -s -- --docker-only
`
	defaultIaaSProviderName = "ec2"
)

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

func (i *UserDataIaaS) ReadUserData() (string, error) {
	userDataURL, err := i.NamedIaaS.GetConfigString("user-data")
	var userData string
	if err != nil {
		userData = defaultUserData
	} else if userDataURL != "" {
		resp, err := http.Get(userDataURL)
		if err != nil {
			return "", err
		}
		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("Invalid user-data status code: %d", resp.StatusCode)
		}
		defer resp.Body.Close()
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return "", err
		}
		userData = string(body)
	}
	return userData, nil
}

func (i *NamedIaaS) GetConfigString(name string) (string, error) {
	val, err := config.Get(fmt.Sprintf("iaas:custom:%s:%s", i.IaaSName, name))
	if err != nil {
		val, err = config.Get(fmt.Sprintf("iaas:%s:%s", i.BaseIaaSName, name))
	}
	if err != nil || val == nil {
		return "", err
	}
	return fmt.Sprintf("%v", val), nil
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
			return nil, fmt.Errorf("IaaS provider %q based on %q not registered", name, providerName)
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
		defaultIaaS, err := config.GetString("iaas:default")
		if err != nil {
			defaultIaaS = defaultIaaSProviderName
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

func ResetAll() {
	iaasLock.Lock()
	defer iaasLock.Unlock()
	iaasProviders = make(map[string]iaasFactory)
	iaasInstances = make(map[string]IaaS)
}
