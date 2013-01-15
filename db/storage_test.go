// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package db

import (
	"github.com/globocom/config"
	"labix.org/v2/mgo"
	. "launchpad.net/gocheck"
	"reflect"
	"testing"
)

type hasUniqueIndexChecker struct{}

func (c *hasUniqueIndexChecker) Info() *CheckerInfo {
	return &CheckerInfo{Name: "HasUniqueField", Params: []string{"collection", "key"}}
}

func (c *hasUniqueIndexChecker) Check(params []interface{}, names []string) (bool, string) {
	collection, ok := params[0].(*mgo.Collection)
	if !ok {
		return false, "first parameter should be a mgo collection"
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

var HasUniqueIndex Checker = &hasUniqueIndexChecker{}

func Test(t *testing.T) { TestingT(t) }

type S struct{}

var _ = Suite(&S{})

func (s *S) TearDownSuite(c *C) {
	storage, err := Open("127.0.0.1:27017", "tsuru_storage_test")
	c.Assert(err, IsNil)
	defer storage.Close()
	storage.session.DB("tsuru_storage_test").DropDatabase()
}

func (s *S) TearDownTest(c *C) {
	conn = make(map[string]*Storage)
}

func (s *S) TestOpenConnectsToTheDatabase(c *C) {
	storage, err := Open("127.0.0.1:27017", "tsuru_storage_test")
	c.Assert(err, IsNil)
	defer storage.Close()
	c.Assert(storage.session.Ping(), IsNil)
}

func (s *S) TestOpenStoresConnectionInThePool(c *C) {
	storage, err := Open("127.0.0.1:27017", "tsuru_storage_test")
	c.Assert(err, IsNil)
	defer storage.Close()
	c.Assert(storage, Equals, conn["127.0.0.1:27017tsuru_storage_test"])
}

func (s *S) TestOpenReusesConnection(c *C) {
	storage, err := Open("127.0.0.1:27017", "tsuru_storage_test")
	c.Assert(err, IsNil)
	defer storage.Close()
	storage2, err := Open("127.0.0.1:27017", "tsuru_storage_test")
	c.Assert(err, IsNil)
	c.Assert(storage, Equals, storage2)
}

func (s *S) TestOpenReconnects(c *C) {
	storage, err := Open("127.0.0.1:27017", "tsuru_storage_test")
	c.Assert(err, IsNil)
	storage.Close()
	storage, err = Open("127.0.0.1:27017", "tsuru_storage_test")
	c.Assert(err, IsNil)
	err = storage.session.Ping()
	c.Assert(err, IsNil)
}

func (s *S) TestOpenConnectionRefused(c *C) {
	storage, err := Open("127.0.0.1:27018", "tsuru_storage_test")
	c.Assert(storage, IsNil)
	c.Assert(err, NotNil)
}

func (s *S) TestConn(c *C) {
	config.Set("database:url", "127.0.0.1:27017")
	defer config.Unset("database:url")
	config.Set("database:name", "tsuru_storage_test")
	defer config.Unset("database:name")
	storage, err := Conn()
	c.Assert(err, IsNil)
	defer storage.Close()
	err = storage.session.Ping()
	c.Assert(err, IsNil)
}

func (s *S) TestConnMissingDatabaseUrl(c *C) {
	storage, err := Conn()
	c.Assert(storage, IsNil)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, `configuration error: key "database:url" not found`)
}

func (s *S) TestConnMissingDatabaseName(c *C) {
	config.Set("database:url", "127.0.0.1:27017")
	defer config.Unset("database:url")
	storage, err := Conn()
	c.Assert(storage, IsNil)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, `configuration error: key "database:name" not found`)
}

func (s *S) TestCloseClosesTheConnectionWithMongoDB(c *C) {
	defer func() {
		if r := recover(); r == nil {
			c.Errorf("Should close the connection.")
		}
	}()
	storage, _ := Open("127.0.0.1:27017", "tsuru_storage_test")
	storage.Close()
	err := storage.session.Ping()
	c.Assert(err, NotNil)
}

func (s *S) TestCloseKeepsTheConnectionInThePool(c *C) {
	storage, _ := Open("127.0.0.1:27017", "tsuru_storage_test")
	storage.Close()
	_, ok := conn["127.0.0.1:27017tsuru_storage_test"]
	c.Assert(ok, Equals, true)
}

func (s *S) TestCollection(c *C) {
	storage, _ := Open("127.0.0.1:27017", "tsuru_storage_test")
	defer storage.Close()
	collection := storage.Collection("users")
	c.Assert(collection.FullName, Equals, storage.dbname+".users")
}

func (s *S) TestUsers(c *C) {
	storage, _ := Open("127.0.0.1:27017", "tsuru_storage_test")
	defer storage.Close()
	users := storage.Users()
	usersc := storage.Collection("users")
	c.Assert(users, DeepEquals, usersc)
	c.Assert(users, HasUniqueIndex, []string{"email"})
}

func (s *S) TestApps(c *C) {
	storage, _ := Open("127.0.0.1:27017", "tsuru_storage_test")
	defer storage.Close()
	apps := storage.Apps()
	appsc := storage.Collection("apps")
	c.Assert(apps, DeepEquals, appsc)
	c.Assert(apps, HasUniqueIndex, []string{"name"})
}

func (s *S) TestServices(c *C) {
	storage, _ := Open("127.0.0.1:27017", "tsuru_storage_test")
	defer storage.Close()
	services := storage.Services()
	servicesc := storage.Collection("services")
	c.Assert(services, DeepEquals, servicesc)
}

func (s *S) TestServiceInstances(c *C) {
	storage, _ := Open("127.0.0.1:27017", "tsuru_storage_test")
	defer storage.Close()
	serviceInstances := storage.ServiceInstances()
	serviceInstancesc := storage.Collection("service_instances")
	c.Assert(serviceInstances, DeepEquals, serviceInstancesc)
}

func (s *S) TestMethodTeamsShouldReturnTeamsCollection(c *C) {
	storage, _ := Open("127.0.0.1:27017", "tsuru_storage_test")
	defer storage.Close()
	teams := storage.Teams()
	teamsc := storage.Collection("teams")
	c.Assert(teams, DeepEquals, teamsc)
}
