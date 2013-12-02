// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package db

import (
	"github.com/globocom/go-mgo"
	"launchpad.net/gocheck"
	"reflect"
	"sync"
	"testing"
	"time"
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

func (s *S) TearDownSuite(c *gocheck.C) {
	strg, err := storage.Open("127.0.0.1:27017", "tsuru_storage_test")
	c.Assert(err, gocheck.IsNil)
	defer strg.Close()
	//s.session.DB("tsuru_storage_test").DropDatabase()
}

func (s *S) TestUsers(c *gocheck.C) {
	storage, _ := storage.Open("127.0.0.1:27017", "tsuru_storage_test")
	defer storage.Close()
	users := storage.Users()
	usersc := storage.Collection("users")
	c.Assert(users, gocheck.DeepEquals, usersc)
	c.Assert(users, HasUniqueIndex, []string{"email"})
}

func (s *S) TestTokens(c *gocheck.C) {
	storage, _ := storage.Open("127.0.0.1:27017", "tsuru_storage_test")
	defer storage.Close()
	tokens := storage.Tokens()
	tokensc := storage.Collection("tokens")
	c.Assert(tokens, gocheck.DeepEquals, tokensc)
}

func (s *S) TestPasswordTokens(c *gocheck.C) {
	storage, _ := storage.Open("127.0.0.1:27017", "tsuru_storage_test")
	defer storage.Close()
	tokens := storage.PasswordTokens()
	tokensc := storage.Collection("password_tokens")
	c.Assert(tokens, gocheck.DeepEquals, tokensc)
}

func (s *S) TestUserActions(c *gocheck.C) {
	storage, _ := storage.Open("127.0.0.1:27017", "tsuru_storage_test")
	defer storage.Close()
	actions := storage.UserActions()
	actionsc := storage.Collection("user_actions")
	c.Assert(actions, gocheck.DeepEquals, actionsc)
}

func (s *S) TestApps(c *gocheck.C) {
	storage, _ := storage.Open("127.0.0.1:27017", "tsuru_storage_test")
	defer storage.Close()
	apps := storage.Apps()
	appsc := storage.Collection("apps")
	c.Assert(apps, gocheck.DeepEquals, appsc)
	c.Assert(apps, HasUniqueIndex, []string{"name"})
}

func (s *S) TestDeploys(c *gocheck.C) {
	storage, _ := storage.Open("127.0.0.1:27017", "tsuru_storage_test")
	defer storage.Close()
	deploys := storage.Deploys()
	deploysc := storage.Collection("deploys")
	c.Assert(deploys, gocheck.DeepEquals, deploysc)
}

func (s *S) TestPlatforms(c *gocheck.C) {
	storage, _ := storage.Open("127.0.0.1:27017", "tsuru_storage_test")
	defer storage.Close()
	plats := storage.Platforms()
	platsc := storage.Collection("platforms")
	c.Assert(plats, gocheck.DeepEquals, platsc)
}

func (s *S) TestLogs(c *gocheck.C) {
	storage, _ := storage.Open("127.0.0.1:27017", "tsuru_storage_test")
	defer storage.Close()
	logs := storage.Logs()
	logsc := storage.Collection("logs")
	c.Assert(logs, gocheck.DeepEquals, logsc)
}

func (s *S) TestLogsAppNameIndex(c *gocheck.C) {
	storage, _ := storage.Open("127.0.0.1", "tsuru_storage_test")
	defer storage.Close()
	logs := storage.Logs()
	c.Assert(logs, HasIndex, []string{"appname"})
}

func (s *S) TestLogsSourceIndex(c *gocheck.C) {
	storage, _ := storage.Open("127.0.0.1", "tsuru_storage_test")
	defer storage.Close()
	logs := storage.Logs()
	c.Assert(logs, HasIndex, []string{"source"})
}

func (s *S) TestLogsDateAscendingIndex(c *gocheck.C) {
	storage, _ := storage.Open("127.0.0.1", "tsuru_storage_test")
	defer storage.Close()
	logs := storage.Logs()
	c.Assert(logs, HasIndex, []string{"date"})
}

func (s *S) TestLogsDateDescendingIndex(c *gocheck.C) {
	storage, _ := storage.Open("127.0.0.1", "tsuru_storage_test")
	defer storage.Close()
	logs := storage.Logs()
	c.Assert(logs, HasIndex, []string{"-date"})
}

func (s *S) TestServices(c *gocheck.C) {
	storage, _ := storage.Open("127.0.0.1:27017", "tsuru_storage_test")
	defer storage.Close()
	services := storage.Services()
	servicesc := storage.Collection("services")
	c.Assert(services, gocheck.DeepEquals, servicesc)
}

func (s *S) TestServiceInstances(c *gocheck.C) {
	storage, _ := storage.Open("127.0.0.1:27017", "tsuru_storage_test")
	defer storage.Close()
	serviceInstances := storage.ServiceInstances()
	serviceInstancesc := storage.Collection("service_instances")
	c.Assert(serviceInstances, gocheck.DeepEquals, serviceInstancesc)
}

func (s *S) TestMethodTeamsShouldReturnTeamsCollection(c *gocheck.C) {
	storage, _ := storage.Open("127.0.0.1:27017", "tsuru_storage_test")
	defer storage.Close()
	teams := storage.Teams()
	teamsc := storage.Collection("teams")
	c.Assert(teams, gocheck.DeepEquals, teamsc)
}

func (s *S) TestQuota(c *gocheck.C) {
	storage, _ := storage.Open("127.0.0.1", "tsuru_storage_test")
	defer storage.Close()
	quota := storage.Quota()
	quotac := storage.Collection("quota")
	c.Assert(quota, gocheck.DeepEquals, quotac)
}

func (s *S) TestQuotaOwnerIsUnique(c *gocheck.C) {
	storage, _ := storage.Open("127.0.0.1", "tsuru_storage_test")
	defer storage.Close()
	quota := storage.Quota()
	c.Assert(quota, HasUniqueIndex, []string{"owner"})
}
