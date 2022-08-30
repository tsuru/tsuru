// Copyright 2022 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"context"
	"fmt"
	"io"

	"github.com/tsuru/tsuru/storage"
	apptypes "github.com/tsuru/tsuru/types/app"
)

var _ apptypes.AppServiceEnvVarService = (*serviceEnvVarService)(nil)

func AppServiceEnvVarService() (apptypes.AppServiceEnvVarService, error) {
	dbDriver, err := storage.GetCurrentDbDriver()
	if err != nil {
		dbDriver, err = storage.GetDefaultDbDriver()
		if err != nil {
			return nil, err
		}
	}
	return &serviceEnvVarService{storage: dbDriver.AppServiceEnvVarStorage}, nil
}

type serviceEnvVarService struct {
	storage apptypes.AppServiceEnvVarStorage
}

func (s *serviceEnvVarService) List(ctx context.Context, a apptypes.App) ([]apptypes.ServiceEnvVar, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	return s.storage.FindAll(ctx, a)
}

func (s *serviceEnvVarService) Set(ctx context.Context, a apptypes.App, envs []apptypes.ServiceEnvVar, args apptypes.SetEnvArgs) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if args.Writer == nil {
		args.Writer = io.Discard
	}

	fmt.Fprintf(args.Writer, "---- Setting %d new environment variables ----\n", len(envs))

	if err := s.storage.Upsert(ctx, a, envs); err != nil {
		return err
	}

	if !args.ShouldRestart {
		return nil
	}

	return a.(*App).restartIfUnits(args.Writer)
}

func (s *serviceEnvVarService) Unset(ctx context.Context, a apptypes.App, envNames []apptypes.ServiceEnvVarIdentifier, args apptypes.UnsetEnvArgs) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if args.Writer == nil {
		args.Writer = io.Discard
	}

	fmt.Fprintf(args.Writer, "---- Unsetting %d environment variables ----\n", len(envNames))

	if err := s.storage.Remove(ctx, a, envNames); err != nil {
		return err
	}

	if !args.ShouldRestart {
		return nil
	}

	return a.(*App).restartIfUnits(args.Writer)
}

func (s *serviceEnvVarService) UnsetAll(ctx context.Context, a apptypes.App, args apptypes.UnsetAllArgs) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	svcEnvs, err := s.List(ctx, a)
	if err != nil {
		return err
	}

	var envs []apptypes.ServiceEnvVarIdentifier
	for _, env := range svcEnvs {
		if args.Service != "" && args.Service != env.ServiceName {
			continue
		}

		if args.Instance != "" && args.Instance != env.InstanceName {
			continue
		}

		envs = append(envs, &apptypes.ServiceEnvVar{
			ServiceName:  env.ServiceName,
			InstanceName: env.InstanceName,
			EnvVar:       apptypes.EnvVar{Name: env.Name},
		})
	}

	return s.Unset(ctx, a, envs, apptypes.UnsetEnvArgs{ShouldRestart: args.ShouldRestart, Writer: args.Writer})
}
