// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package db

import (
	"context"
	"reflect"
	"testing"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/db/storage"
	check "gopkg.in/check.v1"
)

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
	config.Set("log:disable-syslog", true)
	config.Set("database:url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("database:name", "tsuru_db_storage_test")
}

func (s *S) TearDownSuite(c *check.C) {
	strg, err := Conn()
	c.Assert(err, check.IsNil)
	defer strg.Close()
	config.Unset("database:url")
	config.Unset("database:name")
	dbtest.ClearAllCollections(strg.Apps().Database)
}

func (s *S) TestHealthCheck(c *check.C) {
	err := healthCheck(context.TODO())
	c.Assert(err, check.IsNil)
}

func (s *S) TestUsers(c *check.C) {
	strg, err := Conn()
	c.Assert(err, check.IsNil)
	defer strg.Close()
	users := strg.Users()
	usersc := strg.Collection("users")
	c.Assert(users, check.DeepEquals, usersc)
	c.Assert(users, HasUniqueIndex, []string{"email"})
}

func (s *S) TestTokens(c *check.C) {
	strg, err := Conn()
	c.Assert(err, check.IsNil)
	defer strg.Close()
	tokens := strg.Tokens()
	tokensc := strg.Collection("tokens")
	c.Assert(tokens, check.DeepEquals, tokensc)
}

func (s *S) TestPasswordTokens(c *check.C) {
	strg, err := Conn()
	c.Assert(err, check.IsNil)
	defer strg.Close()
	tokens := strg.PasswordTokens()
	tokensc := strg.Collection("password_tokens")
	c.Assert(tokens, check.DeepEquals, tokensc)
}

func (s *S) TestApps(c *check.C) {
	strg, err := Conn()
	c.Assert(err, check.IsNil)
	defer strg.Close()
	apps := strg.Apps()
	appsc := strg.Collection("apps")
	c.Assert(apps, check.DeepEquals, appsc)
	c.Assert(apps, HasUniqueIndex, []string{"name"})
}

func (s *S) TestServices(c *check.C) {
	strg, err := Conn()
	c.Assert(err, check.IsNil)
	defer strg.Close()
	services := strg.Services()
	servicesc := strg.Collection("services")
	c.Assert(services, check.DeepEquals, servicesc)
}

func (s *S) TestPools(c *check.C) {
	storage, err := Conn()
	c.Assert(err, check.IsNil)
	defer storage.Close()
	pools := storage.Pools()
	poolsc := storage.Collection("pool")
	c.Assert(pools, check.DeepEquals, poolsc)
}

func (s *S) TestServiceInstances(c *check.C) {
	strg, err := Conn()
	c.Assert(err, check.IsNil)
	defer strg.Close()
	serviceInstances := strg.ServiceInstances()
	serviceInstancesc := strg.Collection("service_instances")
	c.Assert(serviceInstances, check.DeepEquals, serviceInstancesc)
}

func (s *S) TestRoles(c *check.C) {
	strg, err := Conn()
	c.Assert(err, check.IsNil)
	defer strg.Close()
	roles := strg.Roles()
	rolesc := strg.Collection("roles")
	c.Assert(roles, check.DeepEquals, rolesc)
}
