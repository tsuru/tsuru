// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/cezarsa/form"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db/storagev2"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/event/eventtest"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/permission/permissiontest"
	"github.com/tsuru/tsuru/router/routertest"
	_ "github.com/tsuru/tsuru/storage/mongodb"
	appTypes "github.com/tsuru/tsuru/types/app"
	authTypes "github.com/tsuru/tsuru/types/auth"
	bindTypes "github.com/tsuru/tsuru/types/bind"
	eventTypes "github.com/tsuru/tsuru/types/event"
	permTypes "github.com/tsuru/tsuru/types/permission"
	mongoBSON "go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/bsontype"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"golang.org/x/crypto/bcrypt"
	check "gopkg.in/check.v1"
)

type EventSuite struct {
	token auth.Token
	team  *authTypes.Team
	user  *auth.User
}

var _ = check.Suite(&EventSuite{})

func (s *EventSuite) createUserAndTeam(c *check.C) {
	s.user = &auth.User{Email: "whydidifall@thewho.com", Password: "123456"}
	app.AuthScheme = nativeScheme
	_, err := nativeScheme.Create(context.TODO(), s.user)
	c.Assert(err, check.IsNil)
	s.team = &authTypes.Team{Name: "tsuruteam"}
	s.token = userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermApp,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
}

