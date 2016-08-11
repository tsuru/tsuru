// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/repository/repositorytest"
	"github.com/tsuru/tsuru/router/routertest"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

type EventSuite struct {
	conn    *db.Storage
	logConn *db.LogStorage
	token   auth.Token
	team    *auth.Team
	user    *auth.User
}

var _ = check.Suite(&EventSuite{})

func (s *EventSuite) createUserAndTeam(c *check.C) {
	s.user = &auth.User{Email: "whydidifall@thewho.com", Password: "123456"}
	app.AuthScheme = nativeScheme
	_, err := nativeScheme.Create(s.user)
	c.Assert(err, check.IsNil)
	s.team = &auth.Team{Name: "tsuruteam"}
	err = s.conn.Teams().Insert(s.team)
	c.Assert(err, check.IsNil)
	err = s.conn.Teams().Insert(auth.Team{Name: "other-team"})
	c.Assert(err, check.IsNil)
	s.token = userWithPermission(c, permission.Permission{
		Scheme:  permission.PermApp,
		Context: permission.Context(permission.CtxTeam, s.team.Name),
	})
}

func (s *EventSuite) SetUpSuite(c *check.C) {
	err := config.ReadConfigFile("testdata/config.yaml")
	c.Assert(err, check.IsNil)
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "tsuru_events_api_tests")
	config.Set("auth:hash-cost", 4)
	config.Set("repo-manager", "fake")
	s.conn, err = db.Conn()
	c.Assert(err, check.IsNil)
	s.logConn, err = db.LogConn()
	c.Assert(err, check.IsNil)
}

func (s *EventSuite) TearDownSuite(c *check.C) {
	s.conn.Apps().Database.DropDatabase()
	s.logConn.Logs("myapp").Database.DropDatabase()
	s.conn.Close()
	s.logConn.Close()
}

func (s *EventSuite) SetUpTest(c *check.C) {
	repositorytest.Reset()
	var err error
	routertest.FakeRouter.Reset()
	err = dbtest.ClearAllCollections(s.conn.Apps().Database)
	c.Assert(err, check.IsNil)
	s.createUserAndTeam(c)
	s.conn.Platforms().Insert(app.Platform{Name: "python"})
}

func (s *EventSuite) insertEvents(target string, c *check.C) ([]*event.Event, error) {
	t, err := event.GetTargetType(target)
	if err != nil {
		return nil, err
	}
	evts := make([]*event.Event, 10)
	for i := 0; i < 10; i++ {
		name := fmt.Sprintf("app-%d", i)
		opts := &event.Opts{
			Target:     event.Target{Type: t, Value: name},
			Owner:      s.token,
			Kind:       permission.PermAppDeploy,
			Cancelable: i == 0,
		}
		if t == event.TargetTypeApp {
			opts.Allowed = event.Allowed(permission.PermAppReadEvents, permission.Context(permission.CtxTeam, s.team.Name))
			opts.AllowedCancel = event.Allowed(permission.PermAppUpdateEvents, permission.Context(permission.CtxTeam, s.team.Name))
		} else {
			opts.Allowed = event.Allowed(permission.PermApp)
			opts.AllowedCancel = event.Allowed(permission.PermApp)
		}
		evt, err := event.New(opts)
		c.Assert(err, check.IsNil)
		if i == 1 {
			err = evt.Done(nil)
			c.Assert(err, check.IsNil)
		}
		evts[i] = evt
	}
	return evts, nil
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
	_, err := s.insertEvents("app", c)
	c.Assert(err, check.IsNil)
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
	_, err := s.insertEvents("app", c)
	c.Assert(err, check.IsNil)
	s.insertEvents("node", c)
	request, err := http.NewRequest("GET", "/events?target.type=app", nil)
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
	request, err = http.NewRequest("GET", "/events?target.type=node", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder = httptest.NewRecorder()
	server = RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNoContent)
}

