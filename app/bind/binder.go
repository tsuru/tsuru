// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package bind provides interfaces and types for use when binding an app to a
// service.
package bind

import (
	"context"
	"io"
)

// EnvVar represents a environment variable for an app.
type EnvVar struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Alias  string `json:"alias"`
	Public bool   `json:"public"`
}

type ServiceEnvVar struct {
	EnvVar       `bson:",inline"`
	ServiceName  string `json:"-"`
	InstanceName string `json:"-"`
}

// Unit represents an application unit to be used in binds.
type Unit interface {
	GetID() string
	GetIp() string
}

type App interface {
	// GetAddresses returns the app addresses.
	GetAddresses() ([]string, error)

	// GetInternalAddresses returns the app addresses inside the cluster, if any.
	GetInternalAddresses(context.Context) ([]string, error)

	// GetName returns the app name.
	GetName() string

	// GetUUID returns the App v4 UUID
	GetUUID() (string, error)

	// GetUnits returns the app units.
	GetUnits() ([]Unit, error)

	// AddInstance adds an instance to the application.
	AddInstance(args AddInstanceArgs) error

	// RemoveInstance removes an instance from the application.
	RemoveInstance(args RemoveInstanceArgs) error
}

type SetEnvArgs struct {
	Envs          []EnvVar
	Writer        io.Writer
	ShouldRestart bool
}

type UnsetEnvArgs struct {
	VariableNames []string
	Writer        io.Writer
	ShouldRestart bool
}

type AddInstanceArgs struct {
	Envs          []ServiceEnvVar
	Writer        io.Writer
	ShouldRestart bool
}

type RemoveInstanceArgs struct {
	ServiceName   string
	InstanceName  string
	Writer        io.Writer
	ShouldRestart bool
}
