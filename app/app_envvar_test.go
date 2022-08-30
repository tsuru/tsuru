// Copyright 2022 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"bytes"
	"context"
	"errors"

	apptypes "github.com/tsuru/tsuru/types/app"
	check "gopkg.in/check.v1"
)

func (s *S) TestAppEnvVarService_List_ContextCanceled(c *check.C) {
	ctx, cancel := context.WithCancel(context.TODO())
	cancel()
	envVarService := &appEnvVarService{}
	_, err := envVarService.List(ctx, nil)
	c.Assert(err, check.NotNil)
}

func (s *S) TestAppEnvVarService_List(c *check.C) {
	envVarService := &appEnvVarService{
		storage: &apptypes.MockAppEnvVarStorage{
			OnFindAll: func(a apptypes.App) ([]apptypes.EnvVar, error) {
				c.Assert(a.GetName(), check.Equals, "my-app")
				return []apptypes.EnvVar{
					{Name: "MY_ENV_01", Value: "env 01"},
					{Name: "MY_ENV_02", Value: "env 02", Public: true},
					{Name: "MY_ENV_03", Value: "env 03", ManagedBy: "terraform"},
				}, nil
			},
		},
	}
	envs, err := envVarService.List(context.TODO(), &App{Name: "my-app"})
	c.Assert(err, check.IsNil)
	c.Assert(envs, check.DeepEquals, []apptypes.EnvVar{
		{Name: "MY_ENV_01", Value: "env 01"},
		{Name: "MY_ENV_02", Value: "env 02", Public: true},
		{Name: "MY_ENV_03", Value: "env 03", ManagedBy: "terraform"},
	})
}

func (s *S) TestAppEnvVarService_Set_ContextCanceled(c *check.C) {
	ctx, cancel := context.WithCancel(context.TODO())
	cancel()
	envVarService := &appEnvVarService{}
	err := envVarService.Set(ctx, nil, nil, apptypes.SetEnvArgs{})
	c.Assert(err, check.NotNil)
}

func (s *S) TestAppEnvVarService_Set_NoRestart(c *check.C) {
	envs := []apptypes.EnvVar{
		{Name: "ENV_00", Value: "env 00"},
		{Name: "ENV_01", Value: "env 01", Public: true},
		{Name: "ENV_02", Value: "env 02", Public: true},
	}
	var w bytes.Buffer
	envVarService := &appEnvVarService{
		storage: &apptypes.MockAppEnvVarStorage{
			OnUpsert: func(a apptypes.App, e []apptypes.EnvVar) error {
				c.Assert(a.GetName(), check.Equals, "my-app")
				c.Assert(e, check.DeepEquals, envs)
				return nil
			},
		},
	}
	err := envVarService.Set(context.TODO(), &App{Name: "my-app"}, envs, apptypes.SetEnvArgs{Writer: &w})
	c.Assert(err, check.IsNil)
	c.Assert(w.String(), check.Matches, `---- Setting 3 new environment variables ----\s+`)
}

func (s *S) TestAppEnvVarService_Set_NoRestart_CustomManagedBy(c *check.C) {
	envs := []apptypes.EnvVar{
		{Name: "ENV_00", Value: "env 00"},
		{Name: "ENV_01", Value: "env 01", Public: true},
		{Name: "ENV_02", Value: "env 02", Public: true},
	}
	var w bytes.Buffer
	envVarService := &appEnvVarService{
		storage: &apptypes.MockAppEnvVarStorage{
			OnUpsert: func(a apptypes.App, e []apptypes.EnvVar) error {
				c.Assert(a.GetName(), check.Equals, "my-app")
				c.Assert(e, check.DeepEquals, []apptypes.EnvVar{
					{Name: "ENV_00", Value: "env 00", ManagedBy: "terraform"},
					{Name: "ENV_01", Value: "env 01", Public: true, ManagedBy: "terraform"},
					{Name: "ENV_02", Value: "env 02", Public: true, ManagedBy: "terraform"},
				})
				return nil
			},
		},
	}
	err := envVarService.Set(context.TODO(), &App{Name: "my-app"}, envs, apptypes.SetEnvArgs{ManagedBy: "terraform", Writer: &w})
	c.Assert(err, check.IsNil)
	c.Assert(w.String(), check.Matches, `---- Setting 3 new environment variables ----\s+`)
}

