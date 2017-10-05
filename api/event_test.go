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

	"golang.org/x/crypto/bcrypt"

	"github.com/ajg/form"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/event/eventtest"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/permission/permissiontest"
	"github.com/tsuru/tsuru/repository/repositorytest"
	"github.com/tsuru/tsuru/router/routertest"
	_ "github.com/tsuru/tsuru/storage/mongodb"
	appTypes "github.com/tsuru/tsuru/types/app"
	authTypes "github.com/tsuru/tsuru/types/auth"
	serviceTypes "github.com/tsuru/tsuru/types/service"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

type EventSuite struct {
	conn    *db.Storage
	logConn *db.LogStorage
	token   auth.Token
	team    *authTypes.Team
	user    *auth.User
}

var _ = check.Suite(&EventSuite{})

func (s *EventSuite) createUserAndTeam(c *check.C) {
	s.user = &auth.User{Email: "whydidifall@thewho.com", Password: "123456"}
	app.AuthScheme = nativeScheme
	_, err := nativeScheme.Create(s.user)
	c.Assert(err, check.IsNil)
	s.team = &authTypes.Team{Name: "tsuruteam"}
	err = serviceTypes.Team().Insert(*s.team)
	c.Assert(err, check.IsNil)
	err = serviceTypes.Team().Insert(authTypes.Team{Name: "other-team"})
	c.Assert(err, check.IsNil)
	s.token = userWithPermission(c, permission.Permission{
		Scheme:  permission.PermApp,
		Context: permission.Context(permission.CtxTeam, s.team.Name),
	})
}

func (s *EventSuite) SetUpSuite(c *check.C) {
	err := config.ReadConfigFile("testdata/config.yaml")
	c.Assert(err, check.IsNil)
	config.Set("log:disable-syslog", true)
	config.Set("database:driver", "mongodb")
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "tsuru_events_api_tests")
	config.Set("auth:hash-cost", bcrypt.MinCost)
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
	app.PlatformService().Insert(appTypes.Platform{Name: "python"})
}

