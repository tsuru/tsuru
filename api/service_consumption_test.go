// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"sync/atomic"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/api/context"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/io"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/rec/rectest"
	"github.com/tsuru/tsuru/repository/repositorytest"
	"github.com/tsuru/tsuru/service"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

type ConsumptionSuite struct {
	conn        *db.Storage
	team        *auth.Team
	user        *auth.User
	token       auth.Token
	provisioner *provisiontest.FakeProvisioner
	pool        string
	service     *service.Service
	ts          *httptest.Server
	m           http.Handler
}

var _ = check.Suite(&ConsumptionSuite{})

func (s *ConsumptionSuite) SetUpTest(c *check.C) {
	repositorytest.Reset()
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "tsuru_api_consumption_test")
	config.Set("auth:hash-cost", 4)
	config.Set("repo-manager", "fake")
	var err error
	s.conn, err = db.Conn()
	c.Assert(err, check.IsNil)
	dbtest.ClearAllCollections(s.conn.Apps().Database)
	s.team = &auth.Team{Name: "tsuruteam"}
	err = s.conn.Teams().Insert(s.team)
	c.Assert(err, check.IsNil)
	s.token = customUserWithPermission(c, "consumption-master-user", permission.Permission{
		Scheme:  permission.PermServiceInstance,
		Context: permission.Context(permission.CtxTeam, s.team.Name),
	}, permission.Permission{
		Scheme:  permission.PermServiceRead,
		Context: permission.Context(permission.CtxTeam, s.team.Name),
	})
	s.user, err = s.token.User()
	c.Assert(err, check.IsNil)
	app.AuthScheme = nativeScheme
	s.provisioner = provisiontest.NewFakeProvisioner()
	app.Provisioner = s.provisioner
	s.ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"DATABASE_HOST":"localhost"}`))
	}))
	s.service = &service.Service{
		Name:     "mysql",
		Teams:    []string{s.team.Name},
		Endpoint: map[string]string{"production": s.ts.URL},
	}
	s.service.Create()
	s.m = RunServer(true)
}

func (s *ConsumptionSuite) TearDownTest(c *check.C) {
	s.conn.Services().RemoveId(s.service.Name)
	s.conn.Close()
	s.ts.Close()
}

func (s *ConsumptionSuite) TearDownSuite(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	conn.Apps().Database.DropDatabase()
}

func makeRequestToCreateInstanceHandler(params map[string]string, c *check.C) (*httptest.ResponseRecorder, *http.Request) {
	values := url.Values{}
	url := fmt.Sprintf("/services/%s/instances", params["service_name"])
	delete(params, "service_name")
	for k, v := range params {
		values.Add(k, v)
	}
	b := strings.NewReader(values.Encode())
	request, err := http.NewRequest("POST", url, b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", params["token"])
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	return recorder, request
}

func (s *ConsumptionSuite) TestCreateInstanceWithPlan(c *check.C) {
	requestIDHeader := "RequestID"
	config.Set("request-id-header", requestIDHeader)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.Assert(r.Header.Get(requestIDHeader), check.Equals, "test")
		w.Write([]byte(`{"DATABASE_HOST":"localhost"}`))
	}))
	defer ts.Close()
	se := service.Service{
		Name:     "mysql",
		Teams:    []string{s.team.Name},
		Endpoint: map[string]string{"production": ts.URL},
	}
	se.Create()
	defer s.conn.Services().Remove(bson.M{"_id": se.Name})
	params := map[string]string{
		"name":         "brainSQL",
		"service_name": "mysql",
		"plan":         "small",
		"owner":        s.team.Name,
	}
	recorder, request := makeRequestToCreateInstanceHandler(params, c)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	request.Header.Set(requestIDHeader, "test")
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
	var si service.ServiceInstance
	err := s.conn.ServiceInstances().Find(bson.M{
		"name":         "brainSQL",
		"service_name": "mysql",
		"plan_name":    "small",
	}).One(&si)
	c.Assert(err, check.IsNil)
	s.conn.ServiceInstances().Update(bson.M{"name": si.Name}, si)
	c.Assert(si.Name, check.Equals, "brainSQL")
	c.Assert(si.ServiceName, check.Equals, "mysql")
	c.Assert(si.Teams, check.DeepEquals, []string{s.team.Name})
}

func (s *ConsumptionSuite) TestCreateInstanceWithPlanImplicitTeam(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"DATABASE_HOST":"localhost"}`))
	}))
	defer ts.Close()
	se := service.Service{
		Name:     "mysql",
		Teams:    []string{s.team.Name},
		Endpoint: map[string]string{"production": ts.URL},
	}
	se.Create()
	defer s.conn.Services().Remove(bson.M{"_id": se.Name})
	params := map[string]string{
		"name":         "brainSQL",
		"service_name": "mysql",
		"plan":         "small",
	}
	recorder, request := makeRequestToCreateInstanceHandler(params, c)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
	var si service.ServiceInstance
	err := s.conn.ServiceInstances().Find(bson.M{
		"name":         "brainSQL",
		"service_name": "mysql",
		"plan_name":    "small",
	}).One(&si)
	c.Assert(err, check.IsNil)
	s.conn.ServiceInstances().Update(bson.M{"name": si.Name}, si)
	c.Assert(si.Name, check.Equals, "brainSQL")
	c.Assert(si.ServiceName, check.Equals, "mysql")
	c.Assert(si.Teams, check.DeepEquals, []string{s.team.Name})
}

func (s *ConsumptionSuite) TestCreateInstanceTeamOwnerMissing(c *check.C) {
	p := permission.Permission{
		Scheme:  permission.PermServiceInstance,
		Context: permission.Context(permission.CtxTeam, "anotherTeam"),
	}
	role, err := permission.NewRole("instance-user", string(p.Context.CtxType), "")
	c.Assert(err, check.IsNil)
	defer auth.RemoveRoleFromAllUsers("instance-user")
	err = role.AddPermissions(p.Scheme.FullName())
	c.Assert(err, check.IsNil)
	user, err := s.token.User()
	c.Assert(err, check.IsNil)
	err = user.AddRole(role.Name, p.Context.Value)
	c.Assert(err, check.IsNil)
	params := map[string]string{
		"name":         "brainSQL",
		"service_name": "mysql",
		"token":        "bearer " + s.token.GetValue(),
	}
	m := RunServer(true)
	recorder, request := makeRequestToCreateInstanceHandler(params, c)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Equals, permission.ErrTooManyTeams.Error()+"\n")
}

func (s *ConsumptionSuite) TestCreateInstanceInvalidName(c *check.C) {
	params := map[string]string{
		"name":         "1brainSQL",
		"service_name": "mysql",
		"owner":        s.team.Name,
		"token":        "bearer " + s.token.GetValue(),
	}
	m := RunServer(true)
	recorder, request := makeRequestToCreateInstanceHandler(params, c)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Equals, service.ErrInvalidInstanceName.Error()+"\n")
}

func (s *ConsumptionSuite) TestCreateInstanceNameAlreadyExists(c *check.C) {
	params := map[string]string{
		"name":         "brainSQL",
		"service_name": "mysql",
		"owner":        s.team.Name,
		"token":        "bearer " + s.token.GetValue(),
	}
	m := RunServer(true)
	recorder, request := makeRequestToCreateInstanceHandler(params, c)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
	c.Assert(recorder.Body.String(), check.Equals, "")
	params["service_name"] = "mysql"
	recorder, request = makeRequestToCreateInstanceHandler(params, c)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusConflict)
	c.Assert(recorder.Body.String(), check.Equals, service.ErrInstanceNameAlreadyExists.Error()+"\n")
}