func (s *S) TestAppEnvVarService_Set_WithRestart_WithUnits(c *check.C) {
	a := App{
		Name:      "my-app",
		TeamOwner: s.team.Name,
	}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	newSuccessfulAppVersion(c, &a)
	err = a.AddUnits(1, "web", "", nil)
	c.Assert(err, check.IsNil)
	envs := []apptypes.EnvVar{
		{Name: "ENV_00", Value: "env 00"},
		{Name: "ENV_01", Value: "env 01", Public: true},
		{Name: "ENV_02", Value: "env 02", Public: true},
	}
	var w bytes.Buffer
	envVarService := &appEnvVarService{
		storage: &apptypes.MockAppEnvVarStorage{
			OnUpsert: func(a apptypes.App, e []apptypes.EnvVar) error {
				c.Assert(a.GetName(), check.Equals, "my-app")
				c.Assert(e, check.DeepEquals, envs)
				return nil
			},
		},
	}
	err = envVarService.Set(context.TODO(), &a, envs, apptypes.SetEnvArgs{Writer: &w, ShouldRestart: true})
	c.Assert(err, check.IsNil)
	c.Assert(s.provisioner.Restarts(&a, ""), check.Equals, 1)
	c.Assert(w.String(), check.Matches, "---- Setting 3 new environment variables ----\nrestarting app")
}

func (s *S) TestAppEnvVarService_Set_WithRestart_WithoutUnits(c *check.C) {
	a := App{
		Name:      "my-app",
		TeamOwner: s.team.Name,
	}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	newSuccessfulAppVersion(c, &a)
	c.Assert(err, check.IsNil)
	envs := []apptypes.EnvVar{
		{Name: "ENV_00", Value: "env 00"},
		{Name: "ENV_01", Value: "env 01", Public: true},
		{Name: "ENV_02", Value: "env 02", Public: true},
	}
	var w bytes.Buffer
	envVarService := &appEnvVarService{
		storage: &apptypes.MockAppEnvVarStorage{
			OnUpsert: func(a apptypes.App, e []apptypes.EnvVar) error {
				c.Assert(a.GetName(), check.Equals, "my-app")
				c.Assert(e, check.DeepEquals, envs)
				return nil
			},
		},
	}
	err = envVarService.Set(context.TODO(), &a, envs, apptypes.SetEnvArgs{Writer: &w, ShouldRestart: true})
	c.Assert(err, check.IsNil)
	c.Assert(s.provisioner.Restarts(&a, ""), check.Equals, 0)
	c.Assert(w.String(), check.Matches, "---- Setting 3 new environment variables ----\n")
}

func (s *S) TestAppEnvVarService_Set_InvalidEnvs(c *check.C) {
	tests := []struct {
		env           apptypes.EnvVar
		expectedError string
	}{
		{
			env:           apptypes.EnvVar{Name: "-NO_LEADING_DASH"},
			expectedError: `Invalid environment variable name: '-NO_LEADING_DASH'`,
		},
		{
			env:           apptypes.EnvVar{Name: "_NO_LEADING_UNDERSCORE"},
			expectedError: `Invalid environment variable name: '_NO_LEADING_UNDERSCORE'`,
		},
		{
			env:           apptypes.EnvVar{Name: "ENV.WITH.DOTS"},
			expectedError: `Invalid environment variable name: 'ENV.WITH.DOTS'`,
		},
		{
			env:           apptypes.EnvVar{Name: "ENV VAR WITH SPACES"},
			expectedError: `Invalid environment variable name: 'ENV VAR WITH SPACES'`,
		},
		{
			env:           apptypes.EnvVar{Name: "ENV.WITH.DOTS"},
			expectedError: `Invalid environment variable name: 'ENV.WITH.DOTS'`,
		},
	}

	for _, tt := range tests {
		envVarService := &appEnvVarService{
			storage: &apptypes.MockAppEnvVarStorage{
				OnUpsert: func(a apptypes.App, e []apptypes.EnvVar) error {
					return errors.New("should not be called")
				},
			},
		}
		err := envVarService.Set(context.TODO(), &App{Name: "my-app"}, []apptypes.EnvVar{tt.env}, apptypes.SetEnvArgs{})
		c.Assert(err, check.NotNil)
		c.Assert(err, check.ErrorMatches, tt.expectedError)
	}
}

func (s *S) TestAppEnvVarService_Unset_ContextCanceled(c *check.C) {
	ctx, cancel := context.WithCancel(context.TODO())
	cancel()
	envVarService := &appEnvVarService{}
	err := envVarService.Unset(ctx, nil, nil, apptypes.UnsetEnvArgs{})
	c.Assert(err, check.NotNil)
}

func (s *S) TestAppEnvVarService_Unset_NoEnvToUnset(c *check.C) {
	var storageRemoveCalled bool
	envVarService := &appEnvVarService{
		storage: &apptypes.MockAppEnvVarStorage{
			OnRemove: func(a apptypes.App, e []string) error {
				storageRemoveCalled = true
				return nil
			},
		},
	}
	err := envVarService.Unset(context.TODO(), &App{Name: "my-app"}, nil, apptypes.UnsetEnvArgs{})
	c.Assert(err, check.IsNil)
	c.Assert(storageRemoveCalled, check.Equals, false)
}

