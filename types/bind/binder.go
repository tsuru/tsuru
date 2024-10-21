// Copyright 2023 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package bind

import "io"

type SetEnvArgs struct {
	Envs          []EnvVar
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
