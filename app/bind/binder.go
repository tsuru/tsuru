// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package bind provides interfaces and types for use when binding an app to a
// service.
package bind

import (
	"io"

	bindTypes "github.com/tsuru/tsuru/types/bind"
)

type SetEnvArgs struct {
	Envs          []bindTypes.EnvVar
	Writer        io.Writer
	ManagedBy     string
	PruneUnused   bool
	ShouldRestart bool
}

type UnsetEnvArgs struct {
	VariableNames []string
	Writer        io.Writer
	ShouldRestart bool
}

type AddInstanceArgs struct {
	Envs          []bindTypes.ServiceEnvVar
	Writer        io.Writer
	ShouldRestart bool
}

type RemoveInstanceArgs struct {
	ServiceName   string
	InstanceName  string
	Writer        io.Writer
	ShouldRestart bool
}