func (s *EventSuite) TestEventListFilterRunning(c *check.C) {
	_, err := s.insertEvents("app", c)
	c.Assert(err, check.IsNil)
	err = s.conn.Events().Update(bson.M{}, bson.M{"$set": bson.M{"running": true}})
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
	c.Assert(result, check.HasLen, 9)
	request, err = http.NewRequest("GET", "/events?running=false", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder = httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	err = json.Unmarshal(recorder.Body.Bytes(), &result)
	c.Assert(err, check.IsNil)
	c.Assert(result, check.HasLen, 1)
}

func (s *EventSuite) TestEventListFilterByKind(c *check.C) {
	_, err := s.insertEvents("app", c)
	c.Assert(err, check.IsNil)
	u := fmt.Sprintf("/events?kindName=%s", permission.PermAppDeploy)
	request, err := http.NewRequest("GET", u, nil)
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
	request, err = http.NewRequest("GET", "/events?kindName=invalid", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder = httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNoContent)
}

func (s *EventSuite) TestKindList(c *check.C) {
	_, err := s.insertEvents("app", c)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("GET", "/events/kinds", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	var result []event.Kind
	err = json.Unmarshal(recorder.Body.Bytes(), &result)
	c.Assert(err, check.IsNil)
	c.Assert(result, check.HasLen, 1)
}

func (s *EventSuite) TestKindListNoContent(c *check.C) {
	request, err := http.NewRequest("GET", "/events/kinds", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNoContent)
}

func (s *EventSuite) TestEventInfoInvalidObjectID(c *check.C) {
	u := fmt.Sprintf("/events/%s", "123")
	request, err := http.NewRequest("GET", u, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
}

func (s *EventSuite) TestEventInfoNotFound(c *check.C) {
	uuid := bson.NewObjectId()
	u := fmt.Sprintf("/events/%s", uuid.Hex())
	request, err := http.NewRequest("GET", u, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
}

func (s *EventSuite) TestEventCancel(c *check.C) {
	events, err := s.insertEvents("app", c)
	c.Assert(err, check.IsNil)
	body := strings.NewReader("reason=we ain't gonna take it")
	u := fmt.Sprintf("/events/%s/cancel", events[0].UniqueID.Hex())
	request, err := http.NewRequest("POST", u, body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNoContent)
}

func (s *EventSuite) TestEventCancelInvalidObjectID(c *check.C) {
	u := fmt.Sprintf("/events/%s/cancel", "123")
	body := strings.NewReader("reason=we ain't gonna take it")
	request, err := http.NewRequest("POST", u, body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Equals, "uuid parameter is not ObjectId: 123\n")
}

func (s *EventSuite) TestEventCancelNotFound(c *check.C) {
	id := bson.NewObjectId()
	u := fmt.Sprintf("/events/%s/cancel", id.Hex())
	body := strings.NewReader("reason=we ain't gonna take it")
	request, err := http.NewRequest("POST", u, body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, "event not found\n")
}

func (s *EventSuite) TestEventCancelNoReason(c *check.C) {
	events, err := s.insertEvents("app", c)
	c.Assert(err, check.IsNil)
	body := strings.NewReader("reason=")
	u := fmt.Sprintf("/events/%s/cancel", events[0].UniqueID.Hex())
	request, err := http.NewRequest("POST", u, body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Equals, "reason is mandatory\n")
}

func (s *EventSuite) TestEventCancelNotCancelable(c *check.C) {
	events, err := s.insertEvents("app", c)
	c.Assert(err, check.IsNil)
	body := strings.NewReader("reason=pretty please")
	u := fmt.Sprintf("/events/%s/cancel", events[1].UniqueID.Hex())
	request, err := http.NewRequest("POST", u, body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Equals, "event is not cancelable\n")
}

func (s *EventSuite) TestEventInfoPermission(c *check.C) {
	token := customUserWithPermission(c, "myuser", permission.Permission{
		Scheme:  permission.PermAppRead,
		Context: permission.Context(permission.CtxTeam, s.team.Name),
	})
	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: "aha"},
		Owner:   s.token,
		Kind:    permission.PermAppDeploy,
		Allowed: event.Allowed(permission.PermAppReadEvents, permission.Context(permission.CtxTeam, s.team.Name)),
	})
	c.Assert(err, check.IsNil)
	u := fmt.Sprintf("/events/%s", evt.UniqueID.Hex())
	request, err := http.NewRequest("GET", u, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var result event.Event
	err = json.Unmarshal(recorder.Body.Bytes(), &result)
	c.Assert(err, check.IsNil)
	c.Assert(result.Kind, check.DeepEquals, evt.Kind)
	c.Assert(result.Target, check.DeepEquals, evt.Target)
}

func (s *EventSuite) TestEventInfoWithoutPermission(c *check.C) {
	token := customUserWithPermission(c, "myuser", permission.Permission{
		Scheme:  permission.PermAppRead,
		Context: permission.Context(permission.CtxTeam, "some-other-team"),
	})
	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: "aha"},
		Owner:   s.token,
		Kind:    permission.PermAppDeploy,
		Allowed: event.Allowed(permission.PermAppReadEvents, permission.Context(permission.CtxTeam, s.team.Name)),
	})
	c.Assert(err, check.IsNil)
	u := fmt.Sprintf("/events/%s", evt.UniqueID.Hex())
	request, err := http.NewRequest("GET", u, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}

func (s *EventSuite) TestEventCancelPermission(c *check.C) {
	token := customUserWithPermission(c, "myuser", permission.Permission{
		Scheme:  permission.PermAppUpdate,
		Context: permission.Context(permission.CtxTeam, s.team.Name),
	})
	evt, err := event.New(&event.Opts{
		Target:        event.Target{Type: event.TargetTypeApp, Value: "anything"},
		Owner:         s.token,
		Kind:          permission.PermAppDeploy,
		Cancelable:    true,
		Allowed:       event.Allowed(permission.PermAppReadEvents, permission.Context(permission.CtxTeam, s.team.Name)),
		AllowedCancel: event.Allowed(permission.PermAppUpdateEvents, permission.Context(permission.CtxTeam, s.team.Name)),
	})
	c.Assert(err, check.IsNil)
	body := strings.NewReader("reason=we ain't gonna take it")
	u := fmt.Sprintf("/events/%s/cancel", evt.UniqueID.Hex())
	request, err := http.NewRequest("POST", u, body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNoContent)
}

func (s *EventSuite) TestEventCancelWithoutPermission(c *check.C) {
	token := customUserWithPermission(c, "myuser", permission.Permission{
		Scheme:  permission.PermAppRead,
		Context: permission.Context(permission.CtxTeam, s.team.Name),
	})
	evt, err := event.New(&event.Opts{
		Target:        event.Target{Type: event.TargetTypeApp, Value: "anything"},
		Owner:         s.token,
		Kind:          permission.PermAppDeploy,
		Cancelable:    true,
		Allowed:       event.Allowed(permission.PermAppReadEvents, permission.Context(permission.CtxTeam, s.team.Name)),
		AllowedCancel: event.Allowed(permission.PermAppUpdateEvents, permission.Context(permission.CtxTeam, "other-team")),
	})
	c.Assert(err, check.IsNil)
	body := strings.NewReader("reason=we ain't gonna take it")
	u := fmt.Sprintf("/events/%s/cancel", evt.UniqueID.Hex())
	request, err := http.NewRequest("POST", u, body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}
