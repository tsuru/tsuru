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

var _ ServiceEnvVarIdentifier = (*ServiceEnvVar)(nil)

func (sev ServiceEnvVar) GetServiceName() string {
	return sev.ServiceName
}

func (sev ServiceEnvVar) GetInstanceName() string {
	return sev.InstanceName
}

func (sev ServiceEnvVar) GetEnvVarName() string {
	return sev.EnvVar.Name
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
	Get(ctx context.Context, a App, envName string) (EnvVar, error)
	Set(ctx context.Context, a App, envs []EnvVar, args SetEnvArgs) error
	Unset(ctx context.Context, a App, envs []string, args UnsetEnvArgs) error
}

type AppServiceEnvVarService interface {
	List(ctx context.Context, a App) ([]ServiceEnvVar, error)
	Get(ctx context.Context, a App, envName string) (ServiceEnvVar, error)
	Set(ctx context.Context, a App, envs []ServiceEnvVar, args SetEnvArgs) error
	Unset(ctx context.Context, a App, envNames []string, args UnsetEnvArgs) error
}

type AppEnvVarStorage interface {
	FindAll(ctx context.Context, a App) ([]EnvVar, error)
	Remove(ctx context.Context, a App, envs []string) error
	Upsert(ctx context.Context, a App, envs []EnvVar) error
}

type AppServiceEnvVarStorage interface {
	FindAll(ctx context.Context, a App) ([]ServiceEnvVar, error)
	Remove(ctx context.Context, a App, envNames []ServiceEnvVarIdentifier) error
	Upsert(ctx context.Context, a App, envs []ServiceEnvVar) error
}

type ServiceEnvVarIdentifier interface {
	GetServiceName() string
	GetInstanceName() string
	GetEnvVarName() string
}
