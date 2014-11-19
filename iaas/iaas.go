// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package provision provides interfaces that need to be satisfied in order to
// implement a new iaas on tsuru.
package iaas

import (
	"fmt"

	"github.com/tsuru/config"
)

const (
	UserData = `#!/bin/bash
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

type CustomIaaS interface {
	IaaS
	Clone(string) IaaS
}

type NamedIaaS struct {
	BaseIaaSName string
	IaaSName     string
}

func (i *NamedIaaS) GetConfigString(name string) (string, error) {
	val, err := config.GetString(fmt.Sprintf("iaas:custom:%s:%s", i.IaaSName, name))
	if err != nil {
		val, err = config.GetString(fmt.Sprintf("iaas:%s:%s", i.BaseIaaSName, name))
	}
	return val, err
}

var iaasProviders = make(map[string]IaaS)

func RegisterIaasProvider(name string, iaas IaaS) {
	iaasProviders[name] = iaas
}

func getIaasProvider(name string) (IaaS, error) {
	provider, ok := iaasProviders[name]
	if !ok {
		customProvider, err := config.GetString(fmt.Sprintf("iaas:custom:%s:provider", name))
		if err != nil {
			return nil, fmt.Errorf("IaaS provider %q not registered", name)
		}
		originalProvider, ok := iaasProviders[customProvider]
		if !ok {
			return nil, fmt.Errorf("IaaS provider %q based on %q not registered", name, customProvider)
		}
		customIaaS, isValid := originalProvider.(CustomIaaS)
		if !isValid {
			return nil, fmt.Errorf("IaaS provider %q does not allow clonning", customProvider)
		}
		cloned := customIaaS.Clone(name)
		RegisterIaasProvider(name, cloned)
		return cloned, nil
	}
	return provider, nil
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