func (s *ConsumptionSuite) TestCreateInstance(c *check.C) {
	params := map[string]string{
		"name":         "brainSQL",
		"service_name": "mysql",
		"owner":        s.team.Name,
		"token":        "bearer " + s.token.GetValue(),
	}
	recorder, request := makeRequestToCreateInstanceHandler(params, c)
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
	c.Assert(recorder.Body.String(), check.Equals, "")
	var si service.ServiceInstance
	err := s.conn.ServiceInstances().Find(bson.M{"name": "brainSQL", "service_name": "mysql"}).One(&si)
	c.Assert(err, check.IsNil)
	s.conn.ServiceInstances().Update(bson.M{"name": si.Name}, si)
	c.Assert(si.Name, check.Equals, "brainSQL")
	c.Assert(si.ServiceName, check.Equals, "mysql")
	c.Assert(si.TeamOwner, check.Equals, s.team.Name)
}

func (s *ConsumptionSuite) TestCreateInstanceHandlerHasAccessToTheServiceInTheInstance(c *check.C) {
	t := auth.Team{Name: "judaspriest"}
	err := s.conn.Teams().Insert(t)
	c.Assert(err, check.IsNil)
	params := map[string]string{
		"name":         "brainSQL",
		"service_name": "mysql",
		"owner":        s.team.Name,
		"token":        s.token.GetValue(),
	}
	recorder, request := makeRequestToCreateInstanceHandler(params, c)
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
	var si service.ServiceInstance
	err = s.conn.ServiceInstances().Find(bson.M{"name": "brainSQL"}).One(&si)
	c.Assert(err, check.IsNil)
	c.Assert(si.Teams, check.DeepEquals, []string{s.team.Name})
}

func (s *ConsumptionSuite) TestCreateInstanceHandlerReturnsErrorWhenUserCannotUseService(c *check.C) {
	service := service.Service{Name: "mysqlrestricted", IsRestricted: true}
	service.Create()
	params := map[string]string{
		"name":         "brainSQL",
		"service_name": "mysqlrestricted",
		"owner":        s.team.Name,
		"token":        s.token.GetValue(),
	}
	recorder, request := makeRequestToCreateInstanceHandler(params, c)
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}

func (s *ConsumptionSuite) TestCreateInstanceHandlerIgnoresTeamAuthIfServiceIsNotRestricted(c *check.C) {
	params := map[string]string{
		"name":         "brainSQL",
		"service_name": "mysql",
		"owner":        s.team.Name,
		"token":        s.token.GetValue(),
	}
	recorder, request := makeRequestToCreateInstanceHandler(params, c)
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
	var si service.ServiceInstance
	err := s.conn.ServiceInstances().Find(bson.M{"name": "brainSQL"}).One(&si)
	c.Assert(err, check.IsNil)
	c.Assert(si.Name, check.Equals, "brainSQL")
	c.Assert(si.Teams, check.DeepEquals, []string{s.team.Name})
}

func (s *ConsumptionSuite) TestCreateInstanceHandlerNoPermission(c *check.C) {
	token := customUserWithPermission(c, "cantdoanything")
	srvc := service.Service{Name: "mysqlnoperms"}
	err := srvc.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": "mysqlnoperms"})
	params := map[string]string{
		"name":         "brainSQL",
		"service_name": "mysqlnoperms",
		"token":        token.GetValue(),
	}
	recorder, request := makeRequestToCreateInstanceHandler(params, c)
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}

func (s *ConsumptionSuite) TestCreateInstanceHandlerReturnsErrorWhenServiceDoesntExists(c *check.C) {
	params := map[string]string{
		"name":         "brainSQL",
		"service_name": "notfound",
		"owner":        s.team.Name,
	}
	recorder, request := makeRequestToCreateInstanceHandler(params, c)
	err := createServiceInstance(recorder, request, s.token)
	c.Assert(err.Error(), check.Equals, "Service not found")
}

func (s *ConsumptionSuite) TestCreateInstanceHandlerReturnErrorIfTheServiceAPICallFailAndDoesNotSaveTheInstanceInTheDatabase(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()
	srvc := service.Service{Name: "mysqlerror", Endpoint: map[string]string{"production": ts.URL}}
	err := srvc.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": "mysqlerror"})
	params := map[string]string{
		"name":         "brainSQL",
		"service_name": "mysqlerror",
		"owner":        s.team.Name,
	}
	recorder, request := makeRequestToCreateInstanceHandler(params, c)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusInternalServerError)
}

func (s *ConsumptionSuite) TestCreateInstanceWithDescription(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"DATABASE_HOST":"localhost"}`))
	}))
	defer ts.Close()
	se := service.Service{
		Name:     "mysql",
		Teams:    []string{s.team.Name},
		Endpoint: map[string]string{"production": ts.URL},
	}
	se.Create()
	defer s.conn.Services().Remove(bson.M{"_id": se.Name})
	params := map[string]string{
		"name":         "brainSQL",
		"service_name": "mysql",
		"plan":         "small",
		"owner":        s.team.Name,
		"description":  "desc",
	}
	recorder, request := makeRequestToCreateInstanceHandler(params, c)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
	var si service.ServiceInstance
	err := s.conn.ServiceInstances().Find(bson.M{
		"name":         "brainSQL",
		"service_name": "mysql",
		"plan_name":    "small",
	}).One(&si)
	c.Assert(err, check.IsNil)
	c.Assert(si.Name, check.Equals, "brainSQL")
	c.Assert(si.ServiceName, check.Equals, "mysql")
	c.Assert(si.Teams, check.DeepEquals, []string{s.team.Name})
	c.Assert(si.Description, check.Equals, "desc")
}

func makeRequestToUpdateInstanceHandler(params map[string]string, serviceName, instanceName, token string, c *check.C) (*httptest.ResponseRecorder, *http.Request) {
	values := url.Values{}
	for k, v := range params {
		values.Add(k, v)
	}
	b := strings.NewReader(values.Encode())
	url := fmt.Sprintf("/services/%s/instances/%s", serviceName, instanceName)
	request, err := http.NewRequest("PUT", url, b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+token)
	recorder := httptest.NewRecorder()
	return recorder, request
}

func (s *ConsumptionSuite) TestUpdateServiceHandlerServiceInstanceWithDescription(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"DATABASE_HOST":"localhost"}`))
	}))
	defer ts.Close()
	se := service.Service{
		Name:     "mysql",
		Teams:    []string{s.team.Name},
		Endpoint: map[string]string{"production": ts.URL},
	}
	se.Create()
	defer s.conn.Services().Remove(bson.M{"_id": se.Name})
	si := service.ServiceInstance{
		Name:        "brainSQL",
		ServiceName: "mysql",
		Apps:        []string{"other"},
		Teams:       []string{s.team.Name},
		Description: "desc",
	}
	si.Create()
	defer s.conn.ServiceInstances().Remove(bson.M{"_id": si.Name})
	params := map[string]string{
		"description": "changed",
	}
	token := customUserWithPermission(c, "myuser", permission.Permission{
		Scheme:  permission.PermServiceInstanceUpdate,
		Context: permission.Context(permission.CtxServiceInstance, si.Name),
	})
	recorder, request := makeRequestToUpdateInstanceHandler(params, "mysql", "brainSQL", token.GetValue(), c)
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var instance service.ServiceInstance
	err := s.conn.ServiceInstances().Find(bson.M{
		"name":         "brainSQL",
		"service_name": "mysql",
	}).One(&instance)
	c.Assert(err, check.IsNil)
	c.Assert(instance.Name, check.Equals, "brainSQL")
	c.Assert(instance.ServiceName, check.Equals, "mysql")
	c.Assert(instance.Teams, check.DeepEquals, si.Teams)
	c.Assert(instance.Apps, check.DeepEquals, si.Apps)
	c.Assert(instance.Description, check.DeepEquals, "changed")
}

