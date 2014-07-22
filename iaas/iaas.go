// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package provision provides interfaces that need to be satisfied in order to
// implement a new iaas on tsuru.
package iaas

import (
	"fmt"
)

const UserData = `#!/bin/bash
curl -sL https://raw.github.com/tsuru/now/master/run.bash | bash -s -- --docker-only
`

// Every Tsuru IaaS must implement this interface.
type IaaS interface {
	// Called when tsuru is creating a Machine.
	CreateMachine(params map[string]string) (*Machine, error)

	// Called when tsuru is destroying a Machine.
	DeleteMachine(m *Machine) error
}

var iaasProviders = make(map[string]IaaS)

func RegisterIaasProvider(name string, iaas IaaS) {
	iaasProviders[name] = iaas
}

func getIaasProvider(name string) (IaaS, error) {
	provider, ok := iaasProviders[name]
	if !ok {
		return nil, fmt.Errorf("IaaS provider %q not registered", name)
	}
	return provider, nil
}
