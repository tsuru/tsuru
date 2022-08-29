// Copyright 2022 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storagetest

import (
	"context"

	check "gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"

	"github.com/tsuru/tsuru/db"
	apptypes "github.com/tsuru/tsuru/types/app"
)

type AppServiceEnvVarSuite struct {
	SuiteHooks
	AppServiceEnvVarStorage apptypes.AppServiceEnvVarStorage
}

func (s *AppServiceEnvVarSuite) TestFindAll_ContextCanceled(c *check.C) {
	ctx, cancel := context.WithCancel(context.TODO())
	cancel()
	_, err := s.AppServiceEnvVarStorage.FindAll(ctx, nil)
	c.Assert(err, check.NotNil)
}

func (s *AppServiceEnvVarSuite) TestFindAll(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	err = conn.Apps().Insert(bson.M{"name": "my-app", "serviceenvs": []interface{}{
		map[string]interface{}{"servicename": "mysql", "instancename": "my-instance-01", "name": "DATABASE_URL_STRING", "value": "mysql://user:passwd@mysql.example.com:3306/my-database", "public": false},
		map[string]interface{}{"servicename": "s3", "instancename": "my-instance-02", "name": "S3_URL_BUCKET", "value": "https://my-bucket.s3.example.com/v1/auth_0x123/", "public": true},
	}})
	c.Assert(err, check.IsNil)
	envs, err := s.AppServiceEnvVarStorage.FindAll(context.TODO(), &apptypes.MockApp{Name: "my-app"})
	c.Assert(err, check.IsNil)
	c.Assert(envs, check.DeepEquals, []apptypes.ServiceEnvVar{
		{ServiceName: "mysql", InstanceName: "my-instance-01", EnvVar: apptypes.EnvVar{Name: "DATABASE_URL_STRING", Value: "mysql://user:passwd@mysql.example.com:3306/my-database"}},
		{ServiceName: "s3", InstanceName: "my-instance-02", EnvVar: apptypes.EnvVar{Name: "S3_URL_BUCKET", Value: "https://my-bucket.s3.example.com/v1/auth_0x123/", Public: true}},
	})
}

func (s *AppServiceEnvVarSuite) TestRemove_ContextCanceled(c *check.C) {
	ctx, cancel := context.WithCancel(context.TODO())
	cancel()
	err := s.AppServiceEnvVarStorage.Remove(ctx, nil, nil)
	c.Assert(err, check.NotNil)
}

func (s *AppServiceEnvVarSuite) TestRemove(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	err = conn.Apps().Insert(bson.M{"name": "my-app", "serviceenvs": []interface{}{
		map[string]interface{}{"servicename": "mysql", "instancename": "my-instance-01", "name": "DATABASE_HOST", "value": "mysql.example.com", "public": true},
		map[string]interface{}{"servicename": "mysql", "instancename": "my-instance-01", "name": "DATABASE_PORT", "value": "3306", "public": true},
		map[string]interface{}{"servicename": "mysql", "instancename": "my-instance-01", "name": "DATABASE_USER", "value": "usr01", "public": false},
		map[string]interface{}{"servicename": "mysql", "instancename": "my-instance-01", "name": "DATABASE_PASSWORD", "value": "changeit", "public": false},
		map[string]interface{}{"servicename": "mysql", "instancename": "my-instance-01", "name": "DATABASE_NAME", "value": "my-database", "public": true},
		map[string]interface{}{"servicename": "mysql", "instancename": "my-instance-01", "name": "DATABASE_URL_STRING", "value": "mysql://usr01:changeit@mysql.example.com:3306/my-database", "public": false},
		map[string]interface{}{"servicename": "s3", "instancename": "my-instance-02", "name": "S3_URL_BUCKET", "value": "https://my-bucket.s3.example.com/v1/auth_0x123/", "public": true},
	}})
	c.Assert(err, check.IsNil)
	toRemove := []apptypes.ServiceEnvVarIdentifier{
		apptypes.ServiceEnvVar{ServiceName: "mysql", InstanceName: "my-instance-01", EnvVar: apptypes.EnvVar{Name: "DATABASE_HOST"}},
		apptypes.ServiceEnvVar{ServiceName: "mysql", InstanceName: "my-instance-01", EnvVar: apptypes.EnvVar{Name: "DATABASE_PORT"}},
		apptypes.ServiceEnvVar{ServiceName: "mysql", InstanceName: "my-instance-01", EnvVar: apptypes.EnvVar{Name: "DATABASE_USER"}},
		apptypes.ServiceEnvVar{ServiceName: "mysql", InstanceName: "my-instance-01", EnvVar: apptypes.EnvVar{Name: "DATABASE_PASSWORD"}},
		apptypes.ServiceEnvVar{ServiceName: "mysql", InstanceName: "my-instance-01", EnvVar: apptypes.EnvVar{Name: "DATABASE_NAME"}},
		apptypes.ServiceEnvVar{ServiceName: "mysql", InstanceName: "my-instance-01", EnvVar: apptypes.EnvVar{Name: "DATABASE_URL_STRING"}},
	}
	err = s.AppServiceEnvVarStorage.Remove(context.TODO(), &apptypes.MockApp{Name: "my-app"}, toRemove)
	c.Assert(err, check.IsNil)
	var app struct{ ServiceEnvs []apptypes.ServiceEnvVar }
	err = conn.Apps().Find(bson.M{"name": "my-app"}).One(&app)
	c.Assert(err, check.IsNil)
	c.Assert(app.ServiceEnvs, check.DeepEquals, []apptypes.ServiceEnvVar{
		{ServiceName: "s3", InstanceName: "my-instance-02", EnvVar: apptypes.EnvVar{Name: "S3_URL_BUCKET", Value: "https://my-bucket.s3.example.com/v1/auth_0x123/", Public: true}},
	})
}

