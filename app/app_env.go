// Copyright 2022 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"context"
	"encoding/json"

	"github.com/tsuru/tsuru/storage"
	apptypes "github.com/tsuru/tsuru/types/app"
)

var _ apptypes.AppEnvVarService = &appEnvVarService{}

func AppEnvVarService() (apptypes.AppEnvVarService, error) {
	dbDriver, err := storage.GetCurrentDbDriver()
	if err != nil {
		dbDriver, err = storage.GetDefaultDbDriver()
		if err != nil {
			return nil, err
		}
	}

	return &appEnvVarService{storage: dbDriver.AppEnvVarStorage}, nil
}

type appEnvVarService struct {
	storage apptypes.AppEnvVarStorage
}

func (s *appEnvVarService) List(ctx context.Context, appName string) ([]apptypes.EnvVar, error) {
	envs, err := s.storage.ListAppEnvs(ctx, appName)
	if err != nil {
		return nil, err
	}

	svcEnvs, err := s.storage.ListServiceEnvs(ctx, appName)
	if err != nil {
		return nil, err
	}

	svcEnvVars := fromServiceEnvsToAppEnvVars(svcEnvs)

	finalEnvs := make([]apptypes.EnvVar, 0, len(envs)+len(svcEnvVars))
	finalEnvs = append(finalEnvs, envs...)
	finalEnvs = append(finalEnvs, svcEnvVars...)

	return finalEnvs, nil
}

func fromServiceEnvsToAppEnvVars(vars []apptypes.ServiceEnvVar) []apptypes.EnvVar {
	envs := make([]apptypes.EnvVar, 0, len(vars)+1)
	for _, ev := range vars {
		envs = append(envs, ev.EnvVar)
	}
	envs = append(envs, buildTsuruServiceEnvVar(vars))
	return envs
}

func buildTsuruServiceEnvVar(vars []apptypes.ServiceEnvVar) apptypes.EnvVar {
	type serviceInstanceEnvs struct {
		InstanceName string            `json:"instance_name"`
		Envs         map[string]string `json:"envs"`
	}

	result := map[string][]serviceInstanceEnvs{} // env vars from instance by service name

	for _, v := range vars {
		found := false

		for i, instanceList := range result[v.ServiceName] {
			if instanceList.InstanceName == v.InstanceName {
				result[v.ServiceName][i].Envs[v.Name] = v.Value
				found = true
				break
			}
		}

		if !found {
			result[v.ServiceName] = append(result[v.ServiceName], serviceInstanceEnvs{
				InstanceName: v.InstanceName,
				Envs:         map[string]string{v.Name: v.Value},
			})
		}
	}

	jsonVal, _ := json.Marshal(result)

	return apptypes.EnvVar{
		Name:   apptypes.TsuruServicesEnvVarName,
		Value:  string(jsonVal),
		Public: false,
	}
}
