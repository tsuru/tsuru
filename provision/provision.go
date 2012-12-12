// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package provision provides interfaces that need to be satisfied in order to
// implement a new provisioner on tsuru.
package provision

import (
	"fmt"
	"io"
)

const (
	StatusStarted    = "started"
	StatusPending    = "pending"
	StatusDown       = "down"
	StatusError      = "error"
	StatusInstalling = "installing"
	StatusCreating   = "creating"
)

// Unit represents a provision unit. Can be a machine, container or anything
// IP-addressable.
type Unit struct {
	Name    string
	AppName string
	Type    string
	Machine int
	Ip      string
	Status  string
}

// AppUnit represents a unit in an app.
type AppUnit interface {
	// Returns the name of the unit.
	GetName() string

	// Returns the number of the unit.
	GetMachine() int
}

// App represents a tsuru app.
//
// It contains only relevant information for provisioning.
type App interface {
	// Log should be used to log messages in the app.
	Log(message, source string) error

	// GetName returns the name of the app.
	GetName() string

	// GetFramework returns the framework (type) of the app. It is equivalent
	// to the Unit `Type` field.
	GetFramework() string

	// GetUnits returns all units of the app, in a slice.
	GetUnits() []AppUnit
}

// Provisioner is the basic interface of this package.
//
// Any tsuru provisioner must implement this interface in order to provision
// tsuru apps.
//
// Tsuru comes with a default provisioner: juju. One can add other provisioners
// by satisfying this interface and registering it using the function Register.
type Provisioner interface {
	// Provision is called when tsuru is creating the app.
	Provision(App) *Error

	// Destroy is called when tsuru is destroying the app.
	Destroy(App) *Error

	// ExecuteCommand runs a command in all units of the app.
	ExecuteCommand(io.Writer, App, string, ...string) error

	// CollectStatus returns information about all provisioned units. It's used
	// by tsuru collector when updating the status of apps in the database.
	CollectStatus() ([]Unit, error)
}

var provisioners = make(map[string]Provisioner)

// Register registers a new provisioner in the Provisioner registry.
func Register(name string, p Provisioner) {
	provisioners[name] = p
}

// Get gets the named provisioner from the registry.
func Get(name string) (Provisioner, error) {
	p, ok := provisioners[name]
	if !ok {
		return nil, fmt.Errorf("Unknown provisioner: %q.", name)
	}
	return p, nil
}

type Error struct {
	Reason string
	Err    error
}

func (e *Error) Error() string {
	var err string
	if e.Err != nil {
		err = e.Err.Error() + ": " + e.Reason
	} else {
		err = e.Reason
	}
	return err
}