func (s *S) TestAppEnvVarService_Unset_NoRestart(c *check.C) {
	envs := []string{"ENV_00", "ENV_01", "ENV_02"}
	var w bytes.Buffer
	envVarService := &appEnvVarService{
		storage: &apptypes.MockAppEnvVarStorage{
			OnRemove: func(a apptypes.App, e []string) error {
				c.Assert(a.GetName(), check.Equals, "my-app")
				c.Assert(e, check.DeepEquals, envs)
				return nil
			},
		},
	}
	err := envVarService.Unset(context.TODO(), &App{Name: "my-app"}, envs, apptypes.UnsetEnvArgs{Writer: &w})
	c.Assert(err, check.IsNil)
	c.Assert(w.String(), check.Matches, "---- Unsetting 3 environment variables ----\n")
}

func (s *S) TestAppEnvVarService_Unset_WithRestart_WithUnits(c *check.C) {
	a := App{
		Name:      "my-app",
		TeamOwner: s.team.Name,
	}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	newSuccessfulAppVersion(c, &a)
	err = a.AddUnits(1, "web", "", nil)
	c.Assert(err, check.IsNil)
	envs := []string{"ENV_00", "ENV_01", "ENV_02"}
	var w bytes.Buffer
	envVarService := &appEnvVarService{
		storage: &apptypes.MockAppEnvVarStorage{
			OnUpsert: func(a apptypes.App, e []apptypes.EnvVar) error {
				c.Assert(a.GetName(), check.Equals, "my-app")
				c.Assert(e, check.DeepEquals, envs)
				return nil
			},
		},
	}
	err = envVarService.Unset(context.TODO(), &a, envs, apptypes.UnsetEnvArgs{Writer: &w, ShouldRestart: true})
	c.Assert(err, check.IsNil)
	c.Assert(s.provisioner.Restarts(&a, ""), check.Equals, 1)
	c.Assert(w.String(), check.Matches, "---- Unsetting 3 environment variables ----\nrestarting app")
}

func (s *S) TestAppEnvVarService_Unset_WithRestart_NoUnits(c *check.C) {
	a := App{
		Name:      "my-app",
		TeamOwner: s.team.Name,
	}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	newSuccessfulAppVersion(c, &a)
	envs := []string{"ENV_00", "ENV_01", "ENV_02"}
	var w bytes.Buffer
	envVarService := &appEnvVarService{
		storage: &apptypes.MockAppEnvVarStorage{
			OnUpsert: func(a apptypes.App, e []apptypes.EnvVar) error {
				c.Assert(a.GetName(), check.Equals, "my-app")
				c.Assert(e, check.DeepEquals, envs)
				return nil
			},
		},
	}
	err = envVarService.Unset(context.TODO(), &a, envs, apptypes.UnsetEnvArgs{Writer: &w, ShouldRestart: true})
	c.Assert(err, check.IsNil)
	c.Assert(s.provisioner.Restarts(&a, ""), check.Equals, 0)
	c.Assert(w.String(), check.Matches, "---- Unsetting 3 environment variables ----\n")
}

func (s *S) TestAppEnvVarService_Get_ContextCanceled(c *check.C) {
	ctx, cancel := context.WithCancel(context.TODO())
	cancel()
	envVarService := &appEnvVarService{}
	_, err := envVarService.Get(ctx, nil, "")
	c.Assert(err, check.NotNil)
}

func (s *S) TestAppEnvVarService_Get(c *check.C) {
	envVarService := &appEnvVarService{
		storage: &apptypes.MockAppEnvVarStorage{
			OnFindAll: func(a apptypes.App) ([]apptypes.EnvVar, error) {
				return []apptypes.EnvVar{
					{Name: "ENV_00", Value: "env 00"},
					{Name: "ENV_01", Value: "env 01", Public: true},
					{Name: "ENV_02", Value: "env 02", Public: true},
					{Name: "MY_SPECIAL_ENV", Value: "my awesome env", Public: true},
				}, nil
			},
		},
	}
	env, err := envVarService.Get(context.TODO(), &App{Name: "my-app"}, "MY_SPECIAL_ENV")
	c.Assert(err, check.IsNil)
	c.Assert(env, check.Equals, apptypes.EnvVar{Name: "MY_SPECIAL_ENV", Value: "my awesome env", Public: true})

	env, err = envVarService.Get(context.TODO(), &App{Name: "my-app"}, "ENV_01")
	c.Assert(err, check.IsNil)
	c.Assert(env, check.Equals, apptypes.EnvVar{Name: "ENV_01", Value: "env 01", Public: true})
}

func (s *S) TestAppEnvVarService_Get_EnvNotFound(c *check.C) {
	envVarService := &appEnvVarService{
		storage: &apptypes.MockAppEnvVarStorage{
			OnFindAll: func(a apptypes.App) ([]apptypes.EnvVar, error) {
				return []apptypes.EnvVar{
					{Name: "ENV_00", Value: "env 00"},
					{Name: "ENV_01", Value: "env 01", Public: true},
					{Name: "ENV_02", Value: "env 02", Public: true},
					{Name: "MY_SPECIAL_ENV", Value: "my awesome env", Public: true},
				}, nil
			},
		},
	}
	_, err := envVarService.Get(context.TODO(), &App{Name: "my-app"}, "NOT_FOUND_ENV")
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, "env var not found")
}
