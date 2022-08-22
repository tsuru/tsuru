// Copyright 2022 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import "context"

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

type AppEnvVarService interface {
	List(ctx context.Context, appName string) ([]EnvVar, error)
}

type AppEnvVarStorage interface {
	List(ctx context.Context, appName string) ([]EnvVar, error)
	ListServiceEnvs(ctx context.Context, appName string) ([]ServiceEnvVar, error)
}
