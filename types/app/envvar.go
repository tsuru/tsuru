// Copyright 2022 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"context"
	"io"
)

const TsuruServicesEnvVarName = "TSURU_SERVICES"

type EnvVar struct {
	Name      string `json:"name"`
	Value     string `json:"value"`
	Alias     string `json:"alias"`
	ManagedBy string `json:"managedBy,omitempty"`
	Public    bool   `json:"public"`
}

type ServiceEnvVar struct {
	EnvVar       `bson:",inline"`
	ServiceName  string `json:"-"`
	InstanceName string `json:"-"`
}

type SetEnvArgs struct {
	Writer        io.Writer
	ManagedBy     string
	PruneUnused   bool
	ShouldRestart bool
}

type UnsetEnvArgs struct {
	Writer        io.Writer
	ShouldRestart bool
}

type AppEnvVarService interface {
	List(ctx context.Context, a App) ([]EnvVar, error)
	Set(ctx context.Context, a App, envs []EnvVar, args SetEnvArgs) error
	Unset(ctx context.Context, a App, envs []string, args UnsetEnvArgs) error
}

type AppEnvVarStorage interface {
	ListAppEnvs(ctx context.Context, a App) ([]EnvVar, error)
	UpdateAppEnvs(ctx context.Context, a App, envs []EnvVar) error
	RemoveAppEnvs(ctx context.Context, a App, envs []string) error

	ListServiceEnvs(ctx context.Context, a App) ([]ServiceEnvVar, error)
}