func (s *AppServiceEnvVarSuite) TestRemove_EnvVarNotFound(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	err = conn.Apps().Insert(bson.M{"name": "my-app", "serviceenvs": []interface{}{}})
	c.Assert(err, check.IsNil)
	toRemove := []apptypes.ServiceEnvVarIdentifier{
		apptypes.ServiceEnvVar{ServiceName: "mysql", InstanceName: "my-instance-01", EnvVar: apptypes.EnvVar{Name: "DATABASE_URL_STRING"}},
	}
	err = s.AppServiceEnvVarStorage.Remove(context.TODO(), &apptypes.MockApp{Name: "my-app"}, toRemove)
	c.Assert(err, check.ErrorMatches, "service env var not found")
}

func (s *AppServiceEnvVarSuite) TestUpsert_ContextCanceled(c *check.C) {
	ctx, cancel := context.WithCancel(context.TODO())
	cancel()
	err := s.AppServiceEnvVarStorage.Upsert(ctx, nil, nil)
	c.Assert(err, check.NotNil)
}

func (s *AppServiceEnvVarSuite) TestUpsert(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	err = conn.Apps().Insert(bson.M{"name": "my-app", "serviceenvs": []interface{}{
		map[string]interface{}{"servicename": "s3", "instancename": "my-instance-02", "name": "S3_URL_BUCKET", "value": "https://my-bucket.s3.example.com/v1/auth_0x123/", "public": true},
		map[string]interface{}{"servicename": "s3", "instancename": "my-instance-02", "name": "S3_INTERNAL_ENDPOINT", "value": "https://s3.my.company.local"},
	}})
	c.Assert(err, check.IsNil)
	toUpdate := []apptypes.ServiceEnvVar{
		{ServiceName: "s3", InstanceName: "my-instance-02", EnvVar: apptypes.EnvVar{Name: "S3_AUTH_TOKEN", Value: "awesome token"}},
		{ServiceName: "s3", InstanceName: "my-instance-02", EnvVar: apptypes.EnvVar{Name: "S3_INTERNAL_ENDPOINT", Value: "https://s3.company.example.com", Public: true}},
	}
	err = s.AppServiceEnvVarStorage.Upsert(context.TODO(), &apptypes.MockApp{Name: "my-app"}, toUpdate)
	c.Assert(err, check.IsNil)
	var app struct{ ServiceEnvs []apptypes.ServiceEnvVar }
	err = conn.Apps().Find(bson.M{"name": "my-app"}).One(&app)
	c.Assert(err, check.IsNil)
	c.Assert(app.ServiceEnvs, check.DeepEquals, []apptypes.ServiceEnvVar{
		{ServiceName: "s3", InstanceName: "my-instance-02", EnvVar: apptypes.EnvVar{Name: "S3_URL_BUCKET", Value: "https://my-bucket.s3.example.com/v1/auth_0x123/", Public: true}},
		{ServiceName: "s3", InstanceName: "my-instance-02", EnvVar: apptypes.EnvVar{Name: "S3_INTERNAL_ENDPOINT", Value: "https://s3.company.example.com", Public: true}},
		{ServiceName: "s3", InstanceName: "my-instance-02", EnvVar: apptypes.EnvVar{Name: "S3_AUTH_TOKEN", Value: "awesome token"}},
	})
}
