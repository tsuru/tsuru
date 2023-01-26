// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package migrate

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/app/bind"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/router"
	servicemock "github.com/tsuru/tsuru/servicemanager/mock"
	appTypes "github.com/tsuru/tsuru/types/app"
	check "gopkg.in/check.v1"
)

type S struct {
	conn *db.Storage
}

func Test(t *testing.T) { check.TestingT(t) }

func (s *S) SetUpSuite(c *check.C) {
	config.Set("log:disable-syslog", true)
	config.Set("database:url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("database:name", "tsuru_app_migrate_test")
	var err error
	s.conn, err = db.Conn()
	c.Assert(err, check.IsNil)
	servicemock.SetMockService(&servicemock.MockService{})
}

func (s *S) TearDownSuite(c *check.C) {
	dbtest.ClearAllCollections(s.conn.Apps().Database)
	s.conn.Close()
}

func (s *S) SetUpTest(c *check.C) {
	dbtest.ClearAllCollections(s.conn.Apps().Database)
}

var _ = check.Suite(&S{})

func (s *S) TestMigrateAppPlanRouterToRouter(c *check.C) {
	config.Set("routers:galeb:default", true)
	defer config.Unset("routers")
	a := &app.App{Name: "with-plan-router"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	err = s.conn.Apps().Update(bson.M{"name": "with-plan-router"}, bson.M{"$set": bson.M{"plan.router": "planb"}})
	c.Assert(err, check.IsNil)
	a = &app.App{Name: "without-plan-router"}
	err = s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	c.Assert(err, check.IsNil)
	err = MigrateAppPlanRouterToRouter()
	c.Assert(err, check.IsNil)
	a, err = app.GetByName(context.TODO(), "with-plan-router")
	c.Assert(err, check.IsNil)
	c.Assert(a.Router, check.Equals, "planb")
	a, err = app.GetByName(context.TODO(), "without-plan-router")
	c.Assert(err, check.IsNil)
	c.Assert(a.Router, check.Equals, "galeb")
}

func (s *S) TestMigrateAppPlanRouterToRouterWithoutDefaultRouter(c *check.C) {
	err := MigrateAppPlanRouterToRouter()
	c.Assert(err, check.DeepEquals, router.ErrDefaultRouterNotFound)
}

func (s *S) TestMigrateAppTsuruServicesVarToServiceEnvs(c *check.C) {
	tests := []struct {
		app                 app.App
		expected            []bind.ServiceEnvVar
		expectedAllEnvs     map[string]bind.EnvVar
		expectedServicesEnv string
	}{
		{
			app:             app.App{},
			expected:        []bind.ServiceEnvVar{},
			expectedAllEnvs: map[string]bind.EnvVar{},
		},
		{
			app: app.App{
				Env: map[string]bind.EnvVar{
					"TSURU_SERVICES": {
						Name: "TSURU_SERVICES",
						Value: `{
							"srv1": [
								{"instance_name": "myinst","envs": {"ENV1": "val1"}},
								{"instance_name": "myinst2","envs": {"ENV1": "val2"}},
								{"instance_name": "myinst3","envs": {"ENV1": "val3"}}
							]
						}`,
					},
				},
			},
			expected: []bind.ServiceEnvVar{
				{
					EnvVar:       bind.EnvVar{Name: "ENV1", Value: "val1"},
					ServiceName:  "srv1",
					InstanceName: "myinst",
				},
				{
					EnvVar:       bind.EnvVar{Name: "ENV1", Value: "val2"},
					ServiceName:  "srv1",
					InstanceName: "myinst2",
				},
				{
					EnvVar:       bind.EnvVar{Name: "ENV1", Value: "val3"},
					ServiceName:  "srv1",
					InstanceName: "myinst3",
				},
			},
			expectedAllEnvs: map[string]bind.EnvVar{
				"ENV1": {Name: "ENV1", Value: "val3"},
			},
		},
		{
			app: app.App{
				Env: map[string]bind.EnvVar{
					"TSURU_SERVICES": {
						Name:  "TSURU_SERVICES",
						Value: `{"srv1": [{"instance_name": "myinst","envs": {"ENV1": "val1"}}]}`,
					},
				},
				ServiceEnvs: []bind.ServiceEnvVar{
					{
						EnvVar:       bind.EnvVar{Name: "NO_MIGRATION_VAR", Value: "valx"},
						ServiceName:  "srv2",
						InstanceName: "myinst",
					},
				},
			},
			expected: []bind.ServiceEnvVar{
				{
					EnvVar:       bind.EnvVar{Name: "ENV1", Value: "val1"},
					ServiceName:  "srv1",
					InstanceName: "myinst",
				},
				{
					EnvVar:       bind.EnvVar{Name: "NO_MIGRATION_VAR", Value: "valx"},
					ServiceName:  "srv2",
					InstanceName: "myinst",
				},
			},
			expectedAllEnvs: map[string]bind.EnvVar{
				"ENV1":             {Name: "ENV1", Value: "val1"},
				"NO_MIGRATION_VAR": {Name: "NO_MIGRATION_VAR", Value: "valx"},
			},
			expectedServicesEnv: `{"srv1":[{"instance_name": "myinst","envs": {"ENV1": "val1"}}], "srv2":[{"instance_name": "myinst","envs": {"NO_MIGRATION_VAR": "valx"}}]}`,
		},
		{
			app: app.App{
				Env: map[string]bind.EnvVar{
					"OTHER_VAR": {
						Name:  "OTHER_VAR",
						Value: "otherval",
					},
					"ENV1": {
						Name:  "ENV1",
						Value: "val1",
					},
					"TSURU_SERVICES": {
						Name: "TSURU_SERVICES",
						Value: `{
							"srv1": [{"instance_name": "myinst","envs": {"ENV1": "val1"}}],
							"srv2": [{"instance_name": "myinst","envs": {"ENV2": "val2"}}],
							"srv3": [{"instance_name": "myinst2","envs": {"ENV3": "val3"}}]
						}`,
					},
				},
			},
			expected: []bind.ServiceEnvVar{
				{
					EnvVar:       bind.EnvVar{Name: "ENV1", Value: "val1"},
					ServiceName:  "srv1",
					InstanceName: "myinst",
				},
				{
					EnvVar:       bind.EnvVar{Name: "ENV2", Value: "val2"},
					ServiceName:  "srv2",
					InstanceName: "myinst",
				},
				{
					EnvVar:       bind.EnvVar{Name: "ENV3", Value: "val3"},
					ServiceName:  "srv3",
					InstanceName: "myinst2",
				},
			},
			expectedAllEnvs: map[string]bind.EnvVar{
				"ENV1":      {Name: "ENV1", Value: "val1"},
				"ENV2":      {Name: "ENV2", Value: "val2"},
				"ENV3":      {Name: "ENV3", Value: "val3"},
				"OTHER_VAR": {Name: "OTHER_VAR", Value: "otherval"},
			},
		},
	}
	for i := range tests {
		tests[i].app.Name = fmt.Sprintf("app-%d", i)
		err := s.conn.Apps().Insert(tests[i].app)
		c.Assert(err, check.IsNil)
	}
	err := MigrateAppTsuruServicesVarToServiceEnvs()
	c.Assert(err, check.IsNil)
	var resultApps []app.App
	var dbApp *app.App
	for _, tt := range tests {
		dbApp, err = app.GetByName(context.TODO(), tt.app.Name)
		c.Assert(err, check.IsNil)
		resultApps = append(resultApps, *dbApp)
		c.Assert(dbApp.ServiceEnvs, check.DeepEquals, tt.expected)
		allEnvs := dbApp.Envs()
		if tt.expectedServicesEnv == "" {
			tt.expectedServicesEnv = tt.app.Env[app.TsuruServicesEnvVar].Value
		}
		if tt.expectedServicesEnv != "" {
			var oldServicesEnvVar map[string]interface{}
			var newServicesEnvVar map[string]interface{}
			err = json.Unmarshal([]byte(tt.expectedServicesEnv), &oldServicesEnvVar)
			c.Assert(err, check.IsNil)
			err = json.Unmarshal([]byte(allEnvs[app.TsuruServicesEnvVar].Value), &newServicesEnvVar)
			c.Assert(err, check.IsNil)
			c.Assert(oldServicesEnvVar, check.DeepEquals, newServicesEnvVar)
		}
		delete(allEnvs, app.TsuruServicesEnvVar)
		c.Assert(allEnvs, check.DeepEquals, tt.expectedAllEnvs)
	}
	// Running again should change nothing
	err = MigrateAppTsuruServicesVarToServiceEnvs()
	c.Assert(err, check.IsNil)
	for i, tt := range tests {
		dbApp, err = app.GetByName(context.TODO(), tt.app.Name)
		c.Assert(err, check.IsNil)
		c.Assert(dbApp, check.DeepEquals, &resultApps[i])
	}
}

func (s *S) TestMigrateAppTsuruServicesVarToServiceEnvsNothingToDo(c *check.C) {
	err := MigrateAppTsuruServicesVarToServiceEnvs()
	c.Assert(err, check.IsNil)
}

func (s *S) TestMigrateAppPlanIDToPlanName(c *check.C) {
	a := &app.App{Name: "app-with-plan-name", Plan: appTypes.Plan{Name: "plan-name"}}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	a = &app.App{Name: "app-with-plan-id"}
	err = s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	err = s.conn.Apps().Update(bson.M{"name": "app-with-plan-id"}, bson.M{"$set": bson.M{"plan._id": "plan-id"}})
	c.Assert(err, check.IsNil)
	err = MigrateAppPlanIDToPlanName()
	c.Assert(err, check.IsNil)
	a, err = app.GetByName(context.TODO(), "app-with-plan-name")
	c.Assert(err, check.IsNil)
	c.Assert(a.Plan.Name, check.Equals, "plan-name")
	a, err = app.GetByName(context.TODO(), "app-with-plan-id")
	c.Assert(err, check.IsNil)
	c.Assert(a.Plan.Name, check.Equals, "plan-id")
}