func (s *ConsumptionSuite) TestUpdateServiceHandlerServiceInstanceNoDescription(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"DATABASE_HOST":"localhost"}`))
	}))
	defer ts.Close()
	se := service.Service{
		Name:     "mysql",
		Teams:    []string{s.team.Name},
		Endpoint: map[string]string{"production": ts.URL},
	}
	se.Create()
	defer s.conn.Services().Remove(bson.M{"_id": se.Name})
	si := service.ServiceInstance{
		Name:        "brainSQL",
		ServiceName: "mysql",
		Apps:        []string{"other"},
		Teams:       []string{s.team.Name},
	}
	si.Create()
	defer s.conn.ServiceInstances().Remove(bson.M{"_id": si.Name})
	params := map[string]string{
		"description": "changed",
	}
	token := customUserWithPermission(c, "myuser", permission.Permission{
		Scheme:  permission.PermServiceInstanceUpdate,
		Context: permission.Context(permission.CtxServiceInstance, si.Name),
	})
	recorder, request := makeRequestToUpdateInstanceHandler(params, "mysql", "brainSQL", token.GetValue(), c)
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var instance service.ServiceInstance
	err := s.conn.ServiceInstances().Find(bson.M{
		"name":         "brainSQL",
		"service_name": "mysql",
	}).One(&instance)
	c.Assert(err, check.IsNil)
	c.Assert(instance.Name, check.Equals, "brainSQL")
	c.Assert(instance.ServiceName, check.Equals, "mysql")
	c.Assert(instance.Teams, check.DeepEquals, si.Teams)
	c.Assert(instance.Apps, check.DeepEquals, si.Apps)
	c.Assert(instance.Description, check.DeepEquals, "changed")
}

func (s *ConsumptionSuite) TestUpdateServiceHandlerServiceInstanceNotExist(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"DATABASE_HOST":"localhost"}`))
	}))
	defer ts.Close()
	se := service.Service{
		Name:     "mysql",
		Teams:    []string{s.team.Name},
		Endpoint: map[string]string{"production": ts.URL},
	}
	se.Create()
	defer s.conn.Services().Remove(bson.M{"_id": se.Name})
	params := map[string]string{
		"description": "changed",
	}
	recorder, request := makeRequestToUpdateInstanceHandler(params, "mysql", "brainSQL", s.token.GetValue(), c)
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, "service instance not found\n")
}