func (s *EventSuite) insertEvents(target string, kinds []*permission.PermissionScheme, c *check.C) ([]*event.Event, error) {
	t, err := event.GetTargetType(target)
	if err != nil {
		return nil, err
	}
	if len(kinds) == 0 {
		kinds = []*permission.PermissionScheme{permission.PermAppDeploy}
	}
	evts := make([]*event.Event, 10)
	for i := 0; i < 10; i++ {
		name := fmt.Sprintf("app-%d", i)
		opts := &event.Opts{
			Target:     event.Target{Type: t, Value: name},
			Owner:      s.token,
			Kind:       kinds[i%len(kinds)],
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
	_, err := s.insertEvents("app", nil, c)
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
	_, err := s.insertEvents("app", nil, c)
	c.Assert(err, check.IsNil)
	s.insertEvents("node", nil, c)
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
	_, err := s.insertEvents("app", nil, c)
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

func (s *EventSuite) TestEventListFilterByKinds(c *check.C) {
	kinds := []*permission.PermissionScheme{permission.PermAppCreate, permission.PermAppDeploy}
	_, err := s.insertEvents("app", kinds, c)
	c.Assert(err, check.IsNil)

	u := fmt.Sprintf("/events?kindName=%s&kindname=%s", kinds[0].FullName(), kinds[1].FullName())
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

	u = "/events?KindName=" + kinds[0].FullName()
	request, err = http.NewRequest("GET", u, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder = httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	err = json.Unmarshal(recorder.Body.Bytes(), &result)
	c.Assert(err, check.IsNil)
	c.Assert(result, check.HasLen, 5)

	request, err = http.NewRequest("GET", "/events?KINDNAME=invalid", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder = httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNoContent)
}

func (s *EventSuite) TestKindList(c *check.C) {
	_, err := s.insertEvents("app", nil, c)
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
	events, err := s.insertEvents("app", nil, c)
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
	events, err := s.insertEvents("app", nil, c)
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
	events, err := s.insertEvents("app", nil, c)
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
	_, token := permissiontest.CustomUserWithPermission(c, nativeScheme, "myuser", permission.Permission{
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
	_, token := permissiontest.CustomUserWithPermission(c, nativeScheme, "myuser", permission.Permission{
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
	_, token := permissiontest.CustomUserWithPermission(c, nativeScheme, "myuser", permission.Permission{
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
	_, token := permissiontest.CustomUserWithPermission(c, nativeScheme, "myuser", permission.Permission{
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

func (s *EventSuite) TestEventBlockListAllBlocks(c *check.C) {
	_, token := permissiontest.CustomUserWithPermission(c, nativeScheme, "myuser", permission.Permission{
		Scheme:  permission.PermEventBlockRead,
		Context: permission.PermissionContext{CtxType: permission.CtxGlobal},
	})
	expectedBlocks := addBlocks(c)
	request, err := http.NewRequest("GET", "/events/blocks", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var blocks []*event.Block
	err = json.NewDecoder(recorder.Body).Decode(&blocks)
	c.Assert(err, check.IsNil)
	c.Assert(len(blocks), check.Equals, len(expectedBlocks))
}

func (s *EventSuite) TestEventBlockListFiltered(c *check.C) {
	_, token := permissiontest.CustomUserWithPermission(c, nativeScheme, "myuser", permission.Permission{
		Scheme:  permission.PermEventBlockRead,
		Context: permission.PermissionContext{CtxType: permission.CtxGlobal},
	})
	blocks := addBlocks(c)
	err := event.RemoveBlock(blocks[1].ID)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("GET", "/events/blocks?active=true", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	err = json.NewDecoder(recorder.Body).Decode(&blocks)
	c.Assert(err, check.IsNil)
	c.Assert(len(blocks), check.Equals, 2)
	c.Assert(blocks[0].Active, check.Equals, true)
	c.Assert(blocks[1].Active, check.Equals, true)
}

func (s *EventSuite) TestEventBlockListWithoutPermission(c *check.C) {
	request, err := http.NewRequest("GET", "/events/blocks", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}

func (s *EventSuite) TestEventBlockListEmpty(c *check.C) {
	_, token := permissiontest.CustomUserWithPermission(c, nativeScheme, "myuser", permission.Permission{
		Scheme:  permission.PermEventBlockRead,
		Context: permission.PermissionContext{CtxType: permission.CtxGlobal},
	})
	request, err := http.NewRequest("GET", "/events/blocks?active=true", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNoContent)
}

func (s *EventSuite) TestEventBlockAdd(c *check.C) {
	_, token := permissiontest.CustomUserWithPermission(c, nativeScheme, "myuser", permission.Permission{
		Scheme:  permission.PermEventBlockAdd,
		Context: permission.PermissionContext{CtxType: permission.CtxGlobal},
	})
	block := &event.Block{KindName: "app.deploy", Reason: "block reason"}
	values, err := form.EncodeToValues(block)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("POST", "/events/blocks", strings.NewReader(values.Encode()))
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	blocks, err := event.ListBlocks(nil)
	c.Assert(err, check.IsNil)
	c.Assert(len(blocks), check.Equals, 1)
	c.Assert(blocks[0].Active, check.Equals, true)
	c.Assert(blocks[0].KindName, check.Equals, "app.deploy")
	c.Assert(blocks[0].Reason, check.Equals, "block reason")
}

func (s *EventSuite) TestEventBlockAddWithoutPermission(c *check.C) {
	block := &event.Block{KindName: "app.deploy", Reason: "block reason"}
	values, err := form.EncodeToValues(block)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("POST", "/events/blocks", strings.NewReader(values.Encode()))
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
	blocks, err := event.ListBlocks(nil)
	c.Assert(err, check.IsNil)
	c.Assert(len(blocks), check.Equals, 0)
}

func (s *EventSuite) TestEventBlockAddWithoutReason(c *check.C) {
	_, token := permissiontest.CustomUserWithPermission(c, nativeScheme, "myuser", permission.Permission{
		Scheme:  permission.PermEventBlockAdd,
		Context: permission.PermissionContext{CtxType: permission.CtxGlobal},
	})
	block := &event.Block{KindName: "app.deploy"}
	values, err := form.EncodeToValues(block)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("POST", "/events/blocks", strings.NewReader(values.Encode()))
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Equals, "reason is required\n")
	blocks, err := event.ListBlocks(nil)
	c.Assert(err, check.IsNil)
	c.Assert(len(blocks), check.Equals, 0)
}

func (s *EventSuite) TestEventBlockRemove(c *check.C) {
	_, token := permissiontest.CustomUserWithPermission(c, nativeScheme, "myuser", permission.Permission{
		Scheme:  permission.PermEventBlockRemove,
		Context: permission.PermissionContext{CtxType: permission.CtxGlobal},
	})
	blocks := addBlocks(c)
	request, err := http.NewRequest("DELETE", fmt.Sprintf("/events/blocks/%s", blocks[0].ID.Hex()), nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	active := false
	afterBlocks, err := event.ListBlocks(&active)
	c.Assert(err, check.IsNil)
	c.Assert(len(afterBlocks), check.Equals, 1)
	c.Assert(afterBlocks[0].ID.Hex(), check.Equals, blocks[0].ID.Hex())
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: event.TargetTypeEventBlock, Value: blocks[0].ID.Hex()},
		Owner:  token.GetUserName(),
		Kind:   "event-block.remove",
		StartCustomData: []map[string]interface{}{
			{"name": "ID", "value": blocks[0].ID.Hex()},
		},
	}, eventtest.HasEvent)
}

func (s *EventSuite) TestEventBlockRemoveInvalidUUID(c *check.C) {
	_, token := permissiontest.CustomUserWithPermission(c, nativeScheme, "myuser", permission.Permission{
		Scheme:  permission.PermEventBlockRemove,
		Context: permission.PermissionContext{CtxType: permission.CtxGlobal},
	})
	request, err := http.NewRequest("DELETE", "/events/blocks/abc", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Equals, "uuid parameter is not ObjectId: abc\n")
}

func (s *EventSuite) TestEventBlockRemoveUUIDNotFound(c *check.C) {
	_, token := permissiontest.CustomUserWithPermission(c, nativeScheme, "myuser", permission.Permission{
		Scheme:  permission.PermEventBlockRemove,
		Context: permission.PermissionContext{CtxType: permission.CtxGlobal},
	})
	request, err := http.NewRequest("DELETE", fmt.Sprintf("/events/blocks/%s", bson.NewObjectId().Hex()), nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
}

func (s *EventSuite) TestEventBlockRemoveWithoutPermission(c *check.C) {
	blocks := addBlocks(c)
	request, err := http.NewRequest("DELETE", fmt.Sprintf("/events/blocks/%s", blocks[0].ID.Hex()), nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}

func addBlocks(c *check.C) []*event.Block {
	blocks := []*event.Block{
		{KindName: "app.deploy"},
		{KindName: "app.create"},
		{OwnerName: "blocked-user"},
	}
	for _, b := range blocks {
		err := event.AddBlock(b)
		c.Assert(err, check.IsNil)
	}
	return blocks
}
