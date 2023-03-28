// Copyright 2022 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package job

import (
	bindTypes "github.com/tsuru/tsuru/types/bind"
)

type Job interface {
	GetName() string
	GetPool() string
	GetTeamOwner() string
	GetTeamsName() []string
	GetExecutions() []uint
}

type ContainerInfo struct {
	Name    string
	Image   string
	Command []string
}

type JobSpec struct {
	Completions           *int32
	Parallelism           *int32
	ActiveDeadlineSeconds *int64
	BackoffLimit          *int32
	Schedule              string
	ContainerInfo         ContainerInfo
	ServiceEnvs           []bindTypes.ServiceEnvVar
	Envs                  []bindTypes.EnvVar
}
