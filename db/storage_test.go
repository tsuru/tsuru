// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package db

import (
	"reflect"
	"testing"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db/storage"
	"gopkg.in/check.v1"
)

type hasIndexChecker struct{}

func (c *hasIndexChecker) Info() *check.CheckerInfo {
	return &check.CheckerInfo{Name: "HasIndexChecker", Params: []string{"collection", "key"}}
}

func (c *hasIndexChecker) Check(params []interface{}, names []string) (bool, string) {
	collection, ok := params[0].(*storage.Collection)
	if !ok {
		return false, "first parameter should be a Collection"
	}
	key, ok := params[1].([]string)
	if !ok {
		return false, "second parameter should be the key, as used for mgo index declaration (slice of strings)"
	}
	indexes, err := collection.Indexes()
	if err != nil {
		return false, "failed to get collection indexes: " + err.Error()
	}
	for _, index := range indexes {
		if reflect.DeepEqual(index.Key, key) {
			return true, ""
		}
	}
	return false, ""
}

var HasIndex check.Checker = &hasIndexChecker{}

type hasUniqueIndexChecker struct{}

func (c *hasUniqueIndexChecker) Info() *check.CheckerInfo {
	return &check.CheckerInfo{Name: "HasUniqueField", Params: []string{"collection", "key"}}
}

func (c *hasUniqueIndexChecker) Check(params []interface{}, names []string) (bool, string) {
	collection, ok := params[0].(*storage.Collection)
	if !ok {
		return false, "first parameter should be a Collection"
	}
	key, ok := params[1].([]string)
	if !ok {
		return false, "second parameter should be the key, as used for mgo index declaration (slice of strings)"
	}
	indexes, err := collection.Indexes()
	if err != nil {
		return false, "failed to get collection indexes: " + err.Error()
	}
	for _, index := range indexes {
		if reflect.DeepEqual(index.Key, key) {
			return index.Unique, ""
		}
	}
	return false, ""
}

var HasUniqueIndex check.Checker = &hasUniqueIndexChecker{}

func Test(t *testing.T) { check.TestingT(t) }

type S struct{}

var _ = check.Suite(&S{})

func (s *S) SetUpSuite(c *check.C) {
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "tsuru_storage_test")
}

func (s *S) TearDownSuite(c *check.C) {
	strg, err := Conn()
	c.Assert(err, check.IsNil)
	defer strg.Close()
	config.Unset("database:url")
	config.Unset("database:name")
	strg.Collection("apps").Database.DropDatabase()
}

func (s *S) TestHealthCheck(c *check.C) {
	err := healthCheck()
	c.Assert(err, check.IsNil)
}

func (s *S) TestHealthCheckFailure(c *check.C) {
	config.Set("database:url", "localhost:34456")
	defer config.Unset("database:url")
	err := healthCheck()
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "no reachable servers")
}

func (s *S) TestUsers(c *check.C) {
	strg, err := Conn()
	c.Assert(err, check.IsNil)
	users := strg.Users()
	usersc := strg.Collection("users")
	c.Assert(users, check.DeepEquals, usersc)
	c.Assert(users, HasUniqueIndex, []string{"email"})
}

func (s *S) TestTokens(c *check.C) {
	strg, err := Conn()
	c.Assert(err, check.IsNil)
	tokens := strg.Tokens()
	tokensc := strg.Collection("tokens")
	c.Assert(tokens, check.DeepEquals, tokensc)
}

func (s *S) TestPasswordTokens(c *check.C) {
	strg, err := Conn()
	c.Assert(err, check.IsNil)
	tokens := strg.PasswordTokens()
	tokensc := strg.Collection("password_tokens")
	c.Assert(tokens, check.DeepEquals, tokensc)
}

func (s *S) TestUserActions(c *check.C) {
	strg, err := Conn()
	c.Assert(err, check.IsNil)
	actions := strg.UserActions()
	actionsc := strg.Collection("user_actions")
	c.Assert(actions, check.DeepEquals, actionsc)
}

func (s *S) TestApps(c *check.C) {
	strg, err := Conn()
	c.Assert(err, check.IsNil)
	apps := strg.Apps()
	appsc := strg.Collection("apps")
	c.Assert(apps, check.DeepEquals, appsc)
	c.Assert(apps, HasUniqueIndex, []string{"name"})
}

func (s *S) TestAutoScale(c *check.C) {
	strg, err := Conn()
	c.Assert(err, check.IsNil)
	autoscale := strg.AutoScale()
	autoscalec := strg.Collection("autoscale")
	c.Assert(autoscale, check.DeepEquals, autoscalec)
}

func (s *S) TestDeploys(c *check.C) {
	strg, err := Conn()
	c.Assert(err, check.IsNil)
	deploys := strg.Deploys()
	deploysc := strg.Collection("deploys")
	c.Assert(deploys, check.DeepEquals, deploysc)
}

func (s *S) TestPlatforms(c *check.C) {
	strg, err := Conn()
	c.Assert(err, check.IsNil)
	plats := strg.Platforms()
	platsc := strg.Collection("platforms")
	c.Assert(plats, check.DeepEquals, platsc)
}

func (s *S) TestLogs(c *check.C) {
	strg, err := Conn()
	c.Assert(err, check.IsNil)
	logs := strg.Logs("myapp")
	logsc := strg.Collection("logs_myapp")
	c.Assert(logs, check.DeepEquals, logsc)
}

func (s *S) TestLogsSourceIndex(c *check.C) {
	strg, err := Conn()
	c.Assert(err, check.IsNil)
	logs := strg.Logs("app1")
	c.Assert(logs, HasIndex, []string{"source"})
}

func (s *S) TestLogsUnitIndex(c *check.C) {
	strg, err := Conn()
	c.Assert(err, check.IsNil)
	logs := strg.Logs("app1")
	c.Assert(logs, HasIndex, []string{"unit"})
}

func (s *S) TestServices(c *check.C) {
	strg, err := Conn()
	c.Assert(err, check.IsNil)
	services := strg.Services()
	servicesc := strg.Collection("services")
	c.Assert(services, check.DeepEquals, servicesc)
}

func (s *S) TestPlans(c *check.C) {
	storage, err := Conn()
	c.Assert(err, check.IsNil)
	plans := storage.Plans()
	plansc := storage.Collection("plans")
	c.Assert(plans, check.DeepEquals, plansc)
}

func (s *S) TestServiceInstances(c *check.C) {
	strg, err := Conn()
	c.Assert(err, check.IsNil)
	serviceInstances := strg.ServiceInstances()
	serviceInstancesc := strg.Collection("service_instances")
	c.Assert(serviceInstances, check.DeepEquals, serviceInstancesc)
}

func (s *S) TestMethodTeamsShouldReturnTeamsCollection(c *check.C) {
	strg, err := Conn()
	c.Assert(err, check.IsNil)
	teams := strg.Teams()
	teamsc := strg.Collection("teams")
	c.Assert(teams, check.DeepEquals, teamsc)
}

func (s *S) TestQuota(c *check.C) {
	strg, err := Conn()
	c.Assert(err, check.IsNil)
	quota := strg.Quota()
	quotac := strg.Collection("quota")
	c.Assert(quota, check.DeepEquals, quotac)
}

func (s *S) TestQuotaOwnerIsUnique(c *check.C) {
	strg, err := Conn()
	c.Assert(err, check.IsNil)
	quota := strg.Quota()
	c.Assert(quota, HasUniqueIndex, []string{"owner"})
}
