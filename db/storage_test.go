// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package db

import (
	"github.com/globocom/config"
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
	collection, ok := params[0].(*Collection)
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
	collection, ok := params[0].(*Collection)
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
	ticker.Stop()
}

func (s *S) TearDownSuite(c *gocheck.C) {
	storage, err := Open("127.0.0.1:27017", "tsuru_storage_test")
	c.Assert(err, gocheck.IsNil)
	defer storage.session.Close()
	storage.session.DB("tsuru_storage_test").DropDatabase()
}

func (s *S) TearDownTest(c *gocheck.C) {
	conn = make(map[string]*session)
}

func (s *S) TestOpenConnectsToTheDatabase(c *gocheck.C) {
	storage, err := Open("127.0.0.1:27017", "tsuru_storage_test")
	c.Assert(err, gocheck.IsNil)
	defer storage.session.Close()
	c.Assert(storage.session.Ping(), gocheck.IsNil)
}

func (s *S) TestOpenCopiesConnection(c *gocheck.C) {
	storage, err := Open("127.0.0.1:27017", "tsuru_storage_test")
	c.Assert(err, gocheck.IsNil)
	defer storage.session.Close()
	storage2, err := Open("127.0.0.1:27017", "tsuru_storage_test")
	c.Assert(err, gocheck.IsNil)
	c.Assert(storage.session, gocheck.Not(gocheck.Equals), storage2.session)
}