func (s *ConsumptionSuite) TestUpdateServiceHandlerWithoutPermissions(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"DATABASE_HOST":"localhost"}`))
	}))
	defer ts.Close()
	se := service.Service{
		Name:     "mysql",
		Teams:    []string{s.team.Name},
		Endpoint: map[string]string{"production": ts.URL},
	}
	se.Create()
	defer s.conn.Services().Remove(bson.M{"_id": se.Name})
	si := service.ServiceInstance{
		Name:        "brainSQL",
		ServiceName: "mysql",
		Apps:        []string{"other"},
		Teams:       []string{s.team.Name},
	}
	si.Create()
	defer s.conn.ServiceInstances().Remove(bson.M{"_id": si.Name})
	params := map[string]string{
		"description": "changed",
	}
	token := customUserWithPermission(c, "myuser")
	recorder, request := makeRequestToUpdateInstanceHandler(params, "mysql", "brainSQL", token.GetValue(), c)
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
	c.Assert(recorder.Body.String(), check.Equals, permission.ErrUnauthorized.Error()+"\n")
}

func (s *ConsumptionSuite) TestUpdateServiceHandlerInvalidDescription(c *check.C) {
	params := map[string]string{
		"description": "",
	}
	recorder, request := makeRequestToUpdateInstanceHandler(params, "mysql", "brainSQL", s.token.GetValue(), c)
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Equals, "Invalid value for description\n")
	params = map[string]string{
		"desc": "desc",
	}
	recorder, request = makeRequestToUpdateInstanceHandler(params, "mysql", "brainSQL", s.token.GetValue(), c)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Equals, "Invalid value for description\n")
}

func makeRequestToRemoveInstanceHandler(service, instance string, c *check.C) (*httptest.ResponseRecorder, *http.Request) {
	url := fmt.Sprintf("/services/%s/instances/%s", service, instance)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	return recorder, request
}

func (s *ConsumptionSuite) TestRemoveServiceInstanceNotFound(c *check.C) {
	se := service.Service{Name: "foo"}
	err := se.Create()
	defer s.conn.Services().Remove(bson.M{"_id": se.Name})
	c.Assert(err, check.IsNil)
	recorder, request := makeRequestToRemoveInstanceHandler("foo", "not-found", c)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
}

func (s *ConsumptionSuite) TestRemoveServiceInstanceHandler(c *check.C) {
	requestIDHeader := "RequestID"
	config.Set("request-id-header", requestIDHeader)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.Assert(r.Header.Get(requestIDHeader), check.Equals, "test")
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
	recorder, request := makeRequestToRemoveInstanceHandler("foo", "foo-instance", c)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set(requestIDHeader, "test")
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	var msg io.SimpleJsonMessage
	json.Unmarshal(recorder.Body.Bytes(), &msg)
	c.Assert(msg.Message, check.Equals, `service instance successfuly removed`)
	n, err := s.conn.ServiceInstances().Find(bson.M{"name": "foo-instance", "service_name": "foo"}).Count()
	c.Assert(err, check.IsNil)
	c.Assert(n, check.Equals, 0)
	action := rectest.Action{
		Action: "remove-service-instance",
		User:   s.user.Email,
		Extra:  []interface{}{"foo", "foo-instance"},
	}
	c.Assert(action, rectest.IsRecorded)
}

func (s *ConsumptionSuite) TestRemoveServiceInstanceWithSameInstaceName(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	services := []service.Service{
		{Name: "foo", Endpoint: map[string]string{"production": ts.URL}},
		{Name: "foo2", Endpoint: map[string]string{"production": ts.URL}},
	}
	for _, service := range services {
		err := service.Create()
		c.Assert(err, check.IsNil)
		defer s.conn.Services().Remove(bson.M{"_id": service.Name})
	}
	p := app.Platform{Name: "zend"}
	s.conn.Platforms().Insert(p)
	s.pool = "test1"
	opts := provision.AddPoolOptions{Name: "test1", Default: true}
	err := provision.AddPool(opts)
	c.Assert(err, check.IsNil)
	a := app.App{
		Name:      "app-instance",
		Platform:  "zend",
		TeamOwner: s.team.Name,
	}
	err = app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	units, _ := s.provisioner.AddUnits(&a, 1, "web", nil)
	si := []service.ServiceInstance{
		{
			Name:        "foo-instance",
			ServiceName: "foo",
			Teams:       []string{s.team.Name},
			Apps:        []string{"app-instance"},
			Units:       []string{units[0].ID},
		},
		{
			Name:        "foo-instance",
			ServiceName: "foo2",
			Teams:       []string{s.team.Name},
			Apps:        []string{},
		},
	}
	for _, instance := range si {
		err = instance.Create()
		c.Assert(err, check.IsNil)
	}
	recorder, request := makeRequestToRemoveInstanceHandler("foo2", "foo-instance", c)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	expected := ""
	expected += `{"Message":"service instance successfuly removed"}` + "\n"
	c.Assert(recorder.Body.String(), check.Equals, expected)
	var result []service.ServiceInstance
	n, err := s.conn.ServiceInstances().Find(bson.M{"name": "foo-instance", "service_name": "foo2"}).Count()
	c.Assert(err, check.IsNil)
	c.Assert(n, check.Equals, 0)
	err = s.conn.ServiceInstances().Find(bson.M{"name": "foo-instance", "service_name": "foo"}).All(&result)
	c.Assert(err, check.IsNil)
	c.Assert(result, check.HasLen, 1)
	c.Assert(result[0].Apps, check.DeepEquals, []string{"app-instance"})
	recorder, request = makeRequestToRemoveInstanceHandlerWithUnbind("foo", "foo-instance", c)
	err = removeServiceInstance(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	expected = ""
	expected += `{"Message":"Unbind app \"app-instance\" ...\n"}` + "\n"
	expected += `{"Message":"\nInstance \"foo-instance\" is not bound to the app \"app-instance\" anymore.\n"}` + "\n"
	expected += `{"Message":"service instance successfuly removed"}` + "\n"
	c.Assert(recorder.Body.String(), check.Equals, expected)
	n, err = s.conn.ServiceInstances().Find(bson.M{"name": "foo-instance", "service_name": "foo"}).Count()
	c.Assert(err, check.IsNil)
	c.Assert(n, check.Equals, 0)
}

func (s *ConsumptionSuite) TestRemoveServiceHandlerWithoutPermissionShouldReturn401(c *check.C) {
	se := service.Service{Name: "foo-service"}
	err := se.Create()
	defer s.conn.Services().Remove(bson.M{"_id": se.Name})
	c.Assert(err, check.IsNil)
	si := service.ServiceInstance{Name: "foo-instance", ServiceName: "foo-service"}
	err = si.Create()
	defer s.conn.ServiceInstances().Remove(bson.M{"name": si.Name, "service_name": si.ServiceName})
	c.Assert(err, check.IsNil)
	recorder, request := makeRequestToRemoveInstanceHandler("foo-service", "foo-instance", c)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var msg io.SimpleJsonMessage
	json.Unmarshal(recorder.Body.Bytes(), &msg)
	c.Assert(msg.Error, check.Equals, permission.ErrUnauthorized.Error())
}

func (s *ConsumptionSuite) TestRemoveServiceHandlerWIthAssociatedAppsShouldFailAndReturnError(c *check.C) {
	se := service.Service{Name: "foo"}
	err := se.Create()
	defer s.conn.Services().Remove(bson.M{"_id": se.Name})
	c.Assert(err, check.IsNil)
	si := service.ServiceInstance{Name: "foo-instance", ServiceName: "foo", Apps: []string{"foo-bar"}, Teams: []string{s.team.Name}}
	err = si.Create()
	defer s.conn.ServiceInstances().Remove(bson.M{"name": si.Name})
	c.Assert(err, check.IsNil)
	recorder, request := makeRequestToRemoveInstanceHandler("foo", "foo-instance", c)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var msg io.SimpleJsonMessage
	json.Unmarshal(recorder.Body.Bytes(), &msg)
	c.Assert(msg.Error, check.Equals, "This service instance is bound to at least one app. Unbind them before removing it")
}

func makeRequestToRemoveInstanceHandlerWithUnbind(service, instance string, c *check.C) (*httptest.ResponseRecorder, *http.Request) {
	url := fmt.Sprintf("/services/%s/instances/%s?:service=%s&:instance=%s&unbindall=%s", service, instance, service, instance, "true")
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	return recorder, request
}

func (s *ConsumptionSuite) TestRemoveServiceHandlerWIthAssociatedAppsWithUnbindAll(c *check.C) {
	err := s.conn.Services().RemoveId(s.service.Name)
	c.Assert(err, check.IsNil)
	var called int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" && r.URL.Path == "/resources/my-mysql/bind" {
			atomic.StoreInt32(&called, 1)
		}
	}))
	defer ts.Close()
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}}
	err = srvc.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": "mysql"})
	p := app.Platform{Name: "zend"}
	s.conn.Platforms().Insert(p)
	s.pool = "test1"
	opts := provision.AddPoolOptions{Name: "test1", Default: true}
	err = provision.AddPool(opts)
	c.Assert(err, check.IsNil)
	a := app.App{
		Name:      "painkiller",
		Platform:  "zend",
		TeamOwner: s.team.Name,
	}
	err = app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	units, _ := s.provisioner.AddUnits(&a, 1, "web", nil)
	instance := service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
		Apps:        []string{"painkiller"},
		Units:       []string{units[0].ID},
	}
	err = instance.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": "my-mysql"})
	recorder, request := makeRequestToRemoveInstanceHandlerWithUnbind("mysql", "my-mysql", c)
	err = removeServiceInstance(recorder, request, s.token)
	c.Assert(err, check.IsNil)
}

func makeRequestToRemoveInstanceHandlerWithNoUnbind(service, instance string, c *check.C) (*httptest.ResponseRecorder, *http.Request) {
	url := fmt.Sprintf("/services/%s/instances/%s?:service=%s&:instance=%s&unbindall=%s", service, instance, service, instance, "false")
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	return recorder, request
}

func (s *ConsumptionSuite) TestRemoveServiceHandlerWIthAssociatedAppsWithNoUnbindAll(c *check.C) {
	var called int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" && r.URL.Path == "/resources/my-mysql/bind" {
			atomic.StoreInt32(&called, 1)
		}
	}))
	defer ts.Close()
	srvc := service.Service{Name: "mysqlremove", Endpoint: map[string]string{"production": ts.URL}}
	err := srvc.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": "mysqlremove"})
	p := app.Platform{Name: "zend"}
	s.conn.Platforms().Insert(p)
	s.pool = "test1"
	opts := provision.AddPoolOptions{Name: "test1", Default: true}
	err = provision.AddPool(opts)
	c.Assert(err, check.IsNil)
	a := app.App{
		Name:      "app1",
		Platform:  "zend",
		TeamOwner: s.team.Name,
	}
	err = app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	units, _ := s.provisioner.AddUnits(&a, 1, "web", nil)
	instance := service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysqlremove",
		Teams:       []string{s.team.Name},
		Apps:        []string{"app1"},
		Units:       []string{units[0].ID},
	}
	err = instance.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": "my-mysql"})
	recorder, request := makeRequestToRemoveInstanceHandlerWithNoUnbind("mysqlremove", "my-mysql", c)
	err = removeServiceInstance(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	var msg io.SimpleJsonMessage
	json.Unmarshal(recorder.Body.Bytes(), &msg)
	c.Assert(msg.Error, check.Equals, service.ErrServiceInstanceBound.Error())
}

func (s *ConsumptionSuite) TestRemoveServiceHandlerWIthAssociatedAppsWithNoUnbindAllListAllApp(c *check.C) {
	var called int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" && r.URL.Path == "/resources/my-mysql/bind" {
			atomic.StoreInt32(&called, 1)
		}
	}))
	defer ts.Close()
	srvc := service.Service{Name: "mysqlremove", Endpoint: map[string]string{"production": ts.URL}}
	err := srvc.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": "mysqlremove"})
	p := app.Platform{Name: "zend"}
	s.conn.Platforms().Insert(p)
	s.pool = "test1"
	opts := provision.AddPoolOptions{Name: "test1", Default: true}
	err = provision.AddPool(opts)
	c.Assert(err, check.IsNil)
	a := app.App{
		Name:      "app",
		Platform:  "zend",
		TeamOwner: s.team.Name,
	}
	ab := app.App{
		Name:      "app2",
		Platform:  "zend",
		TeamOwner: s.team.Name,
	}
	err = app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	err = app.CreateApp(&ab, s.user)
	c.Assert(err, check.IsNil)
	units, _ := s.provisioner.AddUnits(&a, 1, "web", nil)
	units, _ = s.provisioner.AddUnits(&ab, 1, "web", nil)
	instance := service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysqlremove",
		Teams:       []string{s.team.Name},
		Apps:        []string{"app", "app2"},
		Units:       []string{units[0].ID},
	}
	err = instance.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": "my-mysql"})
	recorder, request := makeRequestToRemoveInstanceHandlerWithNoUnbind("mysqlremove", "my-mysql", c)
	err = removeServiceInstance(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	var msg io.SimpleJsonMessage
	json.Unmarshal(recorder.Body.Bytes(), &msg)
	c.Assert(msg.Error, check.Equals, service.ErrServiceInstanceBound.Error())
	expectedMsg := "app,app2"
	c.Assert(msg.Message, check.Equals, expectedMsg)
}

func (s *ConsumptionSuite) TestRemoveServiceShouldCallTheServiceAPI(c *check.C) {
	var called bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = r.Method == "DELETE" && r.URL.Path == "/resources/purity-instance"
	}))
	defer ts.Close()
	se := service.Service{Name: "purity", Endpoint: map[string]string{"production": ts.URL}}
	err := se.Create()
	defer s.conn.Services().Remove(bson.M{"_id": se.Name})
	c.Assert(err, check.IsNil)
	si := service.ServiceInstance{Name: "purity-instance", ServiceName: "purity", Teams: []string{s.team.Name}}
	err = si.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": si.Name})
	recorder, request := makeRequestToRemoveInstanceHandler("purity", "purity-instance", c)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(called, check.Equals, true)
}

type ServiceModelList []service.ServiceModel

func (l ServiceModelList) Len() int           { return len(l) }
func (l ServiceModelList) Swap(i, j int)      { l[i], l[j] = l[j], l[i] }
func (l ServiceModelList) Less(i, j int) bool { return l[i].Service < l[j].Service }

func (s *ConsumptionSuite) TestServicesInstancesHandler(c *check.C) {
	err := s.conn.Services().RemoveId(s.service.Name)
	c.Assert(err, check.IsNil)
	srv := service.Service{Name: "redis", Teams: []string{s.team.Name}}
	err = srv.Create()
	c.Assert(err, check.IsNil)
	srv2 := service.Service{Name: "mongodb", Teams: []string{s.team.Name}}
	err = srv2.Create()
	c.Assert(err, check.IsNil)
	instance := service.ServiceInstance{
		Name:        "redis-globo",
		ServiceName: "redis",
		Apps:        []string{"globo"},
		Teams:       []string{s.team.Name},
	}
	err = instance.Create()
	c.Assert(err, check.IsNil)
	instance2 := service.ServiceInstance{
		Name:        "mongodb-other",
		ServiceName: "mongodb",
		Apps:        []string{"other"},
		Teams:       []string{s.team.Name},
	}
	err = instance2.Create()
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("GET", "/services/instances", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = serviceInstances(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	var instances []service.ServiceModel
	err = json.Unmarshal(recorder.Body.Bytes(), &instances)
	c.Assert(err, check.IsNil)
	expected := []service.ServiceModel{
		{Service: "mongodb", Instances: []string{"mongodb-other"}, Plans: []string{""}},
		{Service: "redis", Instances: []string{"redis-globo"}, Plans: []string{""}},
	}
	sort.Sort(ServiceModelList(instances))
	c.Assert(instances, check.DeepEquals, expected)
}

func (s *ConsumptionSuite) TestServicesInstancesHandlerAppFilter(c *check.C) {
	err := s.conn.Services().RemoveId(s.service.Name)
	c.Assert(err, check.IsNil)
	srv := service.Service{Name: "redis", Teams: []string{s.team.Name}}
	err = srv.Create()
	c.Assert(err, check.IsNil)
	srv2 := service.Service{Name: "mongodb", Teams: []string{s.team.Name}}
	err = srv2.Create()
	c.Assert(err, check.IsNil)
	instance := service.ServiceInstance{
		Name:        "redis-globo",
		ServiceName: "redis",
		Apps:        []string{"globo"},
		Teams:       []string{s.team.Name},
	}
	err = instance.Create()
	c.Assert(err, check.IsNil)
	instance2 := service.ServiceInstance{
		Name:        "mongodb-other",
		ServiceName: "mongodb",
		Apps:        []string{"other"},
		Teams:       []string{s.team.Name},
	}
	err = instance2.Create()
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("GET", "/services/instances?app=other", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = serviceInstances(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	var instances []service.ServiceModel
	err = json.Unmarshal(recorder.Body.Bytes(), &instances)
	c.Assert(err, check.IsNil)
	expected := []service.ServiceModel{
		{Service: "mongodb", Instances: []string{"mongodb-other"}, Plans: []string{""}},
		{Service: "redis", Instances: []string{}, Plans: []string(nil)},
	}
	sort.Sort(ServiceModelList(instances))
	c.Assert(instances, check.DeepEquals, expected)
}

func (s *ConsumptionSuite) TestServicesInstancesHandlerReturnsOnlyServicesThatTheUserHasAccess(c *check.C) {
	err := s.conn.Services().RemoveId(s.service.Name)
	c.Assert(err, check.IsNil)
	u := &auth.User{Email: "me@globo.com", Password: "123456"}
	_, err = nativeScheme.Create(u)
	c.Assert(err, check.IsNil)
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	token, err := nativeScheme.Login(map[string]string{"email": u.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
	defer s.conn.Tokens().Remove(bson.M{"token": token.GetValue()})
	srv := service.Service{Name: "redis", IsRestricted: true}
	err = s.conn.Services().Insert(srv)
	c.Assert(err, check.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": "redis"})
	instance := service.ServiceInstance{
		Name:        "redis-globo",
		ServiceName: "redis",
		Apps:        []string{"globo"},
	}
	err = instance.Create()
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("GET", "/services/instances", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNoContent)
}

func (s *ConsumptionSuite) TestServicesInstancesHandlerFilterInstancesPerServiceIncludingServicesThatDoesNotHaveInstances(c *check.C) {
	serviceNames := []string{"redis", "pgsql", "memcached"}
	defer s.conn.Services().RemoveAll(bson.M{"name": bson.M{"$in": serviceNames}})
	defer s.conn.ServiceInstances().RemoveAll(bson.M{"service_name": bson.M{"$in": serviceNames}})
	for _, name := range serviceNames {
		srv := service.Service{Name: name, Teams: []string{s.team.Name}}
		err := srv.Create()
		c.Assert(err, check.IsNil)
		instance := service.ServiceInstance{
			Name:        srv.Name + "1",
			ServiceName: srv.Name,
			Teams:       []string{s.team.Name},
		}
		err = instance.Create()
		c.Assert(err, check.IsNil)
		instance = service.ServiceInstance{
			Name:        srv.Name + "2",
			ServiceName: srv.Name,
			Teams:       []string{s.team.Name},
		}
		err = instance.Create()
	}
	srv := service.Service{Name: "oracle", Teams: []string{s.team.Name}}
	err := srv.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.Services().Remove(bson.M{"name": "oracle"})
	request, err := http.NewRequest("GET", "/services/instances", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = serviceInstances(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	var instances []service.ServiceModel
	err = json.Unmarshal(recorder.Body.Bytes(), &instances)
	c.Assert(err, check.IsNil)
	sort.Sort(ServiceModelList(instances))
	expected := []service.ServiceModel{
		{Service: "memcached", Instances: []string{"memcached1", "memcached2"}, Plans: []string{"", ""}},
		{Service: "mysql", Instances: []string{}, Plans: []string(nil)},
		{Service: "oracle", Instances: []string{}, Plans: []string(nil)},
		{Service: "pgsql", Instances: []string{"pgsql1", "pgsql2"}, Plans: []string{"", ""}},
		{Service: "redis", Instances: []string{"redis1", "redis2"}, Plans: []string{"", ""}},
	}
	c.Assert(instances, check.DeepEquals, expected)
}

func makeRequestToStatusHandler(service string, instance string, c *check.C) (*httptest.ResponseRecorder, *http.Request) {
	url := fmt.Sprintf("/services/%s/instances/%s/status/?:instance=%s&:service=%s", service, instance, instance, service)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	return recorder, request
}

func (s *ConsumptionSuite) TestServiceInstanceStatusHandler(c *check.C) {
	requestIDHeader := "RequestID"
	config.Set("request-id-header", requestIDHeader)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/resources/my_nosql/status" {
			c.Assert(r.Header.Get(requestIDHeader), check.Equals, "test")
		}
		w.WriteHeader(http.StatusNoContent)
		w.Write([]byte(`Service instance "my_nosql" is up`))
	}))
	defer ts.Close()
	srv := service.Service{Name: "mongodb", OwnerTeams: []string{s.team.Name}, Endpoint: map[string]string{"production": ts.URL}}
	err := srv.Create()
	c.Assert(err, check.IsNil)
	defer srv.Delete()
	si := service.ServiceInstance{Name: "my_nosql", ServiceName: srv.Name, Teams: []string{s.team.Name}}
	err = si.Create()
	c.Assert(err, check.IsNil)
	defer service.DeleteInstance(&si, "")
	recorder, request := makeRequestToStatusHandler("mongodb", "my_nosql", c)
	context.SetRequestID(request, requestIDHeader, "test")
	err = serviceInstanceStatus(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Body.String(), check.Equals, "Service instance \"my_nosql\" is up")
}

func (s *ConsumptionSuite) TestServiceInstanceStatusWithSameInstanceName(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
		w.Write([]byte(`Service instance "my_nosql" is up`))
	}))
	ts1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`Service instance "my_nosql" is down`))
	}))

	defer ts.Close()
	srv := service.Service{Name: "mongodb", OwnerTeams: []string{s.team.Name}, Endpoint: map[string]string{"production": ts.URL}}
	err := srv.Create()
	c.Assert(err, check.IsNil)
	defer srv.Delete()
	srv2 := service.Service{Name: "mongodb2", OwnerTeams: []string{s.team.Name}, Endpoint: map[string]string{"production": ts1.URL}}
	err = srv2.Create()
	c.Assert(err, check.IsNil)
	defer srv2.Delete()
	si := service.ServiceInstance{Name: "my_nosql", ServiceName: srv.Name, Teams: []string{s.team.Name}}
	err = si.Create()
	c.Assert(err, check.IsNil)
	defer service.DeleteInstance(&si, "")
	si2 := service.ServiceInstance{Name: "my_nosql", ServiceName: srv2.Name, Teams: []string{s.team.Name}}
	err = si2.Create()
	c.Assert(err, check.IsNil)
	defer service.DeleteInstance(&si2, "")
	recorder, request := makeRequestToStatusHandler("mongodb2", "my_nosql", c)
	err = serviceInstanceStatus(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Body.String(), check.Equals, "Service instance \"my_nosql\" is down")
}

func (s *ConsumptionSuite) TestServiceInstanceStatusHandlerShouldReturnErrorWhenServiceInstanceNotExists(c *check.C) {
	recorder, request := makeRequestToStatusHandler("service", "inexistent-instance", c)
	err := serviceInstanceStatus(recorder, request, s.token)
	c.Assert(err, check.ErrorMatches, "^service instance not found$")
}

func (s *ConsumptionSuite) TestServiceInstanceStatusHandlerShouldReturnForbiddenWhenUserDontHaveAccess(c *check.C) {
	srv := service.Service{Name: "mongodb", OwnerTeams: []string{s.team.Name}}
	err := srv.Create()
	c.Assert(err, check.IsNil)
	defer srv.Delete()
	si := service.ServiceInstance{Name: "my_nosql", ServiceName: srv.Name}
	err = si.Create()
	c.Assert(err, check.IsNil)
	defer service.DeleteInstance(&si, "")
	recorder, request := makeRequestToStatusHandler("mongodb", "my_nosql", c)
	err = serviceInstanceStatus(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusForbidden)
}

func makeRequestToInfoHandler(service, instance, token string, c *check.C) (*httptest.ResponseRecorder, *http.Request) {
	url := fmt.Sprintf("/services/%s/instances/%s", service, instance)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+token)
	recorder := httptest.NewRecorder()
	return recorder, request
}

func (s *ConsumptionSuite) TestServiceInstanceInfoHandler(c *check.C) {
	requestIDHeader := "RequestID"
	config.Set("request-id-header", requestIDHeader)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/resources/my_nosql" {
			w.Write([]byte(`[{"label": "key", "value": "value"}, {"label": "key2", "value": "value2"}]`))
		}
		if r.Method == "GET" && r.URL.Path == "/resources/plans" {
			w.Write([]byte(`[{"name": "ignite", "description": "some value"}, {"name": "small", "description": "not space left for you"}]`))
		}
		c.Assert(r.Header.Get(requestIDHeader), check.Equals, "test")
	}))
	defer ts.Close()
	srv := service.Service{Name: "mongodb", Teams: []string{s.team.Name}, Endpoint: map[string]string{"production": ts.URL}}
	err := srv.Create()
	c.Assert(err, check.IsNil)
	defer srv.Delete()
	si := service.ServiceInstance{
		Name:        "my_nosql",
		ServiceName: srv.Name,
		Apps:        []string{"app1", "app2"},
		Teams:       []string{s.team.Name},
		TeamOwner:   s.team.Name,
		PlanName:    "small",
		Description: "desc",
	}
	err = si.Create()
	c.Assert(err, check.IsNil)
	defer service.DeleteInstance(&si, "")
	recorder, request := makeRequestToInfoHandler("mongodb", "my_nosql", s.token.GetValue(), c)
	request.Header.Set(requestIDHeader, "test")
	s.m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	var instances serviceInstanceInfo
	err = json.Unmarshal(recorder.Body.Bytes(), &instances)
	c.Assert(err, check.IsNil)
	expected := serviceInstanceInfo{
		Apps:      si.Apps,
		Teams:     si.Teams,
		TeamOwner: si.TeamOwner,
		CustomInfo: map[string]string{
			"key":  "value",
			"key2": "value2",
		},
		PlanName:        "small",
		PlanDescription: "not space left for you",
		Description:     si.Description,
	}
	c.Assert(instances, check.DeepEquals, expected)
}

func (s *ConsumptionSuite) TestServiceInstanceInfoHandlerNoPlanAndNoCustomInfo(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[]`))
	}))
	defer ts.Close()
	srv := service.Service{Name: "mongodb", Teams: []string{s.team.Name}, Endpoint: map[string]string{"production": ts.URL}}
	err := srv.Create()
	c.Assert(err, check.IsNil)
	defer srv.Delete()
	si := service.ServiceInstance{
		Name:        "my_nosql",
		ServiceName: srv.Name,
		Apps:        []string{"app1", "app2"},
		Teams:       []string{s.team.Name},
		TeamOwner:   s.team.Name,
	}
	err = si.Create()
	c.Assert(err, check.IsNil)
	defer service.DeleteInstance(&si, "")
	recorder, request := makeRequestToInfoHandler("mongodb", "my_nosql", s.token.GetValue(), c)
	s.m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var instances serviceInstanceInfo
	err = json.Unmarshal(recorder.Body.Bytes(), &instances)
	c.Assert(err, check.IsNil)
	expected := serviceInstanceInfo{
		Apps:            si.Apps,
		Teams:           si.Teams,
		TeamOwner:       si.TeamOwner,
		CustomInfo:      map[string]string{},
		PlanName:        "",
		PlanDescription: "",
		Description:     si.Description,
	}
	c.Assert(instances, check.DeepEquals, expected)
}

