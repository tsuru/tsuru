// Copyright 2022 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/servicemanager"
	"github.com/tsuru/tsuru/storage"
	apptypes "github.com/tsuru/tsuru/types/app"
)

var _ apptypes.AppEnvVarService = (*appEnvVarService)(nil)

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

func (s *appEnvVarService) List(ctx context.Context, a apptypes.App) ([]apptypes.EnvVar, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	envs, err := s.storage.FindAll(ctx, a)
	if err != nil {
		return nil, err
	}

	svcEnvs, err := servicemanager.AppServiceEnvVar.List(ctx, a)
	if err != nil {
		return nil, err
	}

	svcEnvVars := fromServiceEnvsToAppEnvVars(svcEnvs)

	finalEnvs := make([]apptypes.EnvVar, 0, len(envs)+len(svcEnvVars))
	finalEnvs = append(finalEnvs, envs...)
	finalEnvs = append(finalEnvs, svcEnvVars...)

	return finalEnvs, nil
}

func (s *appEnvVarService) Get(ctx context.Context, a apptypes.App, envName string) (apptypes.EnvVar, error) {
	if err := ctx.Err(); err != nil {
		return apptypes.EnvVar{}, err
	}

	envs, err := s.storage.FindAll(ctx, a)
	if err != nil {
		return apptypes.EnvVar{}, err
	}

	for _, env := range envs {
		if env.Name == envName {
			return env, nil
		}
	}

	return apptypes.EnvVar{}, fmt.Errorf("env var not found")
}

func (s *appEnvVarService) Set(ctx context.Context, a apptypes.App, envs []apptypes.EnvVar, args apptypes.SetEnvArgs) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if err := validateEnvs(envs, args); err != nil {
		return err
	}

	if args.Writer == nil {
		args.Writer = io.Discard
	}

	fmt.Fprintf(args.Writer, "---- Setting %d new environment variables ----\n", len(envs))

	// TODO(nettoclaudio): we should review the prune flag.

	final := make([]apptypes.EnvVar, 0, len(envs))
	for _, env := range envs {
		final = append(final, apptypes.EnvVar{
			Name:      env.Name,
			Value:     env.Value,
			Public:    env.Public,
			ManagedBy: args.ManagedBy,
		})

		servicemanager.AppLog.Add(a.GetName(), fmt.Sprintf("setting env %s with value %s", env.Name, env.Value), "tsuru", "api")
	}

	if err := s.storage.Upsert(ctx, a, final); err != nil {
		return err
	}

	if !args.ShouldRestart {
		return nil
	}

	if aa, ok := a.(*App); ok {
		return aa.restartIfUnits(args.Writer)
	}

	return nil
}

func (s *appEnvVarService) Unset(ctx context.Context, a apptypes.App, envs []string, args apptypes.UnsetEnvArgs) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if args.Writer == nil {
		args.Writer = io.Discard
	}

	if len(envs) == 0 {
		return nil
	}

	fmt.Fprintf(args.Writer, "---- Unsetting %d environment variables ----\n", len(envs))

	if err := s.storage.Remove(ctx, a, envs); err != nil {
		return err
	}

	if !args.ShouldRestart {
		return nil
	}

	if aa, ok := a.(*App); ok {
		return aa.restartIfUnits(args.Writer)
	}

	return nil
}

func validateEnvs(envs []apptypes.EnvVar, args apptypes.SetEnvArgs) error {
	for _, env := range envs {
		if !envVarNameRegexp.MatchString(env.Name) {
			return &tsuruErrors.ValidationError{Message: fmt.Sprintf("Invalid environment variable name: '%s'", env.Name)}
		}

		// TODO(nettoclaudio): we shoud also limit the max size of the env var
		// value, something around 32KiB.

		// TODO(nettoclaudio): only Tsuru API should be able to set reserved
		// variables such as: TSURU_APPNAME, TSURU_APP_TOKEN, etc.
	}

	return nil
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
