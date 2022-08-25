// Copyright 2022 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storagetest

import (
	"context"

	"github.com/tsuru/tsuru/db"
	apptypes "github.com/tsuru/tsuru/types/app"
	check "gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

type AppEnvVarSuite struct {
	SuiteHooks
	AppEnvVarStorage apptypes.AppEnvVarStorage
}

func (s *AppEnvVarSuite) TestListAppEnvs(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	err = conn.Apps().Insert(bson.M{"name": "app-1", "env": map[string]interface{}{
		"MY_ENV_1": map[string]interface{}{"name": "MY_ENV_1", "value": "content from env 1"},
		"MY_ENV_2": map[string]interface{}{"name": "MY_ENV_2", "value": "content from env 2", "public": true},
		"MY_ENV_3": map[string]interface{}{"name": "MY_ENV_3", "value": "content from env 3", "managedby": "terraform"},
	}})
	c.Assert(err, check.IsNil)
	envs, err := s.AppEnvVarStorage.ListAppEnvs(context.TODO(), "app-1")
	c.Assert(err, check.IsNil)
	c.Assert(envs, check.DeepEquals, []apptypes.EnvVar{
		{Name: "MY_ENV_1", Value: "content from env 1"},
		{Name: "MY_ENV_2", Value: "content from env 2", Public: true},
		{Name: "MY_ENV_3", Value: "content from env 3", ManagedBy: "terraform"},
	})
}

func (s *AppEnvVarSuite) TestListServiceEnvs(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	err = conn.Apps().Insert(bson.M{"name": "app-1", "serviceenvs": []interface{}{
		map[string]interface{}{"name": "SVC1_INST1_ENV_1", "value": "v1", "public": true, "servicename": "svc1", "instancename": "instance1"},
		map[string]interface{}{"name": "SVC1_INST1_ENV_2", "value": "v2", "public": false, "servicename": "svc1", "instancename": "instance1"},
		map[string]interface{}{"name": "SVC2_INST2_ENV_A", "value": "foo", "public": false, "servicename": "svc2", "instancename": "instance2"},
	}})
	c.Assert(err, check.IsNil)
	svcEnvs, err := s.AppEnvVarStorage.ListServiceEnvs(context.TODO(), "app-1")
	c.Assert(err, check.IsNil)
	c.Assert(svcEnvs, check.DeepEquals, []apptypes.ServiceEnvVar{
		{EnvVar: apptypes.EnvVar{Name: "SVC1_INST1_ENV_1", Value: "v1", Public: true}, ServiceName: "svc1", InstanceName: "instance1"},
		{EnvVar: apptypes.EnvVar{Name: "SVC1_INST1_ENV_2", Value: "v2"}, ServiceName: "svc1", InstanceName: "instance1"},
		{EnvVar: apptypes.EnvVar{Name: "SVC2_INST2_ENV_A", Value: "foo"}, ServiceName: "svc2", InstanceName: "instance2"},
	})
}