func (s *EventSuite) SetUpSuite(c *check.C) {
	err := config.ReadConfigFile("testdata/config.yaml")
	c.Assert(err, check.IsNil)
	config.Set("log:disable-syslog", true)
	config.Set("database:driver", "mongodb")
	config.Set("database:url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("database:name", "tsuru_events_api_tests")
	config.Set("auth:hash-cost", bcrypt.MinCost)

	storagev2.Reset()
}

func (s *EventSuite) TearDownSuite(c *check.C) {
	storagev2.ClearAllCollections(nil)
}

func (s *EventSuite) SetUpTest(c *check.C) {
	var err error
	routertest.FakeRouter.Reset()
	err = storagev2.ClearAllCollections(nil)
	c.Assert(err, check.IsNil)
	s.createUserAndTeam(c)
}

func (s *EventSuite) generateEventCustomData(appName string) *app.DeployOptions {
	return &app.DeployOptions{
		App: &appTypes.App{
			Name: appName,
			Env: map[string]bindTypes.EnvVar{
				"MY_PASSWORD": {
					Name:   "MY_PASSWORD",
					Public: false,
					Value:  "password123",
				},
			},
			ServiceEnvs: []bindTypes.ServiceEnvVar{
				{
					EnvVar: bindTypes.EnvVar{
						Name:   "MY_IMPORTANT_VAR",
						Public: true,
						Value:  "123",
					},
				},
				{
					EnvVar: bindTypes.EnvVar{
						Name:   "MY_PRECIOUS_VAR",
						Public: false,
						Value:  "123",
					},
				},
			},
		},
	}
}

func (s *EventSuite) insertEvents(target string, kinds []*permTypes.PermissionScheme, c *check.C) ([]*event.Event, error) {
	t, err := eventTypes.GetTargetType(target)
	if err != nil {
		return nil, err
	}
	if len(kinds) == 0 {
		kinds = []*permTypes.PermissionScheme{permission.PermAppDeploy}
	}
	evts := make([]*event.Event, 10)
	for i := 0; i < 10; i++ {
		name := fmt.Sprintf("app-%d", i)
		opts := &event.Opts{
			Target:     eventTypes.Target{Type: t, Value: name},
			Owner:      s.token,
			Kind:       kinds[i%len(kinds)],
			Cancelable: i == 0,
		}
		if t == eventTypes.TargetTypeApp {
			opts.Allowed = event.Allowed(permission.PermAppReadEvents, permission.Context(permTypes.CtxTeam, s.team.Name))
			opts.AllowedCancel = event.Allowed(permission.PermAppUpdateEvents, permission.Context(permTypes.CtxTeam, s.team.Name))
		} else {
			opts.Allowed = event.Allowed(permission.PermApp)
			opts.AllowedCancel = event.Allowed(permission.PermApp)
		}
		evt, err := event.New(context.TODO(), opts)
		c.Assert(err, check.IsNil)
		if i == 1 {
			err = evt.Done(context.TODO(), nil)
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

	collection, err := storagev2.EventsCollection()
	c.Assert(err, check.IsNil)
	_, err = collection.UpdateOne(context.TODO(), mongoBSON.M{}, mongoBSON.M{"$set": mongoBSON.M{"running": true}})
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
	kinds := []*permTypes.PermissionScheme{permission.PermAppCreate, permission.PermAppDeploy}
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
	var result []eventTypes.Kind
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
	uuid := primitive.NewObjectID()

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
	id := primitive.NewObjectID()
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
	_, token := permissiontest.CustomUserWithPermission(c, nativeScheme, "myuser", permTypes.Permission{
		Scheme:  permission.PermAppRead,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	evt, err := event.New(context.TODO(), &event.Opts{
		Target:  eventTypes.Target{Type: eventTypes.TargetTypeApp, Value: "aha"},
		Owner:   s.token,
		Kind:    permission.PermAppDeploy,
		Allowed: event.Allowed(permission.PermAppReadEvents, permission.Context(permTypes.CtxTeam, s.team.Name)),
	})
	c.Assert(err, check.IsNil)
	u := fmt.Sprintf("/events/%s", evt.UniqueID.Hex())
	request, err := http.NewRequest("GET", u, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	if !c.Check(recorder.Code, check.Equals, http.StatusOK) {
		c.Assert(recorder.Body.String(), check.Equals, "")
	}
	var result event.Event
	err = json.Unmarshal(recorder.Body.Bytes(), &result)
	c.Assert(err, check.IsNil)
	c.Assert(result.Kind, check.DeepEquals, evt.Kind)
	c.Assert(result.Target, check.DeepEquals, evt.Target)
}

func (s *EventSuite) TestEventInfoPermissionWithSensitiveData(c *check.C) {
	config.Set("events:suppress-sensitive-envs", true)
	defer config.Unset("events:suppress-sensitive-envs")
	_, token := permissiontest.CustomUserWithPermission(c, nativeScheme, "myuser", permTypes.Permission{
		Scheme:  permission.PermAppRead,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	evt, err := event.New(context.TODO(), &event.Opts{
		Target:     eventTypes.Target{Type: eventTypes.TargetTypeApp, Value: "aha"},
		Owner:      s.token,
		Kind:       permission.PermAppDeploy,
		Allowed:    event.Allowed(permission.PermAppReadEvents, permission.Context(permTypes.CtxTeam, s.team.Name)),
		CustomData: s.generateEventCustomData("aha"),
	})
	c.Assert(err, check.IsNil)
	u := fmt.Sprintf("/events/%s", evt.UniqueID.Hex())
	request, err := http.NewRequest("GET", u, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	if !c.Check(recorder.Code, check.Equals, http.StatusOK) {
		c.Assert(recorder.Body.String(), check.Equals, "")
	}
	var result eventTypes.EventInfo
	err = json.Unmarshal(recorder.Body.Bytes(), &result)
	c.Assert(err, check.IsNil)
	c.Assert(result.Kind, check.DeepEquals, evt.Kind)
	c.Assert(result.Target, check.DeepEquals, evt.Target)

	deployOptions := &app.DeployOptions{}

	mongoRaw := mongoBSON.RawValue{
		Type:  bsontype.Type(result.StartCustomData.Kind),
		Value: result.StartCustomData.Data,
	}

	err = mongoRaw.Unmarshal(deployOptions)
	c.Assert(err, check.IsNil)

	c.Assert(deployOptions.App.Env, check.DeepEquals, map[string]bindTypes.EnvVar{
		"MY_PASSWORD": {
			Name:   "MY_PASSWORD",
			Value:  "*** (private variable)",
			Alias:  "",
			Public: false,
		}})
	c.Assert(deployOptions.App.ServiceEnvs, check.DeepEquals, []bindTypes.ServiceEnvVar{
		{
			EnvVar: bindTypes.EnvVar{
				Name:   "MY_IMPORTANT_VAR",
				Value:  "123",
				Public: true,
			},
		},
		{
			EnvVar: bindTypes.EnvVar{
				Name:  "MY_PRECIOUS_VAR",
				Value: "*** (private variable)",
			},
		},
	})
	request, err = http.NewRequest("GET", "/events", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder = httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	var results []event.Event
	err = json.Unmarshal(recorder.Body.Bytes(), &results)
	c.Assert(err, check.IsNil)
	c.Assert(results, check.HasLen, 1)
	deployOptions = &app.DeployOptions{}

	// On list StartCustomData, EndCustomData and OtherCustomData is empty for performance reasons
	c.Assert(results[0].StartCustomData, check.DeepEquals, mongoBSON.RawValue{})

}

func (s *EventSuite) TestEventInfoWithoutPermission(c *check.C) {
	_, token := permissiontest.CustomUserWithPermission(c, nativeScheme, "myuser", permTypes.Permission{
		Scheme:  permission.PermAppRead,
		Context: permission.Context(permTypes.CtxTeam, "some-other-team"),
	})
	evt, err := event.New(context.TODO(), &event.Opts{
		Target:  eventTypes.Target{Type: eventTypes.TargetTypeApp, Value: "aha"},
		Owner:   s.token,
		Kind:    permission.PermAppDeploy,
		Allowed: event.Allowed(permission.PermAppReadEvents, permission.Context(permTypes.CtxTeam, s.team.Name)),
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
	_, token := permissiontest.CustomUserWithPermission(c, nativeScheme, "myuser", permTypes.Permission{
		Scheme:  permission.PermAppUpdate,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	evt, err := event.New(context.TODO(), &event.Opts{
		Target:        eventTypes.Target{Type: eventTypes.TargetTypeApp, Value: "anything"},
		Owner:         s.token,
		Kind:          permission.PermAppDeploy,
		Cancelable:    true,
		Allowed:       event.Allowed(permission.PermAppReadEvents, permission.Context(permTypes.CtxTeam, s.team.Name)),
		AllowedCancel: event.Allowed(permission.PermAppUpdateEvents, permission.Context(permTypes.CtxTeam, s.team.Name)),
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
	_, token := permissiontest.CustomUserWithPermission(c, nativeScheme, "myuser", permTypes.Permission{
		Scheme:  permission.PermAppRead,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	evt, err := event.New(context.TODO(), &event.Opts{
		Target:        eventTypes.Target{Type: eventTypes.TargetTypeApp, Value: "anything"},
		Owner:         s.token,
		Kind:          permission.PermAppDeploy,
		Cancelable:    true,
		Allowed:       event.Allowed(permission.PermAppReadEvents, permission.Context(permTypes.CtxTeam, s.team.Name)),
		AllowedCancel: event.Allowed(permission.PermAppUpdateEvents, permission.Context(permTypes.CtxTeam, "other-team")),
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
	_, token := permissiontest.CustomUserWithPermission(c, nativeScheme, "myuser", permTypes.Permission{
		Scheme:  permission.PermEventBlockRead,
		Context: permTypes.PermissionContext{CtxType: permTypes.CtxGlobal},
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
	_, token := permissiontest.CustomUserWithPermission(c, nativeScheme, "myuser", permTypes.Permission{
		Scheme:  permission.PermEventBlockRead,
		Context: permTypes.PermissionContext{CtxType: permTypes.CtxGlobal},
	})
	blocks := addBlocks(c)
	err := event.RemoveBlock(context.TODO(), blocks[1].ID)
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
	_, token := permissiontest.CustomUserWithPermission(c, nativeScheme, "myuser", permTypes.Permission{
		Scheme:  permission.PermEventBlockRead,
		Context: permTypes.PermissionContext{CtxType: permTypes.CtxGlobal},
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
	_, token := permissiontest.CustomUserWithPermission(c, nativeScheme, "myuser", permTypes.Permission{
		Scheme:  permission.PermEventBlockAdd,
		Context: permTypes.PermissionContext{CtxType: permTypes.CtxGlobal},
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
	blocks, err := event.ListBlocks(context.TODO(), nil)
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
	blocks, err := event.ListBlocks(context.TODO(), nil)
	c.Assert(err, check.IsNil)
	c.Assert(len(blocks), check.Equals, 0)
}

func (s *EventSuite) TestEventBlockAddWithoutReason(c *check.C) {
	_, token := permissiontest.CustomUserWithPermission(c, nativeScheme, "myuser", permTypes.Permission{
		Scheme:  permission.PermEventBlockAdd,
		Context: permTypes.PermissionContext{CtxType: permTypes.CtxGlobal},
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
	blocks, err := event.ListBlocks(context.TODO(), nil)
	c.Assert(err, check.IsNil)
	c.Assert(len(blocks), check.Equals, 0)
}

func (s *EventSuite) TestEventBlockRemove(c *check.C) {
	_, token := permissiontest.CustomUserWithPermission(c, nativeScheme, "myuser", permTypes.Permission{
		Scheme:  permission.PermEventBlockRemove,
		Context: permTypes.PermissionContext{CtxType: permTypes.CtxGlobal},
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
	afterBlocks, err := event.ListBlocks(context.TODO(), &active)
	c.Assert(err, check.IsNil)
	c.Assert(len(afterBlocks), check.Equals, 1)
	c.Assert(afterBlocks[0].ID.Hex(), check.Equals, blocks[0].ID.Hex())
	c.Assert(eventtest.EventDesc{
		Target: eventTypes.Target{Type: eventTypes.TargetTypeEventBlock, Value: blocks[0].ID.Hex()},
		Owner:  token.GetUserName(),
		Kind:   "event-block.remove",
		StartCustomData: []map[string]interface{}{
			{"name": "ID", "value": blocks[0].ID.Hex()},
		},
	}, eventtest.HasEvent)
}

func (s *EventSuite) TestEventBlockRemoveInvalidUUID(c *check.C) {
	_, token := permissiontest.CustomUserWithPermission(c, nativeScheme, "myuser", permTypes.Permission{
		Scheme:  permission.PermEventBlockRemove,
		Context: permTypes.PermissionContext{CtxType: permTypes.CtxGlobal},
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
	_, token := permissiontest.CustomUserWithPermission(c, nativeScheme, "myuser", permTypes.Permission{
		Scheme:  permission.PermEventBlockRemove,
		Context: permTypes.PermissionContext{CtxType: permTypes.CtxGlobal},
	})
	request, err := http.NewRequest("DELETE", fmt.Sprintf("/events/blocks/%s", primitive.NewObjectID().Hex()), nil)
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
		err := event.AddBlock(context.TODO(), b)
		c.Assert(err, check.IsNil)
	}
	return blocks
}
