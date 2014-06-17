// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package provision provides interfaces that need to be satisfied in order to
// implement a new iaas on tsuru.
package iaas

// IaaS VirtualMachine representation
type Machine interface {
	IsAvailable() bool
	GetAddress() string
}

// IaaS is the basic interface of this package.
//
// Any tsuru IaaS must implement this interface.
type Iaas interface {
	// IaaS is called when tsuru is creating a Virtual Machine.
	CreateVirtualMachine(params map[string]interface{}) (Machine, error)

	// IaaS is called when tsuru is destroying a Virtual Machine.
	DeleteVirtualMachine(params map[string]interface{}) error

	// Iaas is called when tsuru is listing Virtual Machines.
	ListVirtualMachines(params map[string]interface{}) error
}
