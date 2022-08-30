// Copyright 2022 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"bytes"
	"context"

	"github.com/pkg/errors"
	apptypes "github.com/tsuru/tsuru/types/app"
	check "gopkg.in/check.v1"
)

func (s *S) TestAppServiceEnvVar_List(c *check.C) {
	svcEnvVar := &serviceEnvVarService{
		storage: &apptypes.MockServiceEnvVarStorage{
			OnFindAll: func(a apptypes.App) ([]apptypes.ServiceEnvVar, error) {
				c.Assert(a.GetName(), check.Equals, "my-app")
				return []apptypes.ServiceEnvVar{
					{ServiceName: "s3", InstanceName: "my-instance-01", EnvVar: apptypes.EnvVar{Name: "S3_URL_BUCKET", Value: "https://s3.company.example.com/", Public: true}},
					{ServiceName: "s3", InstanceName: "my-instance-02", EnvVar: apptypes.EnvVar{Name: "S3_AUTH_ENDPOINT", Value: "https://s3.company.example.com/", Public: true}},
				}, nil
			},
		},
	}
	envs, err := svcEnvVar.List(context.TODO(), &apptypes.MockApp{Name: "my-app"})
	c.Assert(err, check.IsNil)
	c.Assert(envs, check.DeepEquals, []apptypes.ServiceEnvVar{
		{ServiceName: "s3", InstanceName: "my-instance-01", EnvVar: apptypes.EnvVar{Name: "S3_URL_BUCKET", Value: "https://s3.company.example.com/", Public: true}},
		{ServiceName: "s3", InstanceName: "my-instance-02", EnvVar: apptypes.EnvVar{Name: "S3_AUTH_ENDPOINT", Value: "https://s3.company.example.com/", Public: true}},
	})
}

func (s *S) TestAppServiceEnvVar_List_ContextCanceled(c *check.C) {
	ctx, cancel := context.WithCancel(context.TODO())
	cancel()
	svcEnvVar := &serviceEnvVarService{
		storage: &apptypes.MockServiceEnvVarStorage{
			OnFindAll: func(a apptypes.App) ([]apptypes.ServiceEnvVar, error) {
				return nil, errors.New("should not be called")
			},
		},
	}
	_, err := svcEnvVar.List(ctx, nil)
	c.Assert(err, check.NotNil)
}

func (s *S) TestAppServiceEnvVar_Set(c *check.C) {
	svcEnvVar := &serviceEnvVarService{
		storage: &apptypes.MockServiceEnvVarStorage{
			OnUpsert: func(a apptypes.App, envs []apptypes.ServiceEnvVar) error {
				c.Assert(a.GetName(), check.Equals, "my-app")
				c.Assert(envs, check.DeepEquals, []apptypes.ServiceEnvVar{
					{ServiceName: "service-01", InstanceName: "instance-01", EnvVar: apptypes.EnvVar{Name: "SVC_ENV_01", Value: "svc env 01"}},
					{ServiceName: "service-01", InstanceName: "instance-01", EnvVar: apptypes.EnvVar{Name: "SVC_ENV_02", Value: "svc env 02"}},
					{ServiceName: "service-01", InstanceName: "instance-01", EnvVar: apptypes.EnvVar{Name: "SVC_ENV_03", Value: "svc env 03"}},
				})
				return nil
			},
		},
	}
	var w bytes.Buffer
	err := svcEnvVar.Set(context.TODO(), &apptypes.MockApp{Name: "my-app"}, []apptypes.ServiceEnvVar{
		{ServiceName: "service-01", InstanceName: "instance-01", EnvVar: apptypes.EnvVar{Name: "SVC_ENV_01", Value: "svc env 01"}},
		{ServiceName: "service-01", InstanceName: "instance-01", EnvVar: apptypes.EnvVar{Name: "SVC_ENV_02", Value: "svc env 02"}},
		{ServiceName: "service-01", InstanceName: "instance-01", EnvVar: apptypes.EnvVar{Name: "SVC_ENV_03", Value: "svc env 03"}},
	}, apptypes.SetEnvArgs{Writer: &w, ShouldRestart: false})
	c.Assert(err, check.IsNil)
	c.Assert(w.String(), check.Matches, "---- Setting 3 new environment variables ----\n")
}
