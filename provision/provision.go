// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package provision provides interfaces that need to be satisfied in order to
// implement a new provisioner on tsuru.
package provision

import (
	"fmt"
	"io"
)

type Status string

func (s Status) String() string {
	return string(s)
}

const (
	StatusStarted    = Status("started")
	StatusPending    = Status("pending")
	StatusDown       = Status("down")
	StatusError      = Status("error")
	StatusInstalling = Status("installing")
	StatusCreating   = Status("creating")
)

// Unit represents a provision unit. Can be a machine, container or anything
// IP-addressable.
type Unit struct {
	Name       string
	AppName    string
	Type       string
	InstanceId string
	Machine    int
	Ip         string
	Status     Status
}

// AppUnit represents a unit in an app.
type AppUnit interface {
	// Returns the name of the unit.
	GetName() string

	// Returns the number of the unit.
	GetMachine() int

	// Returns the status of the unit.
	GetStatus() Status

	// Returns the IP of the unit.
	GetIp() string
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
	ProvisionUnits() []AppUnit
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
	Provision(App) error

	// Destroy is called when tsuru is destroying the app.
	Destroy(App) error

	// AddUnits adds units to an app. The first parameter is the app, the
	// second is the number of units to add.
	//
	// It returns a slice containing all added units
	AddUnits(App, uint) ([]Unit, error)

	// RemoveUnit removes a unit from the app. It receives the app and the name
	// of the unit to be removed.
	RemoveUnit(App, string) error

	// RemoveUnits removes multiple units from an app. The first parameter it
	// the app, the second is the number of units to remove.
	//
	// It returns a slice containing indices of all removed units (the index
	// must match the slice returned by App.ProvisionUnits). The list of
	// indices must be returned sorted.
	RemoveUnits(App, uint) ([]int, error)

	// ExecuteCommand runs a command in all units of the app.
	ExecuteCommand(stdout, stderr io.Writer, app App, cmd string, args ...string) error

	// CollectStatus returns information about all provisioned units. It's used
	// by tsuru collector when updating the status of apps in the database.
	CollectStatus() ([]Unit, error)

	// Addr returns the address for an app. It will probably be a DNS name
	// or IP address.
	//
	// Tsuru will use this method to get the IP (althought it might not be
	// an actual IP, collector calls it "IP") of the app from the
	// provisioner.
	Addr(App) (string, error)
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