func (s *ConsumptionSuite) TestServiceInstanceInfoHandlerShouldReturnErrorWhenServiceInstanceNotExists(c *check.C) {
	recorder, request := makeRequestToInfoHandler("mongodb", "inexistent-instance", s.token.GetValue(), c)
	s.m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
}

func (s *ConsumptionSuite) TestServiceInstanceInfoHandlerShouldReturnForbiddenWhenUserDontHaveAccess(c *check.C) {
	srv := service.Service{Name: "mongodb", OwnerTeams: []string{s.team.Name}}
	err := srv.Create()
	c.Assert(err, check.IsNil)
	defer srv.Delete()
	si := service.ServiceInstance{Name: "my_nosql", ServiceName: srv.Name}
	err = si.Create()
	c.Assert(err, check.IsNil)
	defer service.DeleteInstance(&si, "")
	recorder, request := makeRequestToInfoHandler("mongodb", "my_nosql", s.token.GetValue(), c)
	s.m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}

func (s *ConsumptionSuite) TestServiceInfoHandler(c *check.C) {
	srv := service.Service{Name: "mongodb", Teams: []string{s.team.Name}}
	err := srv.Create()
	c.Assert(err, check.IsNil)
	defer srv.Delete()
	si1 := service.ServiceInstance{
		Name:        "my_nosql",
		ServiceName: srv.Name,
		Apps:        []string{},
		Teams:       []string{s.team.Name},
	}
	err = si1.Create()
	c.Assert(err, check.IsNil)
	defer service.DeleteInstance(&si1, "")
	si2 := service.ServiceInstance{
		Name:        "your_nosql",
		ServiceName: srv.Name,
		Apps:        []string{"wordpress"},
		Teams:       []string{s.team.Name},
	}
	err = si2.Create()
	c.Assert(err, check.IsNil)
	defer service.DeleteInstance(&si2, "")
	request, err := http.NewRequest("GET", "/services/mongodb?:name=mongodb", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = serviceInfo(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	var instances []service.ServiceInstance
	err = json.Unmarshal(recorder.Body.Bytes(), &instances)
	c.Assert(err, check.IsNil)
	expected := []service.ServiceInstance{si1, si2}
	c.Assert(instances, check.DeepEquals, expected)
}

func (s *ConsumptionSuite) TestServiceInfoHandlerShouldReturnOnlyInstancesOfTheSameTeamOfTheUser(c *check.C) {
	srv := service.Service{Name: "mongodb", Teams: []string{s.team.Name}}
	err := srv.Create()
	c.Assert(err, check.IsNil)
	defer srv.Delete()
	si1 := service.ServiceInstance{
		Name:        "my_nosql",
		ServiceName: srv.Name,
		Apps:        []string{},
		Teams:       []string{s.team.Name},
	}
	err = si1.Create()
	c.Assert(err, check.IsNil)
	defer service.DeleteInstance(&si1, "")
	si2 := service.ServiceInstance{
		Name:        "your_nosql",
		ServiceName: srv.Name,
		Apps:        []string{"wordpress"},
		Teams:       []string{},
	}
	err = si2.Create()
	c.Assert(err, check.IsNil)
	defer service.DeleteInstance(&si2, "")
	request, err := http.NewRequest("GET", fmt.Sprintf("/services/%s?:name=%s", "mongodb", "mongodb"), nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = serviceInfo(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	var instances []service.ServiceInstance
	err = json.Unmarshal(recorder.Body.Bytes(), &instances)
	c.Assert(err, check.IsNil)
	expected := []service.ServiceInstance{si1}
	c.Assert(instances, check.DeepEquals, expected)
}

func (s *ConsumptionSuite) TestServiceInfoHandlerReturns404WhenTheServiceDoesNotExist(c *check.C) {
	request, err := http.NewRequest("GET", fmt.Sprintf("/services/%s?:name=%s", "mongodb", "mongodb"), nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = serviceInfo(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusNotFound)
	c.Assert(e, check.ErrorMatches, "^Service not found$")
}

func (s *ConsumptionSuite) makeRequestToGetDocHandler(name string, c *check.C) (*httptest.ResponseRecorder, *http.Request) {
	url := fmt.Sprintf("/services/%s/doc/?:name=%s", name, name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	return recorder, request
}

func (s *ConsumptionSuite) TestDocHandler(c *check.C) {
	doc := `Doc for coolnosql
Collnosql is a really really cool nosql`
	srv := service.Service{
		Name:  "coolnosql",
		Doc:   doc,
		Teams: []string{s.team.Name},
	}
	err := srv.Create()
	c.Assert(err, check.IsNil)
	recorder, request := s.makeRequestToGetDocHandler("coolnosql", c)
	err = serviceDoc(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Body.String(), check.Equals, doc)
}

func (s *ConsumptionSuite) TestDocHandlerReturns401WhenUserHasNoAccessToService(c *check.C) {
	srv := service.Service{
		Name:         "coolnosql",
		Doc:          "some doc...",
		IsRestricted: true,
	}
	err := srv.Create()
	c.Assert(err, check.IsNil)
	recorder, request := s.makeRequestToGetDocHandler("coolnosql", c)
	err = serviceDoc(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusForbidden)
}

func (s *ConsumptionSuite) TestDocHandlerReturns404WhenServiceDoesNotExists(c *check.C) {
	recorder, request := s.makeRequestToGetDocHandler("inexistentsql", c)
	err := serviceDoc(recorder, request, s.token)
	c.Assert(err, check.ErrorMatches, "^Service not found$")
}

func (s *ConsumptionSuite) TestGetServiceInstanceOrError(c *check.C) {
	err := s.conn.Services().RemoveId(s.service.Name)
	c.Assert(err, check.IsNil)
	si := service.ServiceInstance{Name: "foo", ServiceName: "foo-service", Teams: []string{s.team.Name}}
	err = si.Create()
	c.Assert(err, check.IsNil)
	rSi, err := getServiceInstanceOrError("foo-service", "foo")
	c.Assert(err, check.IsNil)
	c.Assert(rSi.Name, check.Equals, si.Name)
}

func (s *ConsumptionSuite) TestServicePlansHandler(c *check.C) {
	requestIDHeader := "RequestID"
	config.Set("request-id-header", requestIDHeader)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.Assert(r.Header.Get(requestIDHeader), check.Equals, "test")
		content := `[{"name": "ignite", "description": "some value"}, {"name": "small", "description": "not space left for you"}]`
		w.Write([]byte(content))
	}))
	defer ts.Close()
	srvc := service.Service{Name: "mysqlplan", Endpoint: map[string]string{"production": ts.URL}}
	err := srvc.Create()
	c.Assert(err, check.IsNil)
	defer srvc.Delete()
	request, err := http.NewRequest("GET", "/services/mysqlplan/plans", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	request.Header.Set(requestIDHeader, "test")
	s.m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	var plans []service.Plan
	err = json.Unmarshal(recorder.Body.Bytes(), &plans)
	c.Assert(err, check.IsNil)
	expected := []service.Plan{
		{Name: "ignite", Description: "some value"},
		{Name: "small", Description: "not space left for you"},
	}
	c.Assert(plans, check.DeepEquals, expected)
}

type closeNotifierResponseRecorder struct {
	*httptest.ResponseRecorder
}

func (r *closeNotifierResponseRecorder) CloseNotify() <-chan bool {
	return make(chan bool)
}

func (s *ConsumptionSuite) TestServiceInstanceProxy(c *check.C) {
	var proxyedRequest *http.Request
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyedRequest = r
		w.Header().Set("X-Response-Custom", "custom response header")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte("a message"))
	}))
	defer ts.Close()
	se := service.Service{Name: "foo", Endpoint: map[string]string{"production": ts.URL}}
	err := se.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": se.Name})
	si := service.ServiceInstance{Name: "foo-instance", ServiceName: "foo", Teams: []string{s.team.Name}}
	err = si.Create()
	c.Assert(err, check.IsNil)
	defer service.DeleteInstance(&si, "")
	url := fmt.Sprintf("/services/%s/proxy/%s?callback=/mypath", si.ServiceName, si.Name)
	request, err := http.NewRequest("GET", url, nil)
	reqAuth := "bearer " + s.token.GetValue()
	request.Header.Set("Authorization", reqAuth)
	request.Header.Set("X-Custom", "my request header")
	m := RunServer(true)
	recorder := &closeNotifierResponseRecorder{httptest.NewRecorder()}
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
	c.Assert(recorder.Header().Get("X-Response-Custom"), check.Equals, "custom response header")
	c.Assert(recorder.Body.String(), check.Equals, "a message")
	c.Assert(proxyedRequest, check.NotNil)
	c.Assert(proxyedRequest.Header.Get("X-Custom"), check.Equals, "my request header")
	c.Assert(proxyedRequest.Header.Get("Authorization"), check.Not(check.Equals), reqAuth)
	c.Assert(proxyedRequest.URL.String(), check.Equals, "/mypath")
}

