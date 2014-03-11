// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package db

import (
	"github.com/globocom/config"
	"github.com/globocom/tsuru/db/storage"
	"launchpad.net/gocheck"
	"reflect"
	"testing"
)

type hasIndexChecker struct{}

func (c *hasIndexChecker) Info() *gocheck.CheckerInfo {
	return &gocheck.CheckerInfo{Name: "HasIndexChecker", Params: []string{"collection", "key"}}
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

var HasIndex gocheck.Checker = &hasIndexChecker{}

type hasUniqueIndexChecker struct{}

func (c *hasUniqueIndexChecker) Info() *gocheck.CheckerInfo {
	return &gocheck.CheckerInfo{Name: "HasUniqueField", Params: []string{"collection", "key"}}
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

var HasUniqueIndex gocheck.Checker = &hasUniqueIndexChecker{}

func Test(t *testing.T) { gocheck.TestingT(t) }

type S struct{}

var _ = gocheck.Suite(&S{})

func (s *S) SetUpSuite(c *gocheck.C) {
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "tsuru_storage_test")
}

func (s *S) TearDownSuite(c *gocheck.C) {
	//strg, err := storage.Open("127.0.0.1:27017", "tsuru_storage_test")
	//c.Assert(err, gocheck.IsNil)
	//defer strg.Close()
	config.Unset("database:url")
	config.Unset("database:name")
	//s.session.DB("tsuru_storage_test").DropDatabase()
}

func (s *S) TestUsers(c *gocheck.C) {
	strg, err := NewStorage()
	c.Assert(err, gocheck.IsNil)
	users := strg.Users()
	usersc := strg.storage.Collection("users")
	c.Assert(users, gocheck.DeepEquals, usersc)
	c.Assert(users, HasUniqueIndex, []string{"email"})
}

func (s *S) TestTokens(c *gocheck.C) {
	strg, err := NewStorage()
	c.Assert(err, gocheck.IsNil)
	tokens := strg.Tokens()
	tokensc := strg.storage.Collection("tokens")
	c.Assert(tokens, gocheck.DeepEquals, tokensc)
}

func (s *S) TestPasswordTokens(c *gocheck.C) {
	strg, err := NewStorage()
	c.Assert(err, gocheck.IsNil)
	tokens := strg.PasswordTokens()
	tokensc := strg.storage.Collection("password_tokens")
	c.Assert(tokens, gocheck.DeepEquals, tokensc)
}

func (s *S) TestUserActions(c *gocheck.C) {
	strg, err := NewStorage()
	c.Assert(err, gocheck.IsNil)
	actions := strg.UserActions()
	actionsc := strg.storage.Collection("user_actions")
	c.Assert(actions, gocheck.DeepEquals, actionsc)
}

func (s *S) TestApps(c *gocheck.C) {
	strg, err := NewStorage()
	c.Assert(err, gocheck.IsNil)
	apps := strg.Apps()
	appsc := strg.storage.Collection("apps")
	c.Assert(apps, gocheck.DeepEquals, appsc)
	c.Assert(apps, HasUniqueIndex, []string{"name"})
}

func (s *S) TestDeploys(c *gocheck.C) {
	strg, err := NewStorage()
	c.Assert(err, gocheck.IsNil)
	deploys := strg.Deploys()
	deploysc := strg.storage.Collection("deploys")
	c.Assert(deploys, gocheck.DeepEquals, deploysc)
}

func (s *S) TestPlatforms(c *gocheck.C) {
	strg, err := NewStorage()
	c.Assert(err, gocheck.IsNil)
	plats := strg.Platforms()
	platsc := strg.storage.Collection("platforms")
	c.Assert(plats, gocheck.DeepEquals, platsc)
}

func (s *S) TestLogs(c *gocheck.C) {
	strg, err := NewStorage()
	c.Assert(err, gocheck.IsNil)
	logs := strg.Logs()
	logsc := strg.storage.Collection("logs")
	c.Assert(logs, gocheck.DeepEquals, logsc)
}

func (s *S) TestLogsAppNameIndex(c *gocheck.C) {
	strg, err := NewStorage()
	c.Assert(err, gocheck.IsNil)
	logs := strg.Logs()
	c.Assert(logs, HasIndex, []string{"appname"})
}

func (s *S) TestLogsSourceIndex(c *gocheck.C) {
	strg, err := NewStorage()
	c.Assert(err, gocheck.IsNil)
	logs := strg.Logs()
	c.Assert(logs, HasIndex, []string{"source"})
}

func (s *S) TestLogsDateAscendingIndex(c *gocheck.C) {
	strg, err := NewStorage()
	c.Assert(err, gocheck.IsNil)
	logs := strg.Logs()
	c.Assert(logs, HasIndex, []string{"date"})
}

func (s *S) TestLogsDateDescendingIndex(c *gocheck.C) {
	strg, err := NewStorage()
	c.Assert(err, gocheck.IsNil)
	logs := strg.Logs()
	c.Assert(logs, HasIndex, []string{"-date"})
}

func (s *S) TestServices(c *gocheck.C) {
	strg, err := NewStorage()
	c.Assert(err, gocheck.IsNil)
	services := strg.Services()
	servicesc := strg.storage.Collection("services")
	c.Assert(services, gocheck.DeepEquals, servicesc)
}

func (s *S) TestPlans(c *gocheck.C) {
	storage, err := NewStorage()
	c.Assert(err, gocheck.IsNil)
	plans := storage.Plans()
	plansc := storage.Collection("plans")
	c.Assert(plans, gocheck.DeepEquals, plansc)
}

func (s *S) TestServiceInstances(c *gocheck.C) {
	strg, err := NewStorage()
	c.Assert(err, gocheck.IsNil)
	serviceInstances := strg.ServiceInstances()
	serviceInstancesc := strg.storage.Collection("service_instances")
	c.Assert(serviceInstances, gocheck.DeepEquals, serviceInstancesc)
}

func (s *S) TestMethodTeamsShouldReturnTeamsCollection(c *gocheck.C) {
	strg, err := NewStorage()
	c.Assert(err, gocheck.IsNil)
	teams := strg.Teams()
	teamsc := strg.storage.Collection("teams")
	c.Assert(teams, gocheck.DeepEquals, teamsc)
}

func (s *S) TestQuota(c *gocheck.C) {
	strg, err := NewStorage()
	c.Assert(err, gocheck.IsNil)
	quota := strg.Quota()
	quotac := strg.storage.Collection("quota")
	c.Assert(quota, gocheck.DeepEquals, quotac)
}

func (s *S) TestQuotaOwnerIsUnique(c *gocheck.C) {
	strg, err := NewStorage()
	c.Assert(err, gocheck.IsNil)
	quota := strg.Quota()
	c.Assert(quota, HasUniqueIndex, []string{"owner"})
}
