// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package provision provides interfaces that need to be satisfied in order to
// implement a new iaas on tsuru.
package iaas

import (
	"fmt"
)

// IaaS VirtualMachine representation
type Machine interface {
	IsAvailable() bool
	GetAddress() string
}

// IaaS is the basic interface of this package.
//
// Any tsuru IaaS must implement this interface.
type IaaS interface {
	// IaaS is called when tsuru is creating a Virtual Machine.
	CreateMachine(params map[string]interface{}) (Machine, error)

	// IaaS is called when tsuru is destroying a Virtual Machine.
	DeleteMachine(params map[string]interface{}) error

	// IaaS is called when tsuru is listing Virtual Machines.
	ListMachines(params map[string]interface{}) error
}

var iaasProviders = make(map[string]IaaS)

func RegisterIaasProvider(name string, iaas IaaS) {
	iaasProviders[name] = iaas
}

func GetIaasProvider(name string) (IaaS, error) {
	provider, ok := iaasProviders[name]
	if !ok {
		return nil, fmt.Errorf("IaaS provider %q not registered", name)
	}
	return provider, nil
}