func (s *ConsumptionSuite) TestServiceInstanceProxyNoContent(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()
	se := service.Service{Name: "foo", Endpoint: map[string]string{"production": ts.URL}}
	err := se.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": se.Name})
	si := service.ServiceInstance{Name: "foo-instance", ServiceName: "foo", Teams: []string{s.team.Name}}
	err = si.Create()
	c.Assert(err, check.IsNil)
	defer service.DeleteInstance(&si, "")
	url := fmt.Sprintf("/services/%s/proxy/%s?callback=/mypath", si.ServiceName, si.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	reqAuth := "bearer " + s.token.GetValue()
	request.Header.Set("Authorization", reqAuth)
	m := RunServer(true)
	recorder := &closeNotifierResponseRecorder{httptest.NewRecorder()}
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNoContent)
}

func (s *ConsumptionSuite) TestServiceInstanceProxyError(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte("some error"))
	}))
	defer ts.Close()
	se := service.Service{Name: "foo", Endpoint: map[string]string{"production": ts.URL}}
	err := se.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": se.Name})
	si := service.ServiceInstance{Name: "foo-instance", ServiceName: "foo", Teams: []string{s.team.Name}}
	err = si.Create()
	c.Assert(err, check.IsNil)
	defer service.DeleteInstance(&si, "")
	url := fmt.Sprintf("/services/%s/proxy/%s?callback=/mypath", si.ServiceName, si.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	reqAuth := "bearer " + s.token.GetValue()
	request.Header.Set("Authorization", reqAuth)
	m := RunServer(true)
	recorder := &closeNotifierResponseRecorder{httptest.NewRecorder()}
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadGateway)
	c.Assert(recorder.Body.Bytes(), check.DeepEquals, []byte("some error"))
}

