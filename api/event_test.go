// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/iaas"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/repository"
	"github.com/tsuru/tsuru/repository/repositorytest"
	"github.com/tsuru/tsuru/router/routertest"
	"github.com/tsuru/tsuru/service"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

type EventSuite struct {
	conn        *db.Storage
	logConn     *db.LogStorage
	token       auth.Token
	team        *auth.Team
	user        *auth.User
	provisioner *provisiontest.FakeProvisioner
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
	config.Set("database:name", "tsuru_deploy_api_tests")
	config.Set("auth:hash-cost", 4)
	config.Set("repo-manager", "fake")
	s.conn, err = db.Conn()
	c.Assert(err, check.IsNil)
	s.logConn, err = db.LogConn()
	c.Assert(err, check.IsNil)
}

func (s *EventSuite) TearDownTest(c *check.C) {
	s.provisioner.Reset()
}

func (s *EventSuite) TearDownSuite(c *check.C) {
	config.Unset("docker:router")
	s.conn.Apps().Database.DropDatabase()
	s.logConn.Logs("myapp").Database.DropDatabase()
	s.provisioner.Reset()
	s.conn.Close()
	s.logConn.Close()
}

func (s *EventSuite) SetUpTest(c *check.C) {
	routertest.FakeRouter.Reset()
	s.provisioner = provisiontest.NewFakeProvisioner()
	app.Provisioner = s.provisioner
	repositorytest.Reset()
	err := dbtest.ClearAllCollections(s.conn.Apps().Database)
	c.Assert(err, check.IsNil)
	s.createUserAndTeam(c)
	s.conn.Platforms().Insert(app.Platform{Name: "python"})
	user, err := s.token.User()
	c.Assert(err, check.IsNil)
	repository.Manager().CreateUser(user.Email)
	config.Set("docker:router", "fake")
	opts := provision.AddPoolOptions{Name: "test1", Default: true}
	err = provision.AddPool(opts)
	c.Assert(err, check.IsNil)
}

func (s *EventSuite) insertEvents(target string, c *check.C) ([]*event.Event, error) {
	t, err := event.GetTargetType(target)
	if err != nil {
		return nil, err
	}
	evts := make([]*event.Event, 10)
	for i := 0; i < 10; i++ {
		name := fmt.Sprintf("app-%d", i)
		if t == event.TargetTypeApp {
			a := app.App{Name: name, Platform: "whitespace", TeamOwner: s.team.Name}
			err = app.CreateApp(&a, s.user)
			c.Assert(err, check.IsNil)
		}
		evt, err := event.New(&event.Opts{
			Target:     event.Target{Type: t, Value: name},
			Owner:      s.token,
			Kind:       permission.PermAppDeploy,
			Cancelable: i == 0,
		})
		c.Assert(err, check.IsNil)
		if i == 1 {
			err = evt.Done(nil)
			c.Assert(err, check.IsNil)
		}
		evts[i] = evt
	}
	return evts, nil
}

func runPermTest(checker evtPermChecker, c *check.C) func(auth.Token, *event.TargetFilter) {
	return func(t auth.Token, expected *event.TargetFilter) {
		filter, err := checker.filter(t)
		c.Assert(err, check.IsNil)
		if filter != nil {
			sort.Strings(filter.Values)
		}
		if expected != nil {
			sort.Strings(expected.Values)
		}
		c.Assert(filter, check.DeepEquals, expected)
	}
}

func (s *EventSuite) TestAppPermCheckerFilter(c *check.C) {
	runTest := runPermTest(&appPermChecker{}, c)
	err := provision.AddPool(provision.AddPoolOptions{Name: "pool1", Default: false, Public: true})
	c.Assert(err, check.IsNil)
	a := app.App{Name: "myapp", Platform: "whitespace", Pool: "pool1", TeamOwner: s.team.Name}
	err = app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	runTest(customUserWithPermission(c, "usera", permission.Permission{
		Scheme:  permission.PermAppReadEvents,
		Context: permission.Context(permission.CtxTeam, s.team.Name),
	}), &event.TargetFilter{
		Type:   event.TargetTypeApp,
		Values: []string{"myapp"},
	})
	runTest(customUserWithPermission(c, "userb", permission.Permission{
		Scheme:  permission.PermAppReadEvents,
		Context: permission.Context(permission.CtxApp, a.Name),
	}), &event.TargetFilter{
		Type:   event.TargetTypeApp,
		Values: []string{"myapp"},
	})
	runTest(customUserWithPermission(c, "userc", permission.Permission{
		Scheme:  permission.PermAppReadEvents,
		Context: permission.Context(permission.CtxGlobal, ""),
	}), &event.TargetFilter{
		Type:   event.TargetTypeApp,
		Values: []string{"myapp"},
	})
	runTest(customUserWithPermission(c, "userd", permission.Permission{
		Scheme:  permission.PermAppReadEvents,
		Context: permission.Context(permission.CtxPool, a.Pool),
	}), &event.TargetFilter{
		Type:   event.TargetTypeApp,
		Values: []string{"myapp"},
	})
	runTest(customUserWithPermission(c, "usere", permission.Permission{
		Scheme:  permission.PermAppReadEvents,
		Context: permission.Context(permission.CtxTeam, "other-team"),
	}), nil)
}

