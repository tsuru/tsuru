// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package bind provides interfaces and types for use when binding an app to a
// service.
package bind

import "io"

// EnvVar represents a environment variable for an app.
type EnvVar struct {
	Name         string `json:"name"`
	Value        string `json:"value"`
	Public       bool   `json:"public"`
	InstanceName string `json:"-"`
}

// Unit represents an application unit to be used in binds.
type Unit interface {
	GetID() string
	GetIp() string
}

type App interface {
	// GetIp returns the app ip.
	GetIp() string

	// GetName returns the app name.
	GetName() string

	// GetUnits returns the app units.
	GetUnits() ([]Unit, error)

	// InstanceEnv returns the app enviroment variables.
	InstanceEnv(string) map[string]EnvVar

	// SetEnvs adds enviroment variables in the app.
	SetEnvs(setEnvs SetEnvApp, w io.Writer) error

	// UnsetEnvs removes the given enviroment variables from the app.
	UnsetEnvs(unsetEnvs UnsetEnvApp, w io.Writer) error

	// AddInstance adds an instance to the application.
	AddInstance(instanceApp InstanceApp, writer io.Writer) error

	// RemoveInstance removes an instance from the application.
	RemoveInstance(instanceApp InstanceApp, writer io.Writer) error
}

type ServiceInstance struct {
	Name string            `json:"instance_name"`
	Envs map[string]string `json:"envs"`
}

type SetEnvApp struct {
	Envs          []EnvVar
	PublicOnly    bool
	ShouldRestart bool
}

type UnsetEnvApp struct {
	VariableNames []string
	PublicOnly    bool
	ShouldRestart bool
}

type InstanceApp struct {
	ServiceName   string
	Instance      ServiceInstance
	ShouldRestart bool
}