func (s *ConsumptionSuite) TestGrantRevokeServiceToTeam(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("{'AA': 2}"))
	}))
	defer ts.Close()
	se := service.Service{Name: "go", Endpoint: map[string]string{"production": ts.URL}}
	err := se.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": se.Name})
	si := service.ServiceInstance{Name: "si-test", ServiceName: "go", Teams: []string{s.team.Name}}
	err = si.Create()
	c.Assert(err, check.IsNil)
	defer service.DeleteInstance(&si, "")
	team := auth.Team{Name: "test"}
	s.conn.Teams().Insert(team)
	defer s.conn.Teams().Remove(bson.M{"name": team.Name})
	url := fmt.Sprintf("/services/%s/instances/permission/%s/%s?:instance=%s&:team=%s&:service=%s", si.ServiceName, si.Name,
		team.Name, si.Name, team.Name, si.ServiceName)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = serviceInstanceGrantTeam(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	sinst, err := service.GetServiceInstance(si.ServiceName, si.Name)
	c.Assert(err, check.IsNil)
	c.Assert(sinst.Teams, check.DeepEquals, []string{s.team.Name, team.Name})
	request, err = http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	err = serviceInstanceRevokeTeam(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	sinst, err = service.GetServiceInstance(si.ServiceName, si.Name)
	c.Assert(err, check.IsNil)
	c.Assert(sinst.Teams, check.DeepEquals, []string{s.team.Name})
}

func (s *ConsumptionSuite) TestGrantRevokeServiceToTeamWithManyInstanceName(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("{'AA': 2}"))
	}))
	defer ts.Close()
	se := []service.Service{
		{Name: "go", Endpoint: map[string]string{"production": ts.URL}},
		{Name: "go2", Endpoint: map[string]string{"production": ts.URL}},
	}
	for _, service := range se {
		err := service.Create()
		c.Assert(err, check.IsNil)
		defer s.conn.Services().Remove(bson.M{"_id": service.Name})
	}
	si := service.ServiceInstance{Name: "si-test", ServiceName: se[0].Name, Teams: []string{s.team.Name}}
	err := si.Create()
	c.Assert(err, check.IsNil)
	defer service.DeleteInstance(&si, "")
	si2 := service.ServiceInstance{Name: "si-test", ServiceName: se[1].Name, Teams: []string{s.team.Name}}
	err = si2.Create()
	c.Assert(err, check.IsNil)
	defer service.DeleteInstance(&si2, "")
	team := auth.Team{Name: "test"}
	s.conn.Teams().Insert(team)
	defer s.conn.Teams().Remove(bson.M{"name": team.Name})
	url := fmt.Sprintf("/services/%s/instances/permission/%s/%s?:instance=%s&:team=%s&:service=%s", si2.ServiceName, si2.Name,
		team.Name, si2.Name, team.Name, si2.ServiceName)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = serviceInstanceGrantTeam(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	sinst, err := service.GetServiceInstance(si2.ServiceName, si2.Name)
	c.Assert(err, check.IsNil)
	c.Assert(sinst.Teams, check.DeepEquals, []string{s.team.Name, team.Name})
	sinst, err = service.GetServiceInstance(si.ServiceName, si.Name)
	c.Assert(err, check.IsNil)
	c.Assert(sinst.Teams, check.DeepEquals, []string{s.team.Name})
	request, err = http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	err = serviceInstanceRevokeTeam(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	sinst, err = service.GetServiceInstance(si2.ServiceName, si2.Name)
	c.Assert(err, check.IsNil)
	c.Assert(sinst.Teams, check.DeepEquals, []string{s.team.Name})
}