func (s *EventSuite) TestTeamPermCheckerFilter(c *check.C) {
	runTest := runPermTest(&teamPermChecker{}, c)
	runTest(customUserWithPermission(c, "usera", permission.Permission{
		Scheme:  permission.PermTeamReadEvents,
		Context: permission.Context(permission.CtxTeam, "my-team"),
	}), &event.TargetFilter{
		Type:   event.TargetTypeTeam,
		Values: []string{"my-team"},
	})
	runTest(customUserWithPermission(c, "userb", permission.Permission{
		Scheme:  permission.PermTeamReadEvents,
		Context: permission.Context(permission.CtxTeam, "my-team1"),
	}, permission.Permission{
		Scheme:  permission.PermTeamReadEvents,
		Context: permission.Context(permission.CtxTeam, "my-team2"),
	}), &event.TargetFilter{
		Type:   event.TargetTypeTeam,
		Values: []string{"my-team1", "my-team2"},
	})
	runTest(customUserWithPermission(c, "userc", permission.Permission{
		Scheme:  permission.PermTeamReadEvents,
		Context: permission.Context(permission.CtxTeam, "my-team1"),
	}, permission.Permission{
		Scheme:  permission.PermTeamReadEvents,
		Context: permission.Context(permission.CtxGlobal, ""),
	}), &event.TargetFilter{
		Type:   event.TargetTypeTeam,
		Values: nil,
	})
	runTest(customUserWithPermission(c, "userd"), nil)
}

func (s *EventSuite) TestServicePermCheckerFilter(c *check.C) {
	runTest := runPermTest(&servicePermChecker{}, c)
	srv := service.Service{Name: "mongodb", OwnerTeams: []string{s.team.Name}}
	err := srv.Create()
	c.Assert(err, check.IsNil)
	runTest(customUserWithPermission(c, "usera", permission.Permission{
		Scheme:  permission.PermServiceReadEvents,
		Context: permission.Context(permission.CtxTeam, s.team.Name),
	}), &event.TargetFilter{
		Type:   event.TargetTypeService,
		Values: []string{srv.Name},
	})
	runTest(customUserWithPermission(c, "userb", permission.Permission{
		Scheme:  permission.PermServiceReadEvents,
		Context: permission.Context(permission.CtxService, srv.Name),
	}), &event.TargetFilter{
		Type:   event.TargetTypeService,
		Values: []string{srv.Name},
	})
	runTest(customUserWithPermission(c, "userc", permission.Permission{
		Scheme:  permission.PermServiceReadEvents,
		Context: permission.Context(permission.CtxGlobal, ""),
	}), &event.TargetFilter{
		Type:   event.TargetTypeService,
		Values: []string{srv.Name},
	})
	runTest(customUserWithPermission(c, "userd", permission.Permission{
		Scheme:  permission.PermServiceReadEvents,
		Context: permission.Context(permission.CtxTeam, "some-other-team"),
	}), nil)
}

func (s *EventSuite) TestServiceInstancePermCheckerFilter(c *check.C) {
	runTest := runPermTest(&serviceInstancePermChecker{}, c)
	srv := service.Service{Name: "mongodb", OwnerTeams: []string{"otherteam"}}
	err := srv.Create()
	c.Assert(err, check.IsNil)
	si := service.ServiceInstance{
		Name:        "nodata",
		ServiceName: "mongodb",
		Description: "desc",
		Teams:       []string{s.team.Name},
	}
	err = si.Create()
	c.Assert(err, check.IsNil)
	runTest(customUserWithPermission(c, "usera", permission.Permission{
		Scheme:  permission.PermServiceInstanceReadEvents,
		Context: permission.Context(permission.CtxTeam, s.team.Name),
	}), &event.TargetFilter{
		Type:   event.TargetTypeServiceInstance,
		Values: []string{serviceIntancePermName(srv.Name, si.Name)},
	})
	runTest(customUserWithPermission(c, "userb", permission.Permission{
		Scheme:  permission.PermServiceInstanceReadEvents,
		Context: permission.Context(permission.CtxServiceInstance, serviceIntancePermName(srv.Name, si.Name)),
	}), &event.TargetFilter{
		Type:   event.TargetTypeServiceInstance,
		Values: []string{serviceIntancePermName(srv.Name, si.Name)},
	})
	runTest(customUserWithPermission(c, "userc", permission.Permission{
		Scheme:  permission.PermServiceInstanceReadEvents,
		Context: permission.Context(permission.CtxGlobal, ""),
	}), &event.TargetFilter{
		Type:   event.TargetTypeServiceInstance,
		Values: []string{serviceIntancePermName(srv.Name, si.Name)},
	})
	runTest(customUserWithPermission(c, "userd", permission.Permission{
		Scheme:  permission.PermServiceInstanceReadEvents,
		Context: permission.Context(permission.CtxTeam, "some-other-team"),
	}), nil)
}

func (s *EventSuite) TestPoolPermCheckerFilter(c *check.C) {
	runTest := runPermTest(&poolPermChecker{}, c)
	runTest(customUserWithPermission(c, "usera", permission.Permission{
		Scheme:  permission.PermPoolReadEvents,
		Context: permission.Context(permission.CtxPool, "my-pool"),
	}), &event.TargetFilter{
		Type:   event.TargetTypePool,
		Values: []string{"my-pool"},
	})
	runTest(customUserWithPermission(c, "userb", permission.Permission{
		Scheme:  permission.PermPoolReadEvents,
		Context: permission.Context(permission.CtxPool, "my-pool1"),
	}, permission.Permission{
		Scheme:  permission.PermPoolReadEvents,
		Context: permission.Context(permission.CtxPool, "my-pool2"),
	}), &event.TargetFilter{
		Type:   event.TargetTypePool,
		Values: []string{"my-pool1", "my-pool2"},
	})
	runTest(customUserWithPermission(c, "userc", permission.Permission{
		Scheme:  permission.PermPoolReadEvents,
		Context: permission.Context(permission.CtxPool, "my-pool1"),
	}, permission.Permission{
		Scheme:  permission.PermPoolReadEvents,
		Context: permission.Context(permission.CtxGlobal, ""),
	}), &event.TargetFilter{
		Type:   event.TargetTypePool,
		Values: nil,
	})
	runTest(customUserWithPermission(c, "userd"), nil)
}

