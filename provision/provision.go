// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package provision provides interfaces that need to be satisfied in order to
// implement a new provisioner on tsuru.
package provision

import (
	"errors"
	"fmt"
	"io"

	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/app/bind"
)

var ErrInvalidStatus = errors.New("invalid status")

var ErrEmptyApp = errors.New("no units for this app")

// Status represents the status of a unit in tsuru.
type Status string

func (s Status) String() string {
	return string(s)
}

func ParseStatus(status string) (Status, error) {
	switch status {
	case "building":
		return StatusBuilding, nil
	case "error":
		return StatusError, nil
	case "down":
		return StatusDown, nil
	case "unreachable":
		return StatusUnreachable, nil
	case "started":
		return StatusStarted, nil
	case "stopped":
		return StatusStopped, nil
	}
	return Status(""), ErrInvalidStatus
}

const (
	// StatusBuilding is the status for units being provisined by the
	// provisioner, like in the deployment.
	StatusBuilding = Status("building")

	// StatusError is the status for units that failed to start, because of
	// an application error.
	StatusError = Status("error")

	// StatusDown is the status for units that failed to start, because of
	// some internal error on tsuru.
	StatusDown = Status("down")

	// StatusUnreachable is the case where the process is up and running,
	// but the unit is not reachable. Probably because it's not bound to
	// the right host ("0.0.0.0") and/or right port ($PORT).
	StatusUnreachable = Status("unreachable")

	// StatusStarting is set when the container is started in docker.
	StatusStarting = Status("starting")

	// StatusStarted is for cases where the unit is up and running, and bound
	// to the proper status, it's set by RegisterUnit and SetUnitStatus.
	StatusStarted = Status("started")

	// StatusStopped is for cases where the unit has been stopped.
	StatusStopped = Status("stopped")
)

// Unit represents a provision unit. Can be a machine, container or anything
// IP-addressable.
type Unit struct {
	Name    string
	AppName string
	Type    string
	Ip      string
	Status  Status
}

// GetIp returns the Unit.IP.
func (u *Unit) GetIp() string {
	return u.Ip
}

// Available returns true if the unit status is started or unreachable.
func (u *Unit) Available() bool {
	return u.Status == StatusStarted || u.Status == StatusUnreachable
}

// Named is something that has a name, providing the GetName method.
type Named interface {
	GetName() string
}

// App represents a tsuru app.
//
// It contains only relevant information for provisioning.
type App interface {
	Named
	// Log should be used to log messages in the app.
	Log(message, source, unit string) error

	// GetPlatform returns the platform (type) of the app. It is equivalent
	// to the Unit `Type` field.
	GetPlatform() string

	// GetDeploy returns the deploys that an app has.
	GetDeploys() uint

	Units() []Unit

	// Run executes the command in app units. Commands executed with this
	// method should have access to environment variables defined in the
	// app.
	Run(cmd string, w io.Writer, once bool) error

	Restart(io.Writer) error

	Envs() map[string]bind.EnvVar

	// Ready marks the app as ready for deployment.
	Ready() error

	GetMemory() int
	GetSwap() int
	GetUpdatePlatform() bool
}

// CNameManager represents a provisioner that supports cname on applications.
type CNameManager interface {
	SetCName(app App, cname string) error
	UnsetCName(app App, cname string) error
}

// ArchiveDeployer is a provisioner that can deploy archives.
type ArchiveDeployer interface {
	ArchiveDeploy(app App, archiveURL string, w io.Writer) error
}

// GitDeployer is a provisioner that can deploy the application from a Git
// repository.
type GitDeployer interface {
	GitDeploy(app App, version string, w io.Writer) error
}

// UploadDeployer is a provisioner that can deploy the application from an
// uploaded file.
type UploadDeployer interface {
	UploadDeploy(app App, file io.ReadCloser, w io.Writer) error
}

// Provisioner is the basic interface of this package.
//
// Any tsuru provisioner must implement this interface in order to provision
// tsuru apps.
type Provisioner interface {
	// Provision is called when tsuru is creating the app.
	Provision(App) error

	// Destroy is called when tsuru is destroying the app.
	Destroy(App) error

	// AddUnits adds units to an app. The first parameter is the app, the
	// second is the number of units to be added.
	//
	// It returns a slice containing all added units
	AddUnits(App, uint) ([]Unit, error)

	// RemoveUnits "undoes" AddUnits, removing the given number of units
	// from the app.
	RemoveUnits(App, uint) error

	// RemoveUnit removes a unit from the app. It receives the unit to be
	// removed.
	RemoveUnit(Unit) error

	// SetUnitStatus changes the status of a unit.
	SetUnitStatus(Unit, Status) error

	// ExecuteCommand runs a command in all units of the app.
	ExecuteCommand(stdout, stderr io.Writer, app App, cmd string, args ...string) error

	// ExecuteCommandOnce runs a command in one unit of the app.
	ExecuteCommandOnce(stdout, stderr io.Writer, app App, cmd string, args ...string) error

	Restart(App) error
	Stop(App) error

	// Start start the app units.
	Start(App) error

	// Addr returns the address for an app.
	//
	// tsuru will use this method to get the IP (although it might not be
	// an actual IP, collector calls it "IP") of the app from the
	// provisioner.
	Addr(App) (string, error)

	// Swap change the router between two apps.
	Swap(App, App) error

	// Units returns information about units by App.
	Units(App) []Unit

	// Register a unit after the container has been created or restarted.
	RegisterUnit(Unit) error
}

// CustomizedDeployPipelineProvisioner is a provisioner with a customized
// deploy pipeline.
type CustomizedDeployPipelineProvisioner interface {
	DeployPipeline() *action.Pipeline
}

// ExtensibleProvisioner is a provisioner where administrators can manage
// platforms (automatically adding, removing and updating platforms).
type ExtensibleProvisioner interface {
	PlatformAdd(name string, args map[string]string, w io.Writer) error
	PlatformUpdate(name string, args map[string]string, w io.Writer) error
	PlatformRemove(name string) error
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
		return nil, fmt.Errorf("unknown provisioner: %q", name)
	}
	return p, nil
}

// Registry returns the list of registered provisioners.
func Registry() []Provisioner {
	registry := make([]Provisioner, 0, len(provisioners))
	for _, p := range provisioners {
		registry = append(registry, p)
	}
	return registry
}

// Error represents a provisioning error. It encapsulates further errors.
type Error struct {
	Reason string
	Err    error
}

// Error is the string representation of a provisioning error.
func (e *Error) Error() string {
	var err string
	if e.Err != nil {
		err = e.Err.Error() + ": " + e.Reason
	} else {
		err = e.Reason
	}
	return err
}