func (s *S) TestOpenReconnects(c *gocheck.C) {
	storage, err := Open("127.0.0.1:27017", "tsuru_storage_test")
	c.Assert(err, gocheck.IsNil)
	storage.session.Close()
	storage, err = Open("127.0.0.1:27017", "tsuru_storage_test")
	c.Assert(err, gocheck.IsNil)
	err = storage.session.Ping()
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestOpenConnectionRefused(c *gocheck.C) {
	storage, err := Open("127.0.0.1:27018", "tsuru_storage_test")
	c.Assert(storage, gocheck.IsNil)
	c.Assert(err, gocheck.NotNil)
}

func (s *S) TestClose(c *gocheck.C) {
	defer func() {
		r := recover()
		c.Check(r, gocheck.NotNil)
	}()
	storage, err := Open("127.0.0.1:27017", "tsuru_storage_test")
	c.Assert(err, gocheck.IsNil)
	storage.Close()
	err = storage.session.Ping()
	c.Check(err, gocheck.NotNil)
}

func (s *S) TestConn(c *gocheck.C) {
	config.Set("database:url", "127.0.0.1:27017")
	defer config.Unset("database:url")
	config.Set("database:name", "tsuru_storage_test")
	defer config.Unset("database:name")
	storage, err := Conn()
	c.Assert(err, gocheck.IsNil)
	defer storage.session.Close()
	err = storage.session.Ping()
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestConnMissingConfiguration(c *gocheck.C) {
	storage, err := Conn()
	c.Assert(err, gocheck.IsNil)
	defer storage.session.Close()
	err = storage.session.Ping()
	c.Assert(err, gocheck.IsNil)
	c.Assert(storage.dbname, gocheck.Equals, "tsuru")
	c.Assert(storage.session.LiveServers(), gocheck.DeepEquals, []string{"127.0.0.1:27017"})
}

func (s *S) TestCollection(c *gocheck.C) {
	storage, _ := Open("127.0.0.1:27017", "tsuru_storage_test")
	defer storage.session.Close()
	collection := storage.Collection("users")
	c.Assert(collection.FullName, gocheck.Equals, storage.dbname+".users")
}

func (s *S) TestUsers(c *gocheck.C) {
	storage, _ := Open("127.0.0.1:27017", "tsuru_storage_test")
	defer storage.session.Close()
	users := storage.Users()
	usersc := storage.Collection("users")
	c.Assert(users, gocheck.DeepEquals, usersc)
	c.Assert(users, HasUniqueIndex, []string{"email"})
}

func (s *S) TestTokens(c *gocheck.C) {
	storage, _ := Open("127.0.0.1:27017", "tsuru_storage_test")
	defer storage.session.Close()
	tokens := storage.Tokens()
	tokensc := storage.Collection("tokens")
	c.Assert(tokens, gocheck.DeepEquals, tokensc)
}

func (s *S) TestPasswordTokens(c *gocheck.C) {
	storage, _ := Open("127.0.0.1:27017", "tsuru_storage_test")
	defer storage.session.Close()
	tokens := storage.PasswordTokens()
	tokensc := storage.Collection("password_tokens")
	c.Assert(tokens, gocheck.DeepEquals, tokensc)
}

func (s *S) TestUserActions(c *gocheck.C) {
	storage, _ := Open("127.0.0.1:27017", "tsuru_storage_test")
	defer storage.session.Close()
	actions := storage.UserActions()
	actionsc := storage.Collection("user_actions")
	c.Assert(actions, gocheck.DeepEquals, actionsc)
}

func (s *S) TestApps(c *gocheck.C) {
	storage, _ := Open("127.0.0.1:27017", "tsuru_storage_test")
	defer storage.session.Close()
	apps := storage.Apps()
	appsc := storage.Collection("apps")
	c.Assert(apps, gocheck.DeepEquals, appsc)
	c.Assert(apps, HasUniqueIndex, []string{"name"})
}

func (s *S) TestPlatforms(c *gocheck.C) {
	storage, _ := Open("127.0.0.1:27017", "tsuru_storage_test")
	defer storage.session.Close()
	plats := storage.Platforms()
	platsc := storage.Collection("platforms")
	c.Assert(plats, gocheck.DeepEquals, platsc)
}

func (s *S) TestLogs(c *gocheck.C) {
	storage, _ := Open("127.0.0.1:27017", "tsuru_storage_test")
	defer storage.session.Close()
	logs := storage.Logs()
	logsc := storage.Collection("logs")
	c.Assert(logs, gocheck.DeepEquals, logsc)
}

func (s *S) TestLogsAppNameIndex(c *gocheck.C) {
	storage, _ := Open("127.0.0.1", "tsuru_storage_test")
	defer storage.session.Close()
	logs := storage.Logs()
	c.Assert(logs, HasIndex, []string{"appname"})
}

func (s *S) TestLogsSourceIndex(c *gocheck.C) {
	storage, _ := Open("127.0.0.1", "tsuru_storage_test")
	defer storage.session.Close()
	logs := storage.Logs()
	c.Assert(logs, HasIndex, []string{"source"})
}

func (s *S) TestLogsDateAscendingIndex(c *gocheck.C) {
	storage, _ := Open("127.0.0.1", "tsuru_storage_test")
	defer storage.session.Close()
	logs := storage.Logs()
	c.Assert(logs, HasIndex, []string{"date"})
}

func (s *S) TestLogsDateDescendingIndex(c *gocheck.C) {
	storage, _ := Open("127.0.0.1", "tsuru_storage_test")
	defer storage.session.Close()
	logs := storage.Logs()
	c.Assert(logs, HasIndex, []string{"-date"})
}

func (s *S) TestServices(c *gocheck.C) {
	storage, _ := Open("127.0.0.1:27017", "tsuru_storage_test")
	defer storage.session.Close()
	services := storage.Services()
	servicesc := storage.Collection("services")
	c.Assert(services, gocheck.DeepEquals, servicesc)
}

func (s *S) TestServiceInstances(c *gocheck.C) {
	storage, _ := Open("127.0.0.1:27017", "tsuru_storage_test")
	defer storage.session.Close()
	serviceInstances := storage.ServiceInstances()
	serviceInstancesc := storage.Collection("service_instances")
	c.Assert(serviceInstances, gocheck.DeepEquals, serviceInstancesc)
}

func (s *S) TestMethodTeamsShouldReturnTeamsCollection(c *gocheck.C) {
	storage, _ := Open("127.0.0.1:27017", "tsuru_storage_test")
	defer storage.session.Close()
	teams := storage.Teams()
	teamsc := storage.Collection("teams")
	c.Assert(teams, gocheck.DeepEquals, teamsc)
}

func (s *S) TestQuota(c *gocheck.C) {
	storage, _ := Open("127.0.0.1", "tsuru_storage_test")
	defer storage.session.Close()
	quota := storage.Quota()
	quotac := storage.Collection("quota")
	c.Assert(quota, gocheck.DeepEquals, quotac)
}

func (s *S) TestQuotaOwnerIsUnique(c *gocheck.C) {
	storage, _ := Open("127.0.0.1", "tsuru_storage_test")
	defer storage.session.Close()
	quota := storage.Quota()
	c.Assert(quota, HasUniqueIndex, []string{"owner"})
}

func (s *S) TestRetire(c *gocheck.C) {
	defer func() {
		if r := recover(); !c.Failed() && r == nil {
			c.Errorf("Should panic in ping, but did not!")
		}
	}()
	Open("127.0.0.1:27017", "tsuru_storage_test")
	sess := conn["127.0.0.1:27017"]
	sess.used = sess.used.Add(-1 * 2 * period)
	conn["127.0.0.1:27017"] = sess
	var ticker time.Ticker
	ch := make(chan time.Time, 1)
	ticker.C = ch
	ch <- time.Now()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		retire(&ticker)
		wg.Done()
	}()
	close(ch)
	wg.Wait()
	_, ok := conn["127.0.0.1:27017"]
	c.Check(ok, gocheck.Equals, false)
	sess.s.Ping()
}

func (s *S) TestCollectionClose(c *gocheck.C) {
	defer func() {
		if r := recover(); !c.Failed() && r == nil {
			c.Errorf("Should panic in ping, but did not!")
		}
	}()
	storage, _ := Open("127.0.0.1:27017", "tsuru_storage_test")
	coll := Collection{storage.Collection("something").Collection}
	coll.Close()
	storage.session.Ping()
}