func (s *EventSuite) TestUserPermCheckerFilter(c *check.C) {
	runTest := runPermTest(&userPermChecker{}, c)
	runTest(customUserWithPermission(c, "usera"), &event.TargetFilter{
		Type:   event.TargetTypeUser,
		Values: []string{"usera@groundcontrol.com"},
	})
	runTest(customUserWithPermission(c, "userb", permission.Permission{
		Scheme:  permission.PermUserReadEvents,
		Context: permission.Context(permission.CtxGlobal, ""),
	}), &event.TargetFilter{
		Type:   event.TargetTypeUser,
		Values: nil,
	})
}

func (s *EventSuite) TestContainerPermCheckerFilter(c *check.C) {
	runTest := runPermTest(&containerPermChecker{}, c)
	err := provision.AddPool(provision.AddPoolOptions{Name: "pool1", Default: false, Public: true})
	c.Assert(err, check.IsNil)
	a := app.App{Name: "myapp", Platform: "whitespace", Pool: "pool1", TeamOwner: s.team.Name}
	err = app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	_, err = s.provisioner.AddUnits(&a, 3, "web", nil)
	c.Assert(err, check.IsNil)
	a2 := app.App{Name: "otherapp", Platform: "whitespace", Pool: "pool1", TeamOwner: "other-team"}
	err = app.CreateApp(&a2, s.user)
	c.Assert(err, check.IsNil)
	_, err = s.provisioner.AddUnits(&a2, 3, "web", nil)
	c.Assert(err, check.IsNil)
	runTest(customUserWithPermission(c, "usera", permission.Permission{
		Scheme:  permission.PermAppReadEvents,
		Context: permission.Context(permission.CtxTeam, s.team.Name),
	}), &event.TargetFilter{
		Type:   event.TargetTypeContainer,
		Values: []string{"myapp-0", "myapp-1", "myapp-2"},
	})
	runTest(customUserWithPermission(c, "userb", permission.Permission{
		Scheme:  permission.PermAppReadEvents,
		Context: permission.Context(permission.CtxApp, a.Name),
	}), &event.TargetFilter{
		Type:   event.TargetTypeContainer,
		Values: []string{"myapp-0", "myapp-1", "myapp-2"},
	})
	runTest(customUserWithPermission(c, "userc", permission.Permission{
		Scheme:  permission.PermAppReadEvents,
		Context: permission.Context(permission.CtxGlobal, ""),
	}), &event.TargetFilter{
		Type:   event.TargetTypeContainer,
		Values: []string{"myapp-0", "myapp-1", "myapp-2", "otherapp-0", "otherapp-1", "otherapp-2"},
	})
	runTest(customUserWithPermission(c, "userd", permission.Permission{
		Scheme:  permission.PermAppReadEvents,
		Context: permission.Context(permission.CtxPool, a.Pool),
	}), &event.TargetFilter{
		Type:   event.TargetTypeContainer,
		Values: []string{"myapp-0", "myapp-1", "myapp-2", "otherapp-0", "otherapp-1", "otherapp-2"},
	})
	runTest(customUserWithPermission(c, "usere", permission.Permission{
		Scheme:  permission.PermAppReadEvents,
		Context: permission.Context(permission.CtxTeam, "yet-another-team"),
	}), nil)
}

func (s *EventSuite) TestNodePermCheckerFilter(c *check.C) {
	runTest := runPermTest(&nodePermChecker{}, c)
	s.provisioner.AddNode("n1", "p1")
	s.provisioner.AddNode("n2", "p1")
	s.provisioner.AddNode("n3", "p2")
	runTest(customUserWithPermission(c, "usera", permission.Permission{
		Scheme:  permission.PermPoolReadEvents,
		Context: permission.Context(permission.CtxPool, "p1"),
	}), &event.TargetFilter{
		Type:   event.TargetTypeNode,
		Values: []string{"n1", "n2"},
	})
	runTest(customUserWithPermission(c, "userb", permission.Permission{
		Scheme:  permission.PermPoolReadEvents,
		Context: permission.Context(permission.CtxPool, "p1"),
	}, permission.Permission{
		Scheme:  permission.PermPoolReadEvents,
		Context: permission.Context(permission.CtxPool, "p2"),
	}), &event.TargetFilter{
		Type:   event.TargetTypeNode,
		Values: []string{"n1", "n2", "n3"},
	})
	runTest(customUserWithPermission(c, "userc", permission.Permission{
		Scheme:  permission.PermPoolReadEvents,
		Context: permission.Context(permission.CtxPool, "p1"),
	}, permission.Permission{
		Scheme:  permission.PermPoolReadEvents,
		Context: permission.Context(permission.CtxGlobal, ""),
	}), &event.TargetFilter{
		Type:   event.TargetTypeNode,
		Values: nil,
	})
	runTest(customUserWithPermission(c, "userd"), nil)
}

