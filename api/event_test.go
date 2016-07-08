// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/repository"
	"github.com/tsuru/tsuru/repository/repositorytest"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

type EventSuite struct {
	conn    *db.Storage
	logConn *db.LogStorage
	token   auth.Token
	team    *auth.Team
}

var _ = check.Suite(&EventSuite{})

func (s *EventSuite) createUserAndTeam(c *check.C) {
	user := &auth.User{Email: "whydidifall@thewho.com", Password: "123456"}
	app.AuthScheme = nativeScheme
	_, err := nativeScheme.Create(user)
	c.Assert(err, check.IsNil)
	s.team = &auth.Team{Name: "tsuruteam"}
	err = s.conn.Teams().Insert(s.team)
	c.Assert(err, check.IsNil)
	s.token = userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppReadDeploy,
		Context: permission.Context(permission.CtxTeam, s.team.Name),
	}, permission.Permission{
		Scheme:  permission.PermAppDeploy,
		Context: permission.Context(permission.CtxTeam, s.team.Name),
	})
}

func (s *EventSuite) SetUpSuite(c *check.C) {
	err := config.ReadConfigFile("testdata/config.yaml")
	c.Assert(err, check.IsNil)
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "tsuru_deploy_api_tests")
	config.Set("auth:hash-cost", 4)
	config.Set("repo-manager", "fake")
	s.conn, err = db.Conn()
	c.Assert(err, check.IsNil)
	s.logConn, err = db.LogConn()
	c.Assert(err, check.IsNil)
}

func (s *EventSuite) TearDownSuite(c *check.C) {
	config.Unset("docker:router")
	s.conn.Apps().Database.DropDatabase()
	s.logConn.Logs("myapp").Database.DropDatabase()
	s.conn.Close()
	s.logConn.Close()
}

func (s *EventSuite) SetUpTest(c *check.C) {
	repositorytest.Reset()
	err := dbtest.ClearAllCollections(s.conn.Apps().Database)
	c.Assert(err, check.IsNil)
	s.createUserAndTeam(c)
	s.conn.Platforms().Insert(app.Platform{Name: "python"})
	user, err := s.token.User()
	c.Assert(err, check.IsNil)
	repository.Manager().CreateUser(user.Email)
	config.Set("docker:router", "fake")
}

func (s *EventSuite) insertEvents(target string, c *check.C) []*event.Event {
	evts := make([]*event.Event, 10)
	for i := 0; i < 10; i++ {
		evt, err := event.New(&event.Opts{
			Target: event.Target{Name: target, Value: strconv.Itoa(i)},
			Owner:  s.token,
			Kind:   permission.PermAppDeploy,
		})
		c.Assert(err, check.IsNil)
		evts[i] = evt
	}
	return evts
}

func (s *EventSuite) TestEventListEmpty(c *check.C) {
	request, err := http.NewRequest("GET", "/events", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNoContent)
}

func (s *EventSuite) TestEventList(c *check.C) {
	s.insertEvents("something", c)
	request, err := http.NewRequest("GET", "/events", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	var result []event.Event
	err = json.Unmarshal(recorder.Body.Bytes(), &result)
	c.Assert(err, check.IsNil)
	c.Assert(result, check.HasLen, 10)
}

func (s *EventSuite) TestEventListFilterByTarget(c *check.C) {
	s.insertEvents("something", c)
	s.insertEvents("nothing", c)
	request, err := http.NewRequest("GET", "/events?target=something", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	var result []event.Event
	err = json.Unmarshal(recorder.Body.Bytes(), &result)
	c.Assert(err, check.IsNil)
	c.Assert(result, check.HasLen, 10)
}

func (s *EventSuite) TestEventListFilterRunning(c *check.C) {
	s.insertEvents("something", c)
	err := s.conn.Events().Update(bson.M{}, bson.M{"$set": bson.M{"running": true}})
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("GET", "/events?running=true", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	var result []event.Event
	err = json.Unmarshal(recorder.Body.Bytes(), &result)
	c.Assert(err, check.IsNil)
	c.Assert(result, check.HasLen, 10)
	request, err = http.NewRequest("GET", "/events?running=false", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder = httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNoContent)
}
