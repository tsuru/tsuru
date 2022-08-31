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

/*
func (s *S) TestSetEnvCanPruneAllVariables(c *check.C) {
	a := app.App{
		Name:      "black-dog",
		Platform:  "zend",
		TeamOwner: s.team.Name,
		Env: map[string]bind.EnvVar{
			"CMDLINE": {Name: "CMDLINE", Value: "1", Public: true, ManagedBy: "tsuru-client"},
			"OLDENV":  {Name: "OLDENV", Value: "1", Public: true, ManagedBy: "terraform"},
		},
	}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)

	d := apiTypes.Envs{
		Envs:        []apiTypes.Env{},
		PruneUnused: true,
		ManagedBy:   "terraform",
	}

	v, err := json.Marshal(d)
	c.Assert(err, check.IsNil)
	b := bytes.NewReader(v)

	url := fmt.Sprintf("/apps/%s/env", a.Name)
	request, err := http.NewRequest("POST", url, b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())

	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")

	app, err := app.GetByName(context.TODO(), "black-dog")
	c.Assert(err, check.IsNil)

	_, hasOldVar := app.Env["OLDENV"]
	c.Assert(hasOldVar, check.Equals, false)

	expected0 := bind.EnvVar{Name: "CMDLINE", Value: "1", Public: true, ManagedBy: "tsuru-client"}
	c.Assert(app.Env["CMDLINE"], check.DeepEquals, expected0)

	c.Assert(recorder.Body.String(), check.Matches,
		`.*---- Pruning OLDENV from environment variables ----.*
`)
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.update.env.set",
		StartCustomData: []map[string]interface{}{
			{"name": ":app", "value": a.Name},
			{"name": "NoRestart", "value": ""},
			{"name": "Private", "value": ""},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestSetEnvDontPruneWhenMissingManagedBy(c *check.C) {
	a := app.App{
		Name:      "black-dog",
		Platform:  "zend",
		TeamOwner: s.team.Name,
		Env: map[string]bind.EnvVar{
			"CMDLINE": {Name: "CMDLINE", Value: "1", Public: true, ManagedBy: "tsuru-client"},
			"OLDENV":  {Name: "OLDENV", Value: "1", Public: true, ManagedBy: "terraform"},
		},
	}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)

	d := apiTypes.Envs{
		Envs: []apiTypes.Env{
			{Name: "DATABASE_HOST", Value: "localhost", Private: func(b bool) *bool { return &b }(true)},
			{Name: "MY_DB_HOST", Value: "otherhost", Private: func(b bool) *bool { return &b }(false)},
		},
		PruneUnused: true,
	}

	v, err := json.Marshal(d)
	c.Assert(err, check.IsNil)
	b := bytes.NewReader(v)

	url := fmt.Sprintf("/apps/%s/env", a.Name)
	request, err := http.NewRequest("POST", url, b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())

	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "text/plain; charset=utf-8")
	c.Assert(recorder.Body.String(), check.Matches,
		"Prune unused requires a managed-by value\n")
}

func (s *S) TestSetEnvPublicEnvironmentVariableAlias(c *check.C) {
	a := app.App{Name: "black-dog", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/env", a.Name)
	d := apiTypes.Envs{
		Envs: []apiTypes.Env{
			{Name: "DATABASE_HOST", Value: "", Alias: "MY_DB_HOST"},
			{Name: "MY_DB_HOST", Value: "localhost", Alias: ""},
		},
		NoRestart: false,
		Private:   false,
	}
	v, err := form.EncodeToValues(&d)
	c.Assert(err, check.IsNil)
	b := strings.NewReader(v.Encode())
	request, err := http.NewRequest("POST", url, b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	app, err := app.GetByName(context.TODO(), "black-dog")
	c.Assert(err, check.IsNil)
	c.Assert(app.Env["DATABASE_HOST"], check.DeepEquals, bind.EnvVar{
		Name:   "DATABASE_HOST",
		Alias:  "MY_DB_HOST",
		Public: true,
	})
	c.Assert(app.Env["MY_DB_HOST"], check.DeepEquals, bind.EnvVar{
		Name:   "MY_DB_HOST",
		Value:  "localhost",
		Public: true,
	})
	c.Assert(recorder.Body.String(), check.Matches,
		`{"Message":".*---- Setting 2 new environment variables ----\\n","Timestamp":".*"}
`)
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.update.env.set",
		StartCustomData: []map[string]interface{}{
			{"name": ":app", "value": a.Name},
			{"name": "Envs.0.Name", "value": "DATABASE_HOST"},
			{"name": "Envs.0.Alias", "value": "MY_DB_HOST"},
			{"name": "Envs.1.Name", "value": "MY_DB_HOST"},
			{"name": "Envs.1.Value", "value": "localhost"},
			{"name": "NoRestart", "value": ""},
			{"name": "Private", "value": ""},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestSetEnvHandlerShouldSetAPrivateEnvironmentVariableInTheApp(c *check.C) {
	a := app.App{Name: "black-dog", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/env", a.Name)
	d := apiTypes.Envs{
		Envs: []apiTypes.Env{
			{Name: "DATABASE_HOST", Value: "localhost", Alias: ""},
		},
		NoRestart: false,
		Private:   true,
	}
	v, err := form.EncodeToValues(&d)
	c.Assert(err, check.IsNil)
	b := strings.NewReader(v.Encode())
	request, err := http.NewRequest("POST", url, b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	app, err := app.GetByName(context.TODO(), "black-dog")
	c.Assert(err, check.IsNil)
	expected := bind.EnvVar{Name: "DATABASE_HOST", Value: "localhost", Public: false}
	c.Assert(app.Env["DATABASE_HOST"], check.DeepEquals, expected)
	c.Assert(recorder.Body.String(), check.Matches,
		`{"Message":".*---- Setting 1 new environment variables ----\\n","Timestamp":".*"}
`)
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.update.env.set",
		StartCustomData: []map[string]interface{}{
			{"name": ":app", "value": a.Name},
			{"name": "Envs.0.Name", "value": "DATABASE_HOST"},
			{"name": "Envs.0.Value", "value": "*****"},
			{"name": "NoRestart", "value": ""},
			{"name": "Private", "value": "true"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestSetEnvHandlerShouldSetADoublePrivateEnvironmentVariableInTheApp(c *check.C) {
	a := app.App{Name: "black-dog", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/env", a.Name)
	d := apiTypes.Envs{
		Envs: []apiTypes.Env{
			{Name: "DATABASE_HOST", Value: "localhost", Alias: ""},
		},
		NoRestart: false,
		Private:   true,
	}
	v, err := form.EncodeToValues(&d)
	c.Assert(err, check.IsNil)
	b := strings.NewReader(v.Encode())
	request, err := http.NewRequest("POST", url, b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.update.env.set",
		StartCustomData: []map[string]interface{}{
			{"name": ":app", "value": a.Name},
			{"name": "Envs.0.Name", "value": "DATABASE_HOST"},
			{"name": "Envs.0.Value", "value": "*****"},
			{"name": "NoRestart", "value": ""},
			{"name": "Private", "value": "true"},
		},
	}, eventtest.HasEvent)
	d = apiTypes.Envs{
		Envs: []apiTypes.Env{
			{Name: "DATABASE_HOST", Value: "127.0.0.1", Alias: ""},
			{Name: "DATABASE_PORT", Value: "6379", Alias: ""},
		},
		NoRestart: false,
		Private:   true,
	}
	v, err = form.EncodeToValues(&d)
	c.Assert(err, check.IsNil)
	b = strings.NewReader(v.Encode())
	request, err = http.NewRequest("POST", url, b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder = httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	app, err := app.GetByName(context.TODO(), "black-dog")
	c.Assert(err, check.IsNil)
	expected := bind.EnvVar{Name: "DATABASE_HOST", Value: "127.0.0.1", Public: false}
	c.Assert(app.Env["DATABASE_HOST"], check.DeepEquals, expected)
	c.Assert(recorder.Body.String(), check.Matches,
		`{"Message":".*---- Setting 2 new environment variables ----\\n","Timestamp":".*"}
`)
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.update.env.set",
		StartCustomData: []map[string]interface{}{
			{"name": ":app", "value": a.Name},
			{"name": "Envs.0.Name", "value": "DATABASE_HOST"},
			{"name": "Envs.0.Value", "value": "*****"},
			{"name": "Envs.1.Name", "value": "DATABASE_PORT"},
			{"name": "Envs.1.Value", "value": "*****"},
			{"name": "NoRestart", "value": ""},
			{"name": "Private", "value": "true"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestSetEnvHandlerShouldSetMultipleEnvironmentVariablesInTheApp(c *check.C) {
	a := app.App{Name: "vigil", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/env", a.Name)
	d := apiTypes.Envs{
		Envs: []apiTypes.Env{
			{Name: "DATABASE_HOST", Value: "localhost", Alias: ""},
			{Name: "DATABASE_USER", Value: "root", Alias: ""},
		},
		NoRestart: false,
		Private:   false,
	}
	v, err := form.EncodeToValues(&d)
	c.Assert(err, check.IsNil)
	b := strings.NewReader(v.Encode())
	request, err := http.NewRequest("POST", url, b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	app, err := app.GetByName(context.TODO(), "vigil")
	c.Assert(err, check.IsNil)
	expectedHost := bind.EnvVar{Name: "DATABASE_HOST", Value: "localhost", Public: true}
	expectedUser := bind.EnvVar{Name: "DATABASE_USER", Value: "root", Public: true}
	c.Assert(app.Env["DATABASE_HOST"], check.DeepEquals, expectedHost)
	c.Assert(app.Env["DATABASE_USER"], check.DeepEquals, expectedUser)
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.update.env.set",
		StartCustomData: []map[string]interface{}{
			{"name": ":app", "value": a.Name},
			{"name": "Envs.0.Name", "value": "DATABASE_HOST"},
			{"name": "Envs.0.Value", "value": "localhost"},
			{"name": "Envs.1.Name", "value": "DATABASE_USER"},
			{"name": "Envs.1.Value", "value": "root"},
			{"name": "NoRestart", "value": ""},
			{"name": "Private", "value": ""},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestSetEnvHandlerShouldNotChangeValueOfServiceVariables(c *check.C) {
	a := &app.App{Name: "losers", Platform: "zend", Teams: []string{s.team.Name}, ServiceEnvs: []bind.ServiceEnvVar{
		{
			EnvVar: bind.EnvVar{
				Name:  "DATABASE_HOST",
				Value: "privatehost.com",
			},
			ServiceName:  "srv1",
			InstanceName: "some service",
		},
	}}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/env", a.Name)
	d := apiTypes.Envs{
		Envs: []apiTypes.Env{
			{Name: "DATABASE_HOST", Value: "http://foo.com:8080", Alias: ""},
		},
		NoRestart: false,
		Private:   false,
	}
	v, err := form.EncodeToValues(&d)
	c.Assert(err, check.IsNil)
	b := strings.NewReader(v.Encode())
	request, err := http.NewRequest("POST", url, b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	a, err = app.GetByName(context.TODO(), "losers")
	c.Assert(err, check.IsNil)
	envs := a.Envs()
	delete(envs, app.TsuruServicesEnvVar)
	expected := map[string]bind.EnvVar{
		"DATABASE_HOST": {
			Name:  "DATABASE_HOST",
			Value: "privatehost.com",
		},
	}
	c.Assert(envs, check.DeepEquals, expected)
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.update.env.set",
		StartCustomData: []map[string]interface{}{
			{"name": ":app", "value": a.Name},
			{"name": "Envs.0.Name", "value": "DATABASE_HOST"},
			{"name": "Envs.0.Value", "value": "http://foo.com:8080"},
			{"name": "NoRestart", "value": ""},
			{"name": "Private", "value": ""},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestSetEnvHandlerNoRestart(c *check.C) {
	a := app.App{Name: "black-dog", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	d := apiTypes.Envs{
		Envs: []apiTypes.Env{
			{Name: "DATABASE_HOST", Value: "localhost", Alias: ""},
		},
		NoRestart: true,
		Private:   false,
	}
	v, err := form.EncodeToValues(&d)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/env", a.Name)
	b := strings.NewReader(v.Encode())
	request, err := http.NewRequest("POST", url, b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	app, err := app.GetByName(context.TODO(), "black-dog")
	c.Assert(err, check.IsNil)
	expected := bind.EnvVar{Name: "DATABASE_HOST", Value: "localhost", Public: true}
	c.Assert(app.Env["DATABASE_HOST"], check.DeepEquals, expected)
	c.Assert(recorder.Body.String(), check.Matches,
		`{"Message":".*---- Setting 1 new environment variables ----\\n","Timestamp":".*"}
`)
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.update.env.set",
		StartCustomData: []map[string]interface{}{
			{"name": ":app", "value": a.Name},
			{"name": "Envs.0.Name", "value": "DATABASE_HOST"},
			{"name": "Envs.0.Value", "value": "localhost"},
			{"name": "NoRestart", "value": "true"},
			{"name": "Private", "value": ""},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestSetEnvMissingFormBody(c *check.C) {
	a := app.App{Name: "rock", Platform: "zend"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("POST", "/apps/rock/env", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	msg := ".*missing form body\n"
	c.Assert(recorder.Body.String(), check.Matches, msg)
}

func (s *S) TestSetEnvHandlerReturnsBadRequestIfVariablesAreMissing(c *check.C) {
	a := app.App{Name: "rock", Platform: "zend"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("POST", "/apps/rock/env", strings.NewReader(""))
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	msg := "You must provide the list of environment variables\n"
	c.Assert(recorder.Body.String(), check.Equals, msg)
}

func (s *S) TestSetEnvHandlerReturnsNotFoundIfTheAppDoesNotExist(c *check.C) {
	b := strings.NewReader("noRestart=false&private=&false&envs.0.name=DATABASE_HOST&envs.0.value=localhost")
	request, err := http.NewRequest("POST", "/apps/unknown/env", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, "App unknown not found.\n")
}

func (s *S) TestSetEnvHandlerReturnsForbiddenIfTheGivenUserDoesNotHaveAccessToTheApp(c *check.C) {
	a := app.App{Name: "rock-and-roll", Platform: "zend"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppUpdateEnvSet,
		Context: permission.Context(permTypes.CtxApp, "-invalid-"),
	})
	d := apiTypes.Envs{
		Envs: []apiTypes.Env{
			{Name: "DATABASE_HOST", Value: "localhost", Alias: ""},
		},
		NoRestart: false,
		Private:   false,
	}
	v, err := form.EncodeToValues(&d)
	c.Assert(err, check.IsNil)
	b := strings.NewReader(v.Encode())
	url := fmt.Sprintf("/apps/%s/env", a.Name)
	request, err := http.NewRequest("POST", url, b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}

func (s *S) TestSetEnvInvalidEnvName(c *check.C) {
	a := app.App{Name: "black-dog", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/env", a.Name)
	d := apiTypes.Envs{
		Envs: []apiTypes.Env{
			{Name: "INVALID ENV", Value: "value", Alias: ""},
		},
	}
	v, err := form.EncodeToValues(&d)
	c.Assert(err, check.IsNil)
	b := strings.NewReader(v.Encode())
	request, err := http.NewRequest(http.MethodPost, url, b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
}


func (s *S) TestUnsetEnv(c *check.C) {
	a := app.App{
		Name:     "swift",
		Platform: "zend",
		Teams:    []string{s.team.Name},
		Env: map[string]bind.EnvVar{
			"DATABASE_HOST":     {Name: "DATABASE_HOST", Value: "localhost", Public: true},
			"DATABASE_USER":     {Name: "DATABASE_USER", Value: "root", Public: true},
			"DATABASE_PASSWORD": {Name: "DATABASE_PASSWORD", Value: "secret", Public: false},
		},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	expected := a.Env
	delete(expected, "DATABASE_HOST")
	url := fmt.Sprintf("/apps/%s/env?noRestart=false&env=DATABASE_HOST", a.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	app, err := app.GetByName(context.TODO(), "swift")
	c.Assert(err, check.IsNil)
	c.Assert(app.Env, check.DeepEquals, expected)
	c.Assert(recorder.Body.String(), check.Matches,
		`{"Message":".*---- Unsetting 1 environment variables ----\\n","Timestamp":".*"}
`)
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.update.env.unset",
		StartCustomData: []map[string]interface{}{
			{"name": ":app", "value": a.Name},
			{"name": "env", "value": "DATABASE_HOST"},
			{"name": "noRestart", "value": "false"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestUnsetEnvNoRestart(c *check.C) {
	a := app.App{
		Name:     "swift",
		Platform: "zend",
		Teams:    []string{s.team.Name},
		Env: map[string]bind.EnvVar{
			"DATABASE_HOST":     {Name: "DATABASE_HOST", Value: "localhost", Public: true},
			"DATABASE_USER":     {Name: "DATABASE_USER", Value: "root", Public: true},
			"DATABASE_PASSWORD": {Name: "DATABASE_PASSWORD", Value: "secret", Public: false},
		},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	expected := a.Env
	delete(expected, "DATABASE_HOST")
	url := fmt.Sprintf("/apps/%s/env?noRestart=true&env=DATABASE_HOST", a.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	app, err := app.GetByName(context.TODO(), "swift")
	c.Assert(err, check.IsNil)
	c.Assert(app.Env, check.DeepEquals, expected)
	c.Assert(recorder.Body.String(), check.Matches,
		`{"Message":".*---- Unsetting 1 environment variables ----\\n","Timestamp":".*"}
`)
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.update.env.unset",
		StartCustomData: []map[string]interface{}{
			{"name": ":app", "value": a.Name},
			{"name": "env", "value": "DATABASE_HOST"},
			{"name": "noRestart", "value": "true"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestUnsetEnvHandlerRemovesAllGivenEnvironmentVariables(c *check.C) {
	a := app.App{
		Name:     "let-it-be",
		Platform: "zend",
		Teams:    []string{s.team.Name},
		Env: map[string]bind.EnvVar{
			"DATABASE_HOST":     {Name: "DATABASE_HOST", Value: "localhost", Public: true},
			"DATABASE_USER":     {Name: "DATABASE_USER", Value: "root", Public: true},
			"DATABASE_PASSWORD": {Name: "DATABASE_PASSWORD", Value: "secret", Public: false},
		},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/env?noRestart=false&env=DATABASE_HOST&env=DATABASE_USER", a.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	app, err := app.GetByName(context.TODO(), "let-it-be")
	c.Assert(err, check.IsNil)
	expected := map[string]bind.EnvVar{
		"DATABASE_PASSWORD": {
			Name:   "DATABASE_PASSWORD",
			Value:  "secret",
			Public: false,
		},
	}
	c.Assert(app.Env, check.DeepEquals, expected)
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.update.env.unset",
		StartCustomData: []map[string]interface{}{
			{"name": ":app", "value": a.Name},
			{"name": "env", "value": []string{"DATABASE_HOST", "DATABASE_USER"}},
			{"name": "noRestart", "value": "false"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestUnsetHandlerRemovesPrivateVariables(c *check.C) {
	a := app.App{
		Name:     "letitbe",
		Platform: "zend",
		Teams:    []string{s.team.Name},
		Env: map[string]bind.EnvVar{
			"DATABASE_HOST":     {Name: "DATABASE_HOST", Value: "localhost", Public: true},
			"DATABASE_USER":     {Name: "DATABASE_USER", Value: "root", Public: true},
			"DATABASE_PASSWORD": {Name: "DATABASE_PASSWORD", Value: "secret", Public: false},
		},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/env?noRestart=false&env=DATABASE_HOST&env=DATABASE_USER&env=DATABASE_PASSWORD", a.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	app, err := app.GetByName(context.TODO(), "letitbe")
	c.Assert(err, check.IsNil)
	expected := map[string]bind.EnvVar{}
	c.Assert(app.Env, check.DeepEquals, expected)
}

func (s *S) TestUnsetEnvVariablesMissing(c *check.C) {
	a := app.App{
		Name:     "swift",
		Platform: "zend",
		Teams:    []string{s.team.Name},
		Env: map[string]bind.EnvVar{
			"DATABASE_HOST":     {Name: "DATABASE_HOST", Value: "localhost", Public: true},
			"DATABASE_USER":     {Name: "DATABASE_USER", Value: "root", Public: true},
			"DATABASE_PASSWORD": {Name: "DATABASE_PASSWORD", Value: "secret", Public: false},
		},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("DELETE", "/apps/swift/env?noRestart=false&env=", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Equals, "You must provide the list of environment variables.\n")
}

func (s *S) TestUnsetEnvAppDoesNotExist(c *check.C) {
	request, err := http.NewRequest("DELETE", "/apps/unknown/env?noRestart=false&env=ble", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, "App unknown not found.\n")
}

func (s *S) TestUnsetEnvUserDoesNotHaveAccessToTheApp(c *check.C) {
	a := app.App{Name: "mountain-mama"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppUpdateEnvUnset,
		Context: permission.Context(permTypes.CtxApp, "-invalid-"),
	})
	url := fmt.Sprintf("/apps/%s/env?noRestart=false&env=DATABASE_HOST", a.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+token.GetValue())
	recorder := httptest.NewRecorder()
	err = s.provisioner.Provision(context.TODO(), &a)
	c.Assert(err, check.IsNil)
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}
*/