func (s *EventSuite) TestRolePermCheckerFilter(c *check.C) {
	runTest := runPermTest(&rolePermChecker{}, c)
	runTest(customUserWithPermission(c, "usera"), nil)
	runTest(customUserWithPermission(c, "userb", permission.Permission{
		Scheme:  permission.PermRoleReadEvents,
		Context: permission.Context(permission.CtxGlobal, ""),
	}), &event.TargetFilter{
		Type:   event.TargetTypeRole,
		Values: nil,
	})
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

func (s *EventSuite) TestEventInfoAppPermission(c *check.C) {
	token := customUserWithPermission(c, "myuser", permission.Permission{
		Scheme:  permission.PermAppRead,
		Context: permission.Context(permission.CtxTeam, s.team.Name),
	})
	a := app.App{Name: "new-app", Platform: "zend", TeamOwner: s.team.Name}
	usr, err := token.User()
	c.Assert(err, check.IsNil)
	err = app.CreateApp(&a, usr)
	c.Assert(err, check.IsNil)
	evt, err := event.New(&event.Opts{
		Target: event.Target{Type: event.TargetTypeApp, Value: a.Name},
		Owner:  s.token,
		Kind:   permission.PermAppDeploy,
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

func (s *EventSuite) TestEventInfoAppWithoutPermission(c *check.C) {
	token := customUserWithPermission(c, "myuser", permission.Permission{
		Scheme:  permission.PermAppDeploy,
		Context: permission.Context(permission.CtxTeam, s.team.Name),
	})
	a := app.App{Name: "new-app2", Platform: "zend", TeamOwner: s.team.Name}
	usr, err := token.User()
	c.Assert(err, check.IsNil)
	err = app.CreateApp(&a, usr)
	c.Assert(err, check.IsNil)
	evt, err := event.New(&event.Opts{
		Target: event.Target{Type: event.TargetTypeApp, Value: a.Name},
		Owner:  s.token,
		Kind:   permission.PermAppDeploy,
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

func (s *EventSuite) TestEventInfoTeamPermission(c *check.C) {
	token := customUserWithPermission(c, "myuser", permission.Permission{
		Scheme:  permission.PermTeamRead,
		Context: permission.Context(permission.CtxTeam, s.team.Name),
	})
	evt, err := event.New(&event.Opts{
		Target: event.Target{Type: event.TargetTypeTeam, Value: s.team.Name},
		Owner:  s.token,
		Kind:   permission.PermAppDeploy,
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

func (s *EventSuite) TestEventInfoTeamWithoutPermission(c *check.C) {
	token := customUserWithPermission(c, "myuser", permission.Permission{
		Scheme:  permission.PermAppDeploy,
		Context: permission.Context(permission.CtxTeam, s.team.Name),
	})
	evt, err := event.New(&event.Opts{
		Target: event.Target{Type: event.TargetTypeTeam, Value: s.team.Name},
		Owner:  s.token,
		Kind:   permission.PermAppDeploy,
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

func (s *EventSuite) TestEventInfoServicePermission(c *check.C) {
	token := customUserWithPermission(c, "myuser", permission.Permission{
		Scheme:  permission.PermServiceRead,
		Context: permission.Context(permission.CtxTeam, s.team.Name),
	})
	srv := service.Service{
		Name:       "myservice",
		OwnerTeams: []string{s.team.Name},
		Username:   "myuser",
	}
	err := srv.Create()
	c.Assert(err, check.IsNil)
	evt, err := event.New(&event.Opts{
		Target: event.Target{Type: event.TargetTypeService, Value: srv.Name},
		Owner:  s.token,
		Kind:   permission.PermAppDeploy,
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

func (s *EventSuite) TestEventInfoServiceWithoutPermission(c *check.C) {
	token := customUserWithPermission(c, "myuser", permission.Permission{
		Scheme:  permission.PermAppDeploy,
		Context: permission.Context(permission.CtxTeam, s.team.Name),
	})
	srv := service.Service{Name: "myservice", OwnerTeams: []string{s.team.Name}}
	err := srv.Create()
	c.Assert(err, check.IsNil)
	evt, err := event.New(&event.Opts{
		Target: event.Target{Type: event.TargetTypeService, Value: srv.Name},
		Owner:  s.token,
		Kind:   permission.PermAppDeploy,
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

func (s *EventSuite) TestEventInfoServiceInstancePermission(c *check.C) {
	token := customUserWithPermission(c, "myuser", permission.Permission{
		Scheme:  permission.PermServiceInstanceRead,
		Context: permission.Context(permission.CtxTeam, s.team.Name),
	})
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	se := service.Service{Name: "foo", Endpoint: map[string]string{"production": ts.URL}}
	err := se.Create()
	defer s.conn.Services().Remove(bson.M{"_id": se.Name})
	c.Assert(err, check.IsNil)
	si := service.ServiceInstance{Name: "foo-instance", ServiceName: "foo", Teams: []string{s.team.Name}}
	err = si.Create()
	c.Assert(err, check.IsNil)
	evt, err := event.New(&event.Opts{
		Target: event.Target{Type: event.TargetTypeServiceInstance, Value: "foo/foo-instance"},
		Owner:  s.token,
		Kind:   permission.PermAppDeploy,
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

func (s *EventSuite) TestEventInfoServiceInstanceWithoutPermission(c *check.C) {
	token := customUserWithPermission(c, "myuser", permission.Permission{
		Scheme:  permission.PermAppDeploy,
		Context: permission.Context(permission.CtxTeam, s.team.Name),
	})
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	se := service.Service{Name: "foo", Endpoint: map[string]string{"production": ts.URL}}
	err := se.Create()
	defer s.conn.Services().Remove(bson.M{"_id": se.Name})
	c.Assert(err, check.IsNil)
	si := service.ServiceInstance{Name: "foo-instance", ServiceName: "foo", Teams: []string{s.team.Name}}
	err = si.Create()
	c.Assert(err, check.IsNil)
	evt, err := event.New(&event.Opts{
		Target: event.Target{Type: event.TargetTypeServiceInstance, Value: "foo_foo-instance"},
		Owner:  s.token,
		Kind:   permission.PermAppDeploy,
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

func (s *EventSuite) TestEventInfoServiceInstanceInvalidTargetValue(c *check.C) {
	token := customUserWithPermission(c, "myuser", permission.Permission{
		Scheme:  permission.PermAppDeploy,
		Context: permission.Context(permission.CtxTeam, s.team.Name),
	})
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	se := service.Service{Name: "foo", Endpoint: map[string]string{"production": ts.URL}}
	err := se.Create()
	defer s.conn.Services().Remove(bson.M{"_id": se.Name})
	c.Assert(err, check.IsNil)
	si := service.ServiceInstance{Name: "foo-instance", ServiceName: "foo", Teams: []string{s.team.Name}}
	err = si.Create()
	c.Assert(err, check.IsNil)
	evt, err := event.New(&event.Opts{
		Target: event.Target{Type: event.TargetTypeServiceInstance, Value: "foofoo-instance"},
		Owner:  s.token,
		Kind:   permission.PermAppDeploy,
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

func (s *EventSuite) TestEventInfoPoolPermission(c *check.C) {
	token := customUserWithPermission(c, "myuser", permission.Permission{
		Scheme:  permission.PermPoolRead,
		Context: permission.Context(permission.CtxPool, "test1"),
	})
	evt, err := event.New(&event.Opts{
		Target: event.Target{Type: event.TargetTypePool, Value: "test1"},
		Owner:  s.token,
		Kind:   permission.PermAppDeploy,
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

func (s *EventSuite) TestEventInfoPoolWithoutPermission(c *check.C) {
	token := customUserWithPermission(c, "myuser", permission.Permission{
		Scheme:  permission.PermAppDeploy,
		Context: permission.Context(permission.CtxTeam, s.team.Name),
	})
	evt, err := event.New(&event.Opts{
		Target: event.Target{Type: event.TargetTypePool, Value: "test1"},
		Owner:  s.token,
		Kind:   permission.PermAppDeploy,
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

func (s *EventSuite) TestEventInfoUserPermission(c *check.C) {
	token := customUserWithPermission(c, "myuser", permission.Permission{
		Scheme:  permission.PermUserRead,
		Context: permission.Context(permission.CtxGlobal, ""),
	})
	evt, err := event.New(&event.Opts{
		Target: event.Target{Type: event.TargetTypeUser, Value: token.GetUserName()},
		Owner:  s.token,
		Kind:   permission.PermAppDeploy,
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

func (s *EventSuite) TestEventInfoUserWithoutPermission(c *check.C) {
	token := customUserWithPermission(c, "myuser", permission.Permission{
		Scheme:  permission.PermAppDeploy,
		Context: permission.Context(permission.CtxTeam, s.team.Name),
	})
	evt, err := event.New(&event.Opts{
		Target: event.Target{Type: event.TargetTypeUser, Value: token.GetUserName()},
		Owner:  s.token,
		Kind:   permission.PermAppDeploy,
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

func (s *EventSuite) TestEventInfoContainerPermission(c *check.C) {
	token := customUserWithPermission(c, "myuser", permission.Permission{
		Scheme:  permission.PermAppRead,
		Context: permission.Context(permission.CtxTeam, s.team.Name),
	})
	usr, err := token.User()
	c.Assert(err, check.IsNil)
	a := app.App{Name: "velha", Platform: "zend", TeamOwner: s.team.Name}
	err = app.CreateApp(&a, usr)
	c.Assert(err, check.IsNil)
	s.provisioner.AddUnits(&a, 3, "web", nil)
	units, err := a.Units()
	c.Assert(err, check.IsNil)
	cnt := units[0]
	evt, err := event.New(&event.Opts{
		Target: event.Target{Type: event.TargetTypeContainer, Value: cnt.ID},
		Owner:  s.token,
		Kind:   permission.PermAppDeploy,
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

func (s *EventSuite) TestEventInfoContainerWithoutPermission(c *check.C) {
	token := customUserWithPermission(c, "myuser", permission.Permission{
		Scheme:  permission.PermAppDeploy,
		Context: permission.Context(permission.CtxTeam, s.team.Name),
	})
	usr, err := token.User()
	c.Assert(err, check.IsNil)
	a := app.App{Name: "velha2", Platform: "zend", TeamOwner: s.team.Name}
	err = app.CreateApp(&a, usr)
	c.Assert(err, check.IsNil)
	s.provisioner.AddUnits(&a, 3, "web", nil)
	units, err := a.Units()
	c.Assert(err, check.IsNil)
	cnt := units[0]
	evt, err := event.New(&event.Opts{
		Target: event.Target{Type: event.TargetTypeContainer, Value: cnt.ID},
		Owner:  s.token,
		Kind:   permission.PermAppDeploy,
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

func (s *EventSuite) TestEventInfoNodePermission(c *check.C) {
	token := customUserWithPermission(c, "myuser", permission.Permission{
		Scheme:  permission.PermPoolRead,
		Context: permission.Context(permission.CtxPool, "test1"),
	})
	s.provisioner.AddNode("mynode", "test1")
	evt, err := event.New(&event.Opts{
		Target: event.Target{Type: event.TargetTypeNode, Value: "mynode"},
		Owner:  s.token,
		Kind:   permission.PermAppDeploy,
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

func (s *EventSuite) TestEventInfoNodeWithoutPermission(c *check.C) {
	token := customUserWithPermission(c, "myuser", permission.Permission{
		Scheme:  permission.PermAppDeploy,
		Context: permission.Context(permission.CtxTeam, s.team.Name),
	})
	s.provisioner.AddNode("mynode", "test1")
	evt, err := event.New(&event.Opts{
		Target: event.Target{Type: event.TargetTypeNode, Value: "mynode"},
		Owner:  s.token,
		Kind:   permission.PermAppDeploy,
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

func (s *EventSuite) TestEventInfoIassPermission(c *check.C) {
	token := customUserWithPermission(c, "myuser", permission.Permission{
		Scheme:  permission.PermMachineReadEvents,
		Context: permission.Context(permission.CtxIaaS, "test-iaas"),
	})
	iaas.RegisterIaasProvider("test-iaas", newTestIaaS)
	evt, err := event.New(&event.Opts{
		Target: event.Target{Type: event.TargetTypeIaas, Value: "test-iaas"},
		Owner:  s.token,
		Kind:   permission.PermAppDeploy,
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

func (s *EventSuite) TestEventInfoIaasWithoutPermission(c *check.C) {
	token := customUserWithPermission(c, "myuser", permission.Permission{
		Scheme:  permission.PermAppDeploy,
		Context: permission.Context(permission.CtxTeam, s.team.Name),
	})
	iaas.RegisterIaasProvider("test-iaas", newTestIaaS)
	evt, err := event.New(&event.Opts{
		Target: event.Target{Type: event.TargetTypeIaas, Value: "test-iaas"},
		Owner:  s.token,
		Kind:   permission.PermAppDeploy,
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

func (s *EventSuite) TestEventInfoRolePermission(c *check.C) {
	token := customUserWithPermission(c, "myuser", permission.Permission{
		Scheme:  permission.PermRoleReadEvents,
		Context: permission.Context(permission.CtxGlobal, ""),
	})
	_, err := permission.NewRole("test", "app", "")
	c.Assert(err, check.IsNil)
	evt, err := event.New(&event.Opts{
		Target: event.Target{Type: event.TargetTypeRole, Value: "test"},
		Owner:  s.token,
		Kind:   permission.PermAppDeploy,
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

func (s *EventSuite) TestEventInfoRoleWithoutPermission(c *check.C) {
	token := customUserWithPermission(c, "myuser", permission.Permission{
		Scheme:  permission.PermAppDeploy,
		Context: permission.Context(permission.CtxTeam, s.team.Name),
	})
	_, err := permission.NewRole("test", "app", "")
	c.Assert(err, check.IsNil)
	evt, err := event.New(&event.Opts{
		Target: event.Target{Type: event.TargetTypeRole, Value: "test"},
		Owner:  s.token,
		Kind:   permission.PermAppDeploy,
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

func (s *EventSuite) TestEventCancelAppPermission(c *check.C) {
	token := customUserWithPermission(c, "myuser", permission.Permission{
		Scheme:  permission.PermAppUpdate,
		Context: permission.Context(permission.CtxTeam, s.team.Name),
	})
	a := app.App{Name: "new-app", Platform: "zend", TeamOwner: s.team.Name}
	usr, err := token.User()
	c.Assert(err, check.IsNil)
	err = app.CreateApp(&a, usr)
	c.Assert(err, check.IsNil)
	evt, err := event.New(&event.Opts{
		Target:     event.Target{Type: event.TargetTypeApp, Value: a.Name},
		Owner:      s.token,
		Kind:       permission.PermAppDeploy,
		Cancelable: true,
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

func (s *EventSuite) TestEventCancelAppWithoutPermission(c *check.C) {
	token := customUserWithPermission(c, "myuser", permission.Permission{
		Scheme:  permission.PermAppDeploy,
		Context: permission.Context(permission.CtxTeam, s.team.Name),
	})
	a := app.App{Name: "new-app2", Platform: "zend", TeamOwner: s.team.Name}
	usr, err := token.User()
	c.Assert(err, check.IsNil)
	err = app.CreateApp(&a, usr)
	c.Assert(err, check.IsNil)
	evt, err := event.New(&event.Opts{
		Target:     event.Target{Type: event.TargetTypeApp, Value: a.Name},
		Owner:      s.token,
		Kind:       permission.PermAppDeploy,
		Cancelable: true,
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

func (s *EventSuite) TestEventCancelTeamPermission(c *check.C) {
	token := customUserWithPermission(c, "myuser", permission.Permission{
		Scheme:  permission.PermTeamUpdate,
		Context: permission.Context(permission.CtxTeam, s.team.Name),
	})
	evt, err := event.New(&event.Opts{
		Target:     event.Target{Type: event.TargetTypeTeam, Value: s.team.Name},
		Owner:      s.token,
		Kind:       permission.PermAppDeploy,
		Cancelable: true,
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

func (s *EventSuite) TestEventCancelTeamWithoutPermission(c *check.C) {
	token := customUserWithPermission(c, "myuser", permission.Permission{
		Scheme:  permission.PermAppDeploy,
		Context: permission.Context(permission.CtxTeam, s.team.Name),
	})
	evt, err := event.New(&event.Opts{
		Target:     event.Target{Type: event.TargetTypeTeam, Value: s.team.Name},
		Owner:      s.token,
		Kind:       permission.PermAppDeploy,
		Cancelable: true,
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

func (s *EventSuite) TestEventCancelServicePermission(c *check.C) {
	token := customUserWithPermission(c, "myuser", permission.Permission{
		Scheme:  permission.PermServiceUpdate,
		Context: permission.Context(permission.CtxTeam, s.team.Name),
	})
	srv := service.Service{
		Name:       "myservice",
		OwnerTeams: []string{s.team.Name},
		Username:   "myuser",
	}
	err := srv.Create()
	c.Assert(err, check.IsNil)
	evt, err := event.New(&event.Opts{
		Target:     event.Target{Type: event.TargetTypeService, Value: srv.Name},
		Owner:      s.token,
		Kind:       permission.PermAppDeploy,
		Cancelable: true,
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

func (s *EventSuite) TestEventCancelServiceWithoutPermission(c *check.C) {
	token := customUserWithPermission(c, "myuser", permission.Permission{
		Scheme:  permission.PermAppDeploy,
		Context: permission.Context(permission.CtxTeam, s.team.Name),
	})
	srv := service.Service{Name: "myservice", OwnerTeams: []string{s.team.Name}}
	err := srv.Create()
	c.Assert(err, check.IsNil)
	evt, err := event.New(&event.Opts{
		Target:     event.Target{Type: event.TargetTypeService, Value: srv.Name},
		Owner:      s.token,
		Kind:       permission.PermAppDeploy,
		Cancelable: true,
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

func (s *EventSuite) TestEventCancelServiceInstancePermission(c *check.C) {
	token := customUserWithPermission(c, "myuser", permission.Permission{
		Scheme:  permission.PermServiceInstanceUpdate,
		Context: permission.Context(permission.CtxTeam, s.team.Name),
	})
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	se := service.Service{Name: "foo", Endpoint: map[string]string{"production": ts.URL}}
	err := se.Create()
	defer s.conn.Services().Remove(bson.M{"_id": se.Name})
	c.Assert(err, check.IsNil)
	si := service.ServiceInstance{Name: "foo-instance", ServiceName: "foo", Teams: []string{s.team.Name}}
	err = si.Create()
	c.Assert(err, check.IsNil)
	evt, err := event.New(&event.Opts{
		Target:     event.Target{Type: event.TargetTypeServiceInstance, Value: "foo/foo-instance"},
		Owner:      s.token,
		Kind:       permission.PermAppDeploy,
		Cancelable: true,
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

func (s *EventSuite) TestEventCancelServiceInstanceWithoutPermission(c *check.C) {
	token := customUserWithPermission(c, "myuser", permission.Permission{
		Scheme:  permission.PermAppDeploy,
		Context: permission.Context(permission.CtxTeam, s.team.Name),
	})
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	se := service.Service{Name: "foo", Endpoint: map[string]string{"production": ts.URL}}
	err := se.Create()
	defer s.conn.Services().Remove(bson.M{"_id": se.Name})
	c.Assert(err, check.IsNil)
	si := service.ServiceInstance{Name: "foo-instance", ServiceName: "foo", Teams: []string{s.team.Name}}
	err = si.Create()
	c.Assert(err, check.IsNil)
	evt, err := event.New(&event.Opts{
		Target:     event.Target{Type: event.TargetTypeServiceInstance, Value: "foo_foo-instance"},
		Owner:      s.token,
		Kind:       permission.PermAppDeploy,
		Cancelable: true,
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

func (s *EventSuite) TestEventCancelServiceInstanceInvalidTargetValue(c *check.C) {
	token := customUserWithPermission(c, "myuser", permission.Permission{
		Scheme:  permission.PermAppDeploy,
		Context: permission.Context(permission.CtxTeam, s.team.Name),
	})
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	se := service.Service{Name: "foo", Endpoint: map[string]string{"production": ts.URL}}
	err := se.Create()
	defer s.conn.Services().Remove(bson.M{"_id": se.Name})
	c.Assert(err, check.IsNil)
	si := service.ServiceInstance{Name: "foo-instance", ServiceName: "foo", Teams: []string{s.team.Name}}
	err = si.Create()
	c.Assert(err, check.IsNil)
	evt, err := event.New(&event.Opts{
		Target:     event.Target{Type: event.TargetTypeServiceInstance, Value: "foofoo-instance"},
		Owner:      s.token,
		Kind:       permission.PermAppDeploy,
		Cancelable: true,
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

func (s *EventSuite) TestEventCancelPoolPermission(c *check.C) {
	token := customUserWithPermission(c, "myuser", permission.Permission{
		Scheme:  permission.PermPoolUpdate,
		Context: permission.Context(permission.CtxPool, "test1"),
	})
	evt, err := event.New(&event.Opts{
		Target:     event.Target{Type: event.TargetTypePool, Value: "test1"},
		Owner:      s.token,
		Kind:       permission.PermAppDeploy,
		Cancelable: true,
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

func (s *EventSuite) TestEventCancelPoolWithoutPermission(c *check.C) {
	token := customUserWithPermission(c, "myuser", permission.Permission{
		Scheme:  permission.PermAppDeploy,
		Context: permission.Context(permission.CtxTeam, s.team.Name),
	})
	evt, err := event.New(&event.Opts{
		Target:     event.Target{Type: event.TargetTypePool, Value: "test1"},
		Owner:      s.token,
		Kind:       permission.PermAppDeploy,
		Cancelable: true,
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

func (s *EventSuite) TestEventCancelUserPermission(c *check.C) {
	token := customUserWithPermission(c, "myuser", permission.Permission{
		Scheme:  permission.PermUserUpdate,
		Context: permission.Context(permission.CtxGlobal, ""),
	})
	evt, err := event.New(&event.Opts{
		Target:     event.Target{Type: event.TargetTypeUser, Value: token.GetUserName()},
		Owner:      s.token,
		Kind:       permission.PermAppDeploy,
		Cancelable: true,
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

func (s *EventSuite) TestEventCancelUserWithoutPermission(c *check.C) {
	token := customUserWithPermission(c, "myuser", permission.Permission{
		Scheme:  permission.PermAppDeploy,
		Context: permission.Context(permission.CtxTeam, s.team.Name),
	})
	evt, err := event.New(&event.Opts{
		Target:     event.Target{Type: event.TargetTypeUser, Value: token.GetUserName()},
		Owner:      s.token,
		Kind:       permission.PermAppDeploy,
		Cancelable: true,
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

func (s *EventSuite) TestEventCancelContainerPermission(c *check.C) {
	token := customUserWithPermission(c, "myuser", permission.Permission{
		Scheme:  permission.PermAppUpdate,
		Context: permission.Context(permission.CtxTeam, s.team.Name),
	})
	usr, err := token.User()
	c.Assert(err, check.IsNil)
	a := app.App{Name: "velha", Platform: "zend", TeamOwner: s.team.Name}
	err = app.CreateApp(&a, usr)
	c.Assert(err, check.IsNil)
	s.provisioner.AddUnits(&a, 3, "web", nil)
	units, err := a.Units()
	c.Assert(err, check.IsNil)
	cnt := units[0]
	evt, err := event.New(&event.Opts{
		Target:     event.Target{Type: event.TargetTypeContainer, Value: cnt.ID},
		Owner:      s.token,
		Kind:       permission.PermAppDeploy,
		Cancelable: true,
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

func (s *EventSuite) TestEventCancelContainerWithoutPermission(c *check.C) {
	token := customUserWithPermission(c, "myuser", permission.Permission{
		Scheme:  permission.PermAppDeploy,
		Context: permission.Context(permission.CtxTeam, s.team.Name),
	})
	usr, err := token.User()
	c.Assert(err, check.IsNil)
	a := app.App{Name: "velha2", Platform: "zend", TeamOwner: s.team.Name}
	err = app.CreateApp(&a, usr)
	c.Assert(err, check.IsNil)
	s.provisioner.AddUnits(&a, 3, "web", nil)
	units, err := a.Units()
	c.Assert(err, check.IsNil)
	cnt := units[0]
	evt, err := event.New(&event.Opts{
		Target:     event.Target{Type: event.TargetTypeContainer, Value: cnt.ID},
		Owner:      s.token,
		Kind:       permission.PermAppDeploy,
		Cancelable: true,
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

func (s *EventSuite) TestEventCancelNodePermission(c *check.C) {
	token := customUserWithPermission(c, "myuser", permission.Permission{
		Scheme:  permission.PermPoolUpdate,
		Context: permission.Context(permission.CtxPool, "test1"),
	})
	s.provisioner.AddNode("mynode", "test1")
	evt, err := event.New(&event.Opts{
		Target:     event.Target{Type: event.TargetTypeNode, Value: "mynode"},
		Owner:      s.token,
		Kind:       permission.PermAppDeploy,
		Cancelable: true,
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

func (s *EventSuite) TestEventCancelNodeWithoutPermission(c *check.C) {
	token := customUserWithPermission(c, "myuser", permission.Permission{
		Scheme:  permission.PermAppDeploy,
		Context: permission.Context(permission.CtxTeam, s.team.Name),
	})
	s.provisioner.AddNode("mynode", "test1")
	evt, err := event.New(&event.Opts{
		Target:     event.Target{Type: event.TargetTypeNode, Value: "mynode"},
		Owner:      s.token,
		Kind:       permission.PermAppDeploy,
		Cancelable: true,
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

func (s *EventSuite) TestEventCancelIassPermission(c *check.C) {
	token := customUserWithPermission(c, "myuser", permission.Permission{
		Scheme:  permission.PermMachineUpdate,
		Context: permission.Context(permission.CtxIaaS, "test-iaas"),
	})
	iaas.RegisterIaasProvider("test-iaas", newTestIaaS)
	evt, err := event.New(&event.Opts{
		Target:     event.Target{Type: event.TargetTypeIaas, Value: "test-iaas"},
		Owner:      s.token,
		Kind:       permission.PermAppDeploy,
		Cancelable: true,
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

func (s *EventSuite) TestEventCancelIaasWithoutPermission(c *check.C) {
	token := customUserWithPermission(c, "myuser", permission.Permission{
		Scheme:  permission.PermAppDeploy,
		Context: permission.Context(permission.CtxTeam, s.team.Name),
	})
	iaas.RegisterIaasProvider("test-iaas", newTestIaaS)
	evt, err := event.New(&event.Opts{
		Target:     event.Target{Type: event.TargetTypeIaas, Value: "test-iaas"},
		Owner:      s.token,
		Kind:       permission.PermAppDeploy,
		Cancelable: true,
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

func (s *EventSuite) TestEventCancelRolePermission(c *check.C) {
	token := customUserWithPermission(c, "myuser", permission.Permission{
		Scheme:  permission.PermRoleUpdate,
		Context: permission.Context(permission.CtxGlobal, ""),
	})
	_, err := permission.NewRole("test", "app", "")
	c.Assert(err, check.IsNil)
	evt, err := event.New(&event.Opts{
		Target:     event.Target{Type: event.TargetTypeRole, Value: "test"},
		Owner:      s.token,
		Kind:       permission.PermAppDeploy,
		Cancelable: true,
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

func (s *EventSuite) TestEventCancelRoleWithoutPermission(c *check.C) {
	token := customUserWithPermission(c, "myuser", permission.Permission{
		Scheme:  permission.PermApp,
		Context: permission.Context(permission.CtxTeam, s.team.Name),
	})
	_, err := permission.NewRole("test", "app", "")
	c.Assert(err, check.IsNil)
	evt, err := event.New(&event.Opts{
		Target:     event.Target{Type: event.TargetTypeRole, Value: "test"},
		Owner:      s.token,
		Kind:       permission.PermAppDeploy,
		Cancelable: true,
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
