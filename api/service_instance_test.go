// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"sync/atomic"

	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/api/context"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/event/eventtest"
	"github.com/tsuru/tsuru/io"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/permission/permissiontest"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/repository/repositorytest"
	"github.com/tsuru/tsuru/router/routertest"
	"github.com/tsuru/tsuru/service"
	"github.com/tsuru/tsuru/servicemanager"
	_ "github.com/tsuru/tsuru/storage/mongodb"
	appTypes "github.com/tsuru/tsuru/types/app"
	authTypes "github.com/tsuru/tsuru/types/auth"
	"golang.org/x/crypto/bcrypt"
	"gopkg.in/check.v1"
)

type ServiceInstanceSuite struct {
	conn            *db.Storage
	team            *authTypes.Team
	user            *auth.User
	token           auth.Token
	provisioner     *provisiontest.FakeProvisioner
	pool            string
	service         *service.Service
	ts              *httptest.Server
	testServer      http.Handler
	mockTeamService *auth.MockTeamService
}

var _ = check.Suite(&ServiceInstanceSuite{})

func (s *ServiceInstanceSuite) SetUpSuite(c *check.C) {
	s.testServer = RunServer(true)
}

func (s *ServiceInstanceSuite) SetUpTest(c *check.C) {
	repositorytest.Reset()
	routertest.FakeRouter.Reset()
	config.Set("database:driver", "mongodb")
	config.Set("database:url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("database:name", "tsuru_api_consumption_test")
	config.Set("auth:hash-cost", bcrypt.MinCost)
	config.Set("repo-manager", "fake")
	config.Set("docker:router", "fake")
	config.Set("routers:fake:default", true)
	config.Set("routers:fake:type", "fake")
	var err error
	s.conn, err = db.Conn()
	c.Assert(err, check.IsNil)
	dbtest.ClearAllCollections(s.conn.Apps().Database)
	s.team = &authTypes.Team{Name: "tsuruteam"}
	_, s.token = permissiontest.CustomUserWithPermission(c, nativeScheme, "consumption-master-user", permission.Permission{
		Scheme:  permission.PermServiceInstance,
		Context: permission.Context(permission.CtxTeam, s.team.Name),
	}, permission.Permission{
		Scheme:  permission.PermServiceRead,
		Context: permission.Context(permission.CtxTeam, s.team.Name),
	})
	s.user, err = s.token.User()
	c.Assert(err, check.IsNil)
	app.AuthScheme = nativeScheme
	s.provisioner = provisiontest.ProvisionerInstance
	provision.DefaultProvisioner = "fake"
	s.provisioner.Reset()
	s.ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"DATABASE_HOST":"localhost"}`))
	}))
	s.mockTeamService = &auth.MockTeamService{}
	s.mockTeamService.OnList = func() ([]authTypes.Team, error) {
		return []authTypes.Team{{Name: s.team.Name}}, nil
	}
	s.mockTeamService.OnFindByName = func(name string) (*authTypes.Team, error) {
		return &authTypes.Team{Name: name}, nil
	}
	s.mockTeamService.OnFindByNames = func(names []string) ([]authTypes.Team, error) {
		teams := []authTypes.Team{}
		for _, name := range names {
			teams = append(teams, authTypes.Team{Name: name})
		}
		return teams, nil
	}
	ServiceManager.Team = s.mockTeamService
	servicemanager.Team = s.mockTeamService
	s.service = &service.Service{
		Name:       "mysql",
		Teams:      []string{s.team.Name},
		OwnerTeams: []string{s.team.Name},
		Endpoint:   map[string]string{"production": s.ts.URL},
		Password:   "abcde",
	}
	err = s.service.Create()
	c.Assert(err, check.IsNil)
}

func (s *ServiceInstanceSuite) TearDownTest(c *check.C) {
	s.conn.Services().RemoveId(s.service.Name)
	s.conn.Close()
	s.ts.Close()
}

func (s *ServiceInstanceSuite) TearDownSuite(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	conn.Apps().Database.DropDatabase()
}

func makeRequestToCreateServiceInstance(params map[string]interface{}, c *check.C) (*httptest.ResponseRecorder, *http.Request) {
	values := url.Values{}
	url := fmt.Sprintf("/services/%s/instances", params["service_name"])
	delete(params, "service_name")
	for k, v := range params {
		switch v.(type) {
		case string:
			values.Add(k, v.(string))
		case []string:
			for _, str := range v.([]string) {
				values.Add(k, str)
			}
		}
	}
	b := strings.NewReader(values.Encode())
	request, err := http.NewRequest("POST", url, b)
	c.Assert(err, check.IsNil)
	if token, ok := params["token"].(string); ok {
		request.Header.Set("Authorization", token)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	return recorder, request
}

func (s *ServiceInstanceSuite) TestCreateInstanceWithPlan(c *check.C) {
	requestIDHeader := "RequestID"
	config.Set("request-id-header", requestIDHeader)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.Assert(r.Header.Get(requestIDHeader), check.Equals, "test")
		w.Write([]byte(`{"DATABASE_HOST":"localhost"}`))
	}))
	defer ts.Close()
	se := service.Service{
		Name:       "mysql",
		Teams:      []string{s.team.Name},
		OwnerTeams: []string{s.team.Name},
		Endpoint:   map[string]string{"production": ts.URL},
		Password:   "abcde",
	}
	se.Create()
	params := map[string]interface{}{
		"name":         "brainsql",
		"service_name": "mysql",
		"plan":         "small",
		"owner":        s.team.Name,
	}
	recorder, request := makeRequestToCreateServiceInstance(params, c)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	request.Header.Set(requestIDHeader, "test")
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
	var si service.ServiceInstance
	err := s.conn.ServiceInstances().Find(bson.M{
		"name":         "brainsql",
		"service_name": "mysql",
		"plan_name":    "small",
	}).One(&si)
	c.Assert(err, check.IsNil)
	s.conn.ServiceInstances().Update(bson.M{"name": si.Name}, si)
	c.Assert(si.Name, check.Equals, "brainsql")
	c.Assert(si.ServiceName, check.Equals, "mysql")
	c.Assert(si.Teams, check.DeepEquals, []string{s.team.Name})
}

func (s *ServiceInstanceSuite) TestCreateInstanceWithPlanImplicitTeam(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"DATABASE_HOST":"localhost"}`))
	}))
	defer ts.Close()
	se := service.Service{
		Name:       "mysql",
		Teams:      []string{s.team.Name},
		OwnerTeams: []string{s.team.Name},
		Endpoint:   map[string]string{"production": ts.URL},
		Password:   "abcde",
	}
	se.Create()
	params := map[string]interface{}{
		"name":         "brainsql",
		"service_name": "mysql",
		"plan":         "small",
	}
	recorder, request := makeRequestToCreateServiceInstance(params, c)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
	var si service.ServiceInstance
	err := s.conn.ServiceInstances().Find(bson.M{
		"name":         "brainsql",
		"service_name": "mysql",
		"plan_name":    "small",
	}).One(&si)
	c.Assert(err, check.IsNil)
	s.conn.ServiceInstances().Update(bson.M{"name": si.Name}, si)
	c.Assert(si.Name, check.Equals, "brainsql")
	c.Assert(si.ServiceName, check.Equals, "mysql")
	c.Assert(si.Teams, check.DeepEquals, []string{s.team.Name})
}

func (s *ServiceInstanceSuite) TestCreateInstanceTeamOwnerMissing(c *check.C) {
	p := permission.Permission{
		Scheme:  permission.PermServiceInstance,
		Context: permission.Context(permission.CtxTeam, "anotherTeam"),
	}
	role, err := permission.NewRole("instance-user", string(p.Context.CtxType), "")
	c.Assert(err, check.IsNil)
	err = role.AddPermissions(p.Scheme.FullName())
	c.Assert(err, check.IsNil)
	user, err := s.token.User()
	c.Assert(err, check.IsNil)
	err = user.AddRole(role.Name, p.Context.Value)
	c.Assert(err, check.IsNil)
	params := map[string]interface{}{
		"name":         "brainsql",
		"service_name": "mysql",
		"token":        "bearer " + s.token.GetValue(),
	}
	recorder, request := makeRequestToCreateServiceInstance(params, c)
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Equals, permission.ErrTooManyTeams.Error()+"\n")
}

func (s *ServiceInstanceSuite) TestCreateInstanceInvalidName(c *check.C) {
	params := map[string]interface{}{
		"name":         "1brainsql",
		"service_name": "mysql",
		"owner":        s.team.Name,
		"token":        "bearer " + s.token.GetValue(),
	}
	recorder, request := makeRequestToCreateServiceInstance(params, c)
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Equals, service.ErrInvalidInstanceName.Error()+"\n")
}

func (s *ServiceInstanceSuite) TestCreateInstanceNameAlreadyExists(c *check.C) {
	params := map[string]interface{}{
		"name":         "brainsql",
		"service_name": "mysql",
		"owner":        s.team.Name,
		"token":        "bearer " + s.token.GetValue(),
	}
	recorder, request := makeRequestToCreateServiceInstance(params, c)
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
	c.Assert(recorder.Body.String(), check.Equals, "")
	params["service_name"] = "mysql"
	recorder, request = makeRequestToCreateServiceInstance(params, c)
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusConflict)
	c.Assert(recorder.Body.String(), check.Equals, service.ErrInstanceNameAlreadyExists.Error()+"\n")
}

func (s *ServiceInstanceSuite) TestCreateInstance(c *check.C) {
	params := map[string]interface{}{
		"name":         "brainsql",
		"service_name": "mysql",
		"owner":        s.team.Name,
		"token":        "bearer " + s.token.GetValue(),
	}
	recorder, request := makeRequestToCreateServiceInstance(params, c)
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
	c.Assert(recorder.Body.String(), check.Equals, "")
	var si service.ServiceInstance
	err := s.conn.ServiceInstances().Find(bson.M{"name": "brainsql", "service_name": "mysql"}).One(&si)
	c.Assert(err, check.IsNil)
	s.conn.ServiceInstances().Update(bson.M{"name": si.Name}, si)
	c.Assert(si.Name, check.Equals, "brainsql")
	c.Assert(si.ServiceName, check.Equals, "mysql")
	c.Assert(si.TeamOwner, check.Equals, s.team.Name)
}

func (s *ServiceInstanceSuite) TestCreateServiceInstanceHasAccessToTheServiceInTheInstance(c *check.C) {
	params := map[string]interface{}{
		"name":         "brainsql",
		"service_name": "mysql",
		"owner":        s.team.Name,
		"token":        s.token.GetValue(),
	}
	recorder, request := makeRequestToCreateServiceInstance(params, c)
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
	var si service.ServiceInstance
	err := s.conn.ServiceInstances().Find(bson.M{"name": "brainsql"}).One(&si)
	c.Assert(err, check.IsNil)
	c.Assert(si.Teams, check.DeepEquals, []string{s.team.Name})
}

func (s *ServiceInstanceSuite) TestCreateServiceInstanceReturnsErrorWhenUserCannotUseService(c *check.C) {
	service := service.Service{
		Name:         "mysqlrestricted",
		IsRestricted: true,
		Endpoint:     map[string]string{"production": "http://localhost:1234"},
		Password:     "abcde",
		OwnerTeams:   []string{s.team.Name},
	}
	err := service.Create()
	c.Assert(err, check.IsNil)
	params := map[string]interface{}{
		"name":         "brainsql",
		"service_name": "mysqlrestricted",
		"owner":        s.team.Name,
		"token":        s.token.GetValue(),
	}
	recorder, request := makeRequestToCreateServiceInstance(params, c)
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}

func (s *ServiceInstanceSuite) TestCreateServiceInstanceIgnoresTeamAuthIfServiceIsNotRestricted(c *check.C) {
	params := map[string]interface{}{
		"name":         "brainsql",
		"service_name": "mysql",
		"owner":        s.team.Name,
		"token":        s.token.GetValue(),
	}
	recorder, request := makeRequestToCreateServiceInstance(params, c)
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
	var si service.ServiceInstance
	err := s.conn.ServiceInstances().Find(bson.M{"name": "brainsql"}).One(&si)
	c.Assert(err, check.IsNil)
	c.Assert(si.Name, check.Equals, "brainsql")
	c.Assert(si.Teams, check.DeepEquals, []string{s.team.Name})
	c.Assert(eventtest.EventDesc{
		Target: serviceInstanceTarget("mysql", "brainsql"),
		Owner:  s.token.GetUserName(),
		Kind:   "service-instance.create",
		StartCustomData: []map[string]interface{}{
			{"name": "name", "value": "brainsql"},
			{"name": ":service", "value": "mysql"},
			{"name": "owner", "value": s.team.Name},
		},
	}, eventtest.HasEvent)
}

func (s *ServiceInstanceSuite) TestCreateServiceInstanceNoPermission(c *check.C) {
	_, token := permissiontest.CustomUserWithPermission(c, nativeScheme, "cantdoanything")
	srvc := service.Service{Name: "mysqlnoperms", Endpoint: map[string]string{"production": "http://localhost:1234"}, Password: "abcde", OwnerTeams: []string{s.team.Name}}
	err := srvc.Create()
	c.Assert(err, check.IsNil)
	params := map[string]interface{}{
		"name":         "brainsql",
		"service_name": "mysqlnoperms",
		"token":        token.GetValue(),
	}
	recorder, request := makeRequestToCreateServiceInstance(params, c)
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}

func (s *ServiceInstanceSuite) TestCreateServiceInstanceReturnsErrorWhenServiceDoesntExists(c *check.C) {
	params := map[string]interface{}{
		"name":         "brainsql",
		"service_name": "notfound",
		"owner":        s.team.Name,
	}
	recorder, request := makeRequestToCreateServiceInstance(params, c)
	err := createServiceInstance(recorder, request, s.token)
	c.Assert(err.Error(), check.Equals, "Service not found")
}

func (s *ServiceInstanceSuite) TestCreateServiceInstanceReturnErrorIfTheServiceAPICallFailAndDoesNotSaveTheInstanceInTheDatabase(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()
	srvc := service.Service{Name: "mysqlerror", Endpoint: map[string]string{"production": ts.URL}, Password: "abcde", OwnerTeams: []string{s.team.Name}}
	err := srvc.Create()
	c.Assert(err, check.IsNil)
	params := map[string]interface{}{
		"name":         "brainsql",
		"service_name": "mysqlerror",
		"owner":        s.team.Name,
	}
	recorder, request := makeRequestToCreateServiceInstance(params, c)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusInternalServerError)
	c.Assert(eventtest.EventDesc{
		Target:       serviceInstanceTarget("mysqlerror", "brainsql"),
		Owner:        s.token.GetUserName(),
		Kind:         "service-instance.create",
		ErrorMatches: `.*Failed to create the instance brainsql.*`,
		StartCustomData: []map[string]interface{}{
			{"name": "name", "value": "brainsql"},
			{"name": ":service", "value": "mysqlerror"},
			{"name": "owner", "value": s.team.Name},
		},
	}, eventtest.HasEvent)
}

func (s *ServiceInstanceSuite) TestCreateInstanceWithDescription(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"DATABASE_HOST":"localhost"}`))
	}))
	defer ts.Close()
	se := service.Service{
		Name:       "mysql",
		Teams:      []string{s.team.Name},
		OwnerTeams: []string{s.team.Name},
		Endpoint:   map[string]string{"production": ts.URL},
		Password:   "abcde",
	}
	se.Create()
	params := map[string]interface{}{
		"name":         "brainsql",
		"service_name": "mysql",
		"plan":         "small",
		"owner":        s.team.Name,
		"description":  "desc",
	}
	recorder, request := makeRequestToCreateServiceInstance(params, c)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
	var si service.ServiceInstance
	err := s.conn.ServiceInstances().Find(bson.M{
		"name":         "brainsql",
		"service_name": "mysql",
		"plan_name":    "small",
	}).One(&si)
	c.Assert(err, check.IsNil)
	c.Assert(si.Name, check.Equals, "brainsql")
	c.Assert(si.ServiceName, check.Equals, "mysql")
	c.Assert(si.Teams, check.DeepEquals, []string{s.team.Name})
	c.Assert(si.Description, check.Equals, "desc")
}

func (s *ServiceInstanceSuite) TestCreateServiceInstanceWithTags(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"DATABASE_HOST":"localhost"}`))
	}))
	defer ts.Close()
	se := service.Service{
		Name:       "mysql",
		Teams:      []string{s.team.Name},
		OwnerTeams: []string{s.team.Name},
		Endpoint:   map[string]string{"production": ts.URL},
		Password:   "abcde",
	}
	se.Create()
	params := map[string]interface{}{
		"name":         "brainsql",
		"service_name": "mysql",
		"plan":         "small",
		"owner":        s.team.Name,
		"tag":          []string{"tag a", "tag b"},
	}
	recorder, request := makeRequestToCreateServiceInstance(params, c)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
	var si service.ServiceInstance
	err := s.conn.ServiceInstances().Find(bson.M{
		"name":         "brainsql",
		"service_name": "mysql",
		"plan_name":    "small",
	}).One(&si)
	c.Assert(err, check.IsNil)
	c.Assert(si.Name, check.Equals, "brainsql")
	c.Assert(si.ServiceName, check.Equals, "mysql")
	c.Assert(si.Teams, check.DeepEquals, []string{s.team.Name})
	c.Assert(si.Tags, check.DeepEquals, []string{"tag a", "tag b"})
}

func makeRequestToUpdateServiceInstance(params map[string]interface{}, serviceName, instanceName, token string, c *check.C) (*httptest.ResponseRecorder, *http.Request) {
	values := url.Values{}
	for k, v := range params {
		switch v.(type) {
		case string:
			values.Add(k, v.(string))
		case []string:
			for _, str := range v.([]string) {
				values.Add(k, str)
			}
		}
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

func (s *ServiceInstanceSuite) TestUpdateServiceInstanceWithDescription(c *check.C) {
	requestIDHeader := "RequestID"
	config.Set("request-id-header", requestIDHeader)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.Assert(r.Header.Get(requestIDHeader), check.Equals, "test")
		w.Write([]byte(`{"DATABASE_HOST":"localhost"}`))
	}))
	defer ts.Close()
	si := service.ServiceInstance{
		Name:        "brainsql",
		ServiceName: "mysql",
		Apps:        []string{"other"},
		Teams:       []string{s.team.Name},
		Description: "desc",
		TeamOwner:   s.team.Name,
	}
	err := s.conn.ServiceInstances().Insert(si)
	c.Assert(err, check.IsNil)
	params := map[string]interface{}{
		"description": "changed",
	}
	_, token := permissiontest.CustomUserWithPermission(c, nativeScheme, "myuser", permission.Permission{
		Scheme:  permission.PermServiceInstanceUpdateDescription,
		Context: permission.Context(permission.CtxServiceInstance, serviceIntancePermName("mysql", si.Name)),
	})
	recorder, request := makeRequestToUpdateServiceInstance(params, "mysql", "brainsql", token.GetValue(), c)
	request.Header.Set(requestIDHeader, "test")
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var instance service.ServiceInstance
	err = s.conn.ServiceInstances().Find(bson.M{
		"name":         "brainsql",
		"service_name": "mysql",
	}).One(&instance)
	c.Assert(err, check.IsNil)
	c.Assert(instance.Name, check.Equals, "brainsql")
	c.Assert(instance.ServiceName, check.Equals, "mysql")
	c.Assert(instance.Teams, check.DeepEquals, si.Teams)
	c.Assert(instance.Apps, check.DeepEquals, si.Apps)
	c.Assert(instance.Description, check.Equals, "changed")
	c.Assert(eventtest.EventDesc{
		Target: serviceInstanceTarget("mysql", "brainsql"),
		Owner:  token.GetUserName(),
		Kind:   "service-instance.update",
		StartCustomData: []map[string]interface{}{
			{"name": "description", "value": "changed"},
		},
	}, eventtest.HasEvent)
}

func (s *ServiceInstanceSuite) TestUpdateServiceInstanceWithTeamOwner(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"DATABASE_HOST":"localhost"}`))
	}))
	defer ts.Close()
	si := service.ServiceInstance{
		Name:        "brainsql",
		ServiceName: "mysql",
		Apps:        []string{"other"},
		Teams:       []string{s.team.Name},
		TeamOwner:   s.team.Name,
	}
	err := s.conn.ServiceInstances().Insert(si)
	c.Assert(err, check.IsNil)
	t := authTypes.Team{Name: "changed"}
	params := map[string]interface{}{
		"teamowner": t.Name,
	}
	_, token := permissiontest.CustomUserWithPermission(c, nativeScheme, "myuser", permission.Permission{
		Scheme:  permission.PermServiceInstanceUpdateTeamowner,
		Context: permission.Context(permission.CtxServiceInstance, serviceIntancePermName("mysql", si.Name)),
	})
	recorder, request := makeRequestToUpdateServiceInstance(params, "mysql", "brainsql", token.GetValue(), c)
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var instance service.ServiceInstance
	err = s.conn.ServiceInstances().Find(bson.M{
		"name":         "brainsql",
		"service_name": "mysql",
	}).One(&instance)
	c.Assert(err, check.IsNil)
	c.Assert(instance.Name, check.Equals, "brainsql")
	c.Assert(instance.ServiceName, check.Equals, "mysql")
	c.Assert(instance.Teams, check.DeepEquals, append(si.Teams, t.Name))
	c.Assert(instance.Apps, check.DeepEquals, si.Apps)
	c.Assert(instance.TeamOwner, check.Equals, "changed")
	c.Assert(eventtest.EventDesc{
		Target: serviceInstanceTarget("mysql", "brainsql"),
		Owner:  token.GetUserName(),
		Kind:   "service-instance.update",
		StartCustomData: []map[string]interface{}{
			{"name": "teamowner", "value": t.Name},
		},
	}, eventtest.HasEvent)
}

func (s *ServiceInstanceSuite) TestUpdateServiceInstanceWithTags(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"DATABASE_HOST":"localhost"}`))
	}))
	defer ts.Close()
	si := service.ServiceInstance{
		Name:        "brainsql",
		ServiceName: "mysql",
		Apps:        []string{"other"},
		Teams:       []string{s.team.Name},
		Tags:        []string{"tag a"},
		TeamOwner:   s.team.Name,
	}
	err := s.conn.ServiceInstances().Insert(si)
	c.Assert(err, check.IsNil)
	params := map[string]interface{}{
		"tag": []string{"tag b", "tag c"},
	}
	_, token := permissiontest.CustomUserWithPermission(c, nativeScheme, "myuser", permission.Permission{
		Scheme:  permission.PermServiceInstanceUpdateTags,
		Context: permission.Context(permission.CtxServiceInstance, serviceIntancePermName("mysql", si.Name)),
	})
	recorder, request := makeRequestToUpdateServiceInstance(params, "mysql", "brainsql", token.GetValue(), c)
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var instance service.ServiceInstance
	err = s.conn.ServiceInstances().Find(bson.M{
		"name":         "brainsql",
		"service_name": "mysql",
	}).One(&instance)
	c.Assert(err, check.IsNil)
	c.Assert(instance.Name, check.Equals, "brainsql")
	c.Assert(instance.ServiceName, check.Equals, "mysql")
	c.Assert(instance.Teams, check.DeepEquals, si.Teams)
	c.Assert(instance.Apps, check.DeepEquals, si.Apps)
	c.Assert(instance.Tags, check.DeepEquals, []string{"tag b", "tag c"})
	c.Assert(eventtest.EventDesc{
		Target: serviceInstanceTarget("mysql", "brainsql"),
		Owner:  token.GetUserName(),
		Kind:   "service-instance.update",
		StartCustomData: []map[string]interface{}{
			{"name": "tag", "value": []string{"tag b", "tag c"}},
		},
	}, eventtest.HasEvent)
}

func (s *ServiceInstanceSuite) TestUpdateServiceInstanceWithEmptyTagRemovesTags(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"DATABASE_HOST":"localhost"}`))
	}))
	defer ts.Close()
	si := service.ServiceInstance{
		Name:        "brainsql",
		ServiceName: "mysql",
		Apps:        []string{"other"},
		Teams:       []string{s.team.Name},
		TeamOwner:   s.team.Name,
		Tags:        []string{"tag a"},
	}
	err := s.conn.ServiceInstances().Insert(si)
	c.Assert(err, check.IsNil)
	params := map[string]interface{}{
		"tag": []string{""},
	}
	_, token := permissiontest.CustomUserWithPermission(c, nativeScheme, "myuser", permission.Permission{
		Scheme:  permission.PermServiceInstanceUpdateTags,
		Context: permission.Context(permission.CtxServiceInstance, serviceIntancePermName("mysql", si.Name)),
	})
	recorder, request := makeRequestToUpdateServiceInstance(params, "mysql", "brainsql", token.GetValue(), c)
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var instance service.ServiceInstance
	err = s.conn.ServiceInstances().Find(bson.M{
		"name":         "brainsql",
		"service_name": "mysql",
	}).One(&instance)
	c.Assert(err, check.IsNil)
	c.Assert(instance.Tags, check.HasLen, 0)
}

func (s *ServiceInstanceSuite) TestUpdateServiceInstanceDoesNotExist(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"DATABASE_HOST":"localhost"}`))
	}))
	defer ts.Close()
	params := map[string]interface{}{
		"description": "changed",
	}
	recorder, request := makeRequestToUpdateServiceInstance(params, "mysql", "brainsql", s.token.GetValue(), c)
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, "service instance not found\n")
}

func (s *ServiceInstanceSuite) TestUpdateServiceInstanceWithoutPermissions(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"DATABASE_HOST":"localhost"}`))
	}))
	defer ts.Close()
	si := service.ServiceInstance{
		Name:        "brainsql",
		ServiceName: "mysql",
		Apps:        []string{"other"},
		Teams:       []string{s.team.Name},
	}
	err := s.conn.ServiceInstances().Insert(si)
	c.Assert(err, check.IsNil)
	params := map[string]interface{}{
		"description": "changed",
	}
	_, token := permissiontest.CustomUserWithPermission(c, nativeScheme, "myuser")
	recorder, request := makeRequestToUpdateServiceInstance(params, "mysql", "brainsql", token.GetValue(), c)
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
	c.Assert(recorder.Body.String(), check.Equals, permission.ErrUnauthorized.Error()+"\n")
}

func (s *ServiceInstanceSuite) TestUpdateServiceInstancePlan(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"DATABASE_HOST":"localhost"}`))
	}))
	defer ts.Close()
	si := service.ServiceInstance{
		Name:        "brainsql",
		ServiceName: "mysql",
		Apps:        []string{"other"},
		Teams:       []string{s.team.Name},
		TeamOwner:   s.team.Name,
	}
	err := s.conn.ServiceInstances().Insert(si)
	c.Assert(err, check.IsNil)
	params := map[string]interface{}{
		"plan": "newplan",
	}
	_, token := permissiontest.CustomUserWithPermission(c, nativeScheme, "myuser", permission.Permission{
		Scheme:  permission.PermServiceInstanceUpdatePlan,
		Context: permission.Context(permission.CtxServiceInstance, serviceIntancePermName("mysql", si.Name)),
	})
	recorder, request := makeRequestToUpdateServiceInstance(params, "mysql", "brainsql", token.GetValue(), c)
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(eventtest.EventDesc{
		Target: serviceInstanceTarget("mysql", "brainsql"),
		Owner:  token.GetUserName(),
		Kind:   "service-instance.update",
		StartCustomData: []map[string]interface{}{
			{"name": "plan", "value": "newplan"},
		},
	}, eventtest.HasEvent)
}

func (s *ServiceInstanceSuite) TestUpdateServiceInstanceEmptyFields(c *check.C) {
	si := service.ServiceInstance{
		Name:        "brainsql",
		ServiceName: "mysql",
		Apps:        []string{"other"},
		Teams:       []string{s.team.Name},
	}
	err := s.conn.ServiceInstances().Insert(si)
	c.Assert(err, check.IsNil)
	params := map[string]interface{}{
		"description": "",
		"teamowner":   "",
	}
	recorder, request := makeRequestToUpdateServiceInstance(params, "mysql", "brainsql", s.token.GetValue(), c)
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Equals, "Neither the description, team owner, tags or plan were set. You must define at least one.\n")
}

func makeRequestToRemoveServiceInstance(service, instance string, c *check.C) (*httptest.ResponseRecorder, *http.Request) {
	url := fmt.Sprintf("/services/%s/instances/%s", service, instance)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	return recorder, request
}

func (s *ServiceInstanceSuite) TestRemoveServiceInstanceNotFound(c *check.C) {
	se := service.Service{Name: "foo", Endpoint: map[string]string{"production": "http://localhost:1234"}, Password: "abcde", OwnerTeams: []string{s.team.Name}}
	err := se.Create()
	c.Assert(err, check.IsNil)
	recorder, request := makeRequestToRemoveServiceInstance("foo", "not-found", c)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
}

func (s *ServiceInstanceSuite) TestRemoveServiceServiceInstance(c *check.C) {
	requestIDHeader := "RequestID"
	config.Set("request-id-header", requestIDHeader)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.Assert(r.Header.Get(requestIDHeader), check.Equals, "test")
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	se := service.Service{Name: "foo", Endpoint: map[string]string{"production": ts.URL}, Password: "abcde", OwnerTeams: []string{s.team.Name}}
	err := se.Create()
	c.Assert(err, check.IsNil)
	si := service.ServiceInstance{Name: "foo-instance", ServiceName: "foo", Teams: []string{s.team.Name}}
	err = s.conn.ServiceInstances().Insert(si)
	c.Assert(err, check.IsNil)
	recorder, request := makeRequestToRemoveServiceInstance("foo", "foo-instance", c)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set(requestIDHeader, "test")
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	var msg io.SimpleJsonMessage
	json.Unmarshal(recorder.Body.Bytes(), &msg)
	c.Assert(msg.Message, check.Equals, "service instance successfully removed\n")
	n, err := s.conn.ServiceInstances().Find(bson.M{"name": "foo-instance", "service_name": "foo"}).Count()
	c.Assert(err, check.IsNil)
	c.Assert(n, check.Equals, 0)
	c.Assert(eventtest.EventDesc{
		Target: serviceInstanceTarget("foo", "foo-instance"),
		Owner:  s.token.GetUserName(),
		Kind:   "service-instance.delete",
		StartCustomData: []map[string]interface{}{
			{"name": ":service", "value": "foo"},
			{"name": ":instance", "value": "foo-instance"},
		},
	}, eventtest.HasEvent)
}

func (s *ServiceInstanceSuite) TestRemoveServiceInstanceWithSameInstaceName(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	services := []service.Service{
		{Name: "foo", Endpoint: map[string]string{"production": ts.URL}, Password: "abcde", OwnerTeams: []string{s.team.Name}},
		{Name: "foo2", Endpoint: map[string]string{"production": ts.URL}, Password: "abcde", OwnerTeams: []string{s.team.Name}},
	}
	for _, service := range services {
		err := service.Create()
		c.Assert(err, check.IsNil)
	}
	p := appTypes.Platform{Name: "zend"}
	app.PlatformService().Insert(p)
	s.pool = "test1"
	opts := pool.AddPoolOptions{Name: "test1", Default: true}
	err := pool.AddPool(opts)
	c.Assert(err, check.IsNil)
	a := app.App{
		Name:      "app-instance",
		Platform:  "zend",
		TeamOwner: s.team.Name,
	}
	err = app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	err = s.provisioner.AddUnits(&a, 1, "web", nil)
	c.Assert(err, check.IsNil)
	units, err := s.provisioner.Units(&a)
	c.Assert(err, check.IsNil)
	si := []service.ServiceInstance{
		{
			Name:        "foo-instance",
			ServiceName: "foo",
			Teams:       []string{s.team.Name},
			Apps:        []string{"app-instance"},
			BoundUnits:  []service.Unit{{ID: units[0].ID, IP: units[0].IP}},
		},
		{
			Name:        "foo-instance",
			ServiceName: "foo2",
			Teams:       []string{s.team.Name},
			Apps:        []string{},
		},
	}
	for _, instance := range si {
		err = s.conn.ServiceInstances().Insert(instance)
		c.Assert(err, check.IsNil)
	}
	recorder, request := makeRequestToRemoveServiceInstance("foo2", "foo-instance", c)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	expected := ""
	expected += `{"Message":"service instance successfully removed\n"}` + "\n"
	c.Assert(recorder.Body.String(), check.Equals, expected)
	var result []service.ServiceInstance
	n, err := s.conn.ServiceInstances().Find(bson.M{"name": "foo-instance", "service_name": "foo2"}).Count()
	c.Assert(err, check.IsNil)
	c.Assert(n, check.Equals, 0)
	err = s.conn.ServiceInstances().Find(bson.M{"name": "foo-instance", "service_name": "foo"}).All(&result)
	c.Assert(err, check.IsNil)
	c.Assert(result, check.HasLen, 1)
	c.Assert(result[0].Apps, check.DeepEquals, []string{"app-instance"})
	recorder, request = makeRequestToRemoveServiceInstanceWithUnbind("foo", "foo-instance", c)
	err = removeServiceInstance(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	expected = ""
	expected += `{"Message":"Unbind app \"app-instance\" ...\n"}` + "\n"
	expected += `{"Message":"\nInstance \"foo-instance\" is not bound to the app \"app-instance\" anymore.\n"}` + "\n"
	expected += `{"Message":"service instance successfully removed\n"}` + "\n"
	c.Assert(recorder.Body.String(), check.Equals, expected)
	n, err = s.conn.ServiceInstances().Find(bson.M{"name": "foo-instance", "service_name": "foo"}).Count()
	c.Assert(err, check.IsNil)
	c.Assert(n, check.Equals, 0)
}

func (s *ServiceInstanceSuite) TestRemoveServiceInstanceWithoutPermissionShouldReturn401(c *check.C) {
	se := service.Service{Name: "foo-service", Endpoint: map[string]string{"production": "http://localhost:1234"}, Password: "abcde", OwnerTeams: []string{s.team.Name}}
	err := se.Create()
	c.Assert(err, check.IsNil)
	si := service.ServiceInstance{Name: "foo-instance", ServiceName: "foo-service"}
	err = s.conn.ServiceInstances().Insert(si)
	c.Assert(err, check.IsNil)
	recorder, request := makeRequestToRemoveServiceInstance("foo-service", "foo-instance", c)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}

func (s *ServiceInstanceSuite) TestRemoveServiceInstanceWithAssociatedAppsShouldFailAndReturnError(c *check.C) {
	se := service.Service{Name: "foo", Endpoint: map[string]string{"production": "http://localhost:1234"}, Password: "abcde", OwnerTeams: []string{s.team.Name}}
	err := se.Create()
	c.Assert(err, check.IsNil)
	si := service.ServiceInstance{Name: "foo-instance", ServiceName: "foo", Apps: []string{"foo-bar"}, Teams: []string{s.team.Name}}
	err = s.conn.ServiceInstances().Insert(si)
	c.Assert(err, check.IsNil)
	recorder, request := makeRequestToRemoveServiceInstance("foo", "foo-instance", c)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Equals, "Applications bound to the service \"foo-instance\": \"foo-bar\"\n: This service instance is bound to at least one app. Unbind them before removing it\n")
}

func makeRequestToRemoveServiceInstanceWithUnbind(service, instance string, c *check.C) (*httptest.ResponseRecorder, *http.Request) {
	url := fmt.Sprintf("/services/%s/instances/%s?:service=%s&:instance=%s&unbindall=%s", service, instance, service, instance, "true")
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	return recorder, request
}

func (s *ServiceInstanceSuite) TestRemoveServiceInstanceWIthAssociatedAppsWithUnbindAll(c *check.C) {
	err := s.conn.Services().RemoveId(s.service.Name)
	c.Assert(err, check.IsNil)
	var called int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" && r.URL.Path == "/resources/my-mysql/bind" {
			atomic.StoreInt32(&called, 1)
		}
	}))
	defer ts.Close()
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}, Password: "abcde", OwnerTeams: []string{s.team.Name}}
	err = srvc.Create()
	c.Assert(err, check.IsNil)
	p := appTypes.Platform{Name: "zend"}
	app.PlatformService().Insert(p)
	s.pool = "test1"
	opts := pool.AddPoolOptions{Name: "test1", Default: true}
	err = pool.AddPool(opts)
	c.Assert(err, check.IsNil)
	a := app.App{
		Name:      "painkiller",
		Platform:  "zend",
		TeamOwner: s.team.Name,
	}
	err = app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	err = s.provisioner.AddUnits(&a, 1, "web", nil)
	c.Assert(err, check.IsNil)
	units, err := s.provisioner.Units(&a)
	c.Assert(err, check.IsNil)
	instance := service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
		Apps:        []string{"painkiller"},
		BoundUnits:  []service.Unit{{ID: units[0].ID, IP: units[0].IP}},
	}
	err = s.conn.ServiceInstances().Insert(instance)
	c.Assert(err, check.IsNil)
	recorder, request := makeRequestToRemoveServiceInstanceWithUnbind("mysql", "my-mysql", c)
	err = removeServiceInstance(recorder, request, s.token)
	c.Assert(err, check.IsNil)
}

func makeRequestToRemoveServiceInstanceWithNoUnbind(service, instance string, c *check.C) (*httptest.ResponseRecorder, *http.Request) {
	url := fmt.Sprintf("/services/%s/instances/%s?:service=%s&:instance=%s&unbindall=%s", service, instance, service, instance, "false")
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	return recorder, request
}

func (s *ServiceInstanceSuite) TestRemoveServiceInstanceWIthAssociatedAppsWithNoUnbindAll(c *check.C) {
	var called int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" && r.URL.Path == "/resources/my-mysql/bind" {
			atomic.StoreInt32(&called, 1)
		}
	}))
	defer ts.Close()
	srvc := service.Service{Name: "mysqlremove", Endpoint: map[string]string{"production": ts.URL}, Password: "abcde", OwnerTeams: []string{s.team.Name}}
	err := srvc.Create()
	c.Assert(err, check.IsNil)
	p := appTypes.Platform{Name: "zend"}
	app.PlatformService().Insert(p)
	s.pool = "test1"
	opts := pool.AddPoolOptions{Name: "test1", Default: true}
	err = pool.AddPool(opts)
	c.Assert(err, check.IsNil)
	a := app.App{
		Name:      "app1",
		Platform:  "zend",
		TeamOwner: s.team.Name,
	}
	err = app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	err = s.provisioner.AddUnits(&a, 1, "web", nil)
	c.Assert(err, check.IsNil)
	units, err := s.provisioner.Units(&a)
	c.Assert(err, check.IsNil)
	instance := service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysqlremove",
		Teams:       []string{s.team.Name},
		Apps:        []string{"app1"},
		BoundUnits:  []service.Unit{{ID: units[0].ID, IP: units[0].IP}},
	}
	err = s.conn.ServiceInstances().Insert(instance)
	c.Assert(err, check.IsNil)
	recorder, request := makeRequestToRemoveServiceInstanceWithNoUnbind("mysqlremove", "my-mysql", c)
	err = removeServiceInstance(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Applications bound to the service \"my-mysql\": \"app1\"\n: This service instance is bound to at least one app. Unbind them before removing it")
}

func (s *ServiceInstanceSuite) TestRemoveServiceInstanceWIthAssociatedAppsWithNoUnbindAllListAllApp(c *check.C) {
	var called int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" && r.URL.Path == "/resources/my-mysql/bind" {
			atomic.StoreInt32(&called, 1)
		}
	}))
	defer ts.Close()
	srvc := service.Service{Name: "mysqlremove", Endpoint: map[string]string{"production": ts.URL}, Password: "abcde", OwnerTeams: []string{s.team.Name}}
	err := srvc.Create()
	c.Assert(err, check.IsNil)
	p := appTypes.Platform{Name: "zend"}
	app.PlatformService().Insert(p)
	s.pool = "test1"
	opts := pool.AddPoolOptions{Name: "test1", Default: true}
	err = pool.AddPool(opts)
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
	err = s.provisioner.AddUnits(&a, 1, "web", nil)
	c.Assert(err, check.IsNil)
	err = s.provisioner.AddUnits(&ab, 1, "web", nil)
	c.Assert(err, check.IsNil)
	units, err := s.provisioner.Units(&ab)
	c.Assert(err, check.IsNil)
	instance := service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysqlremove",
		Teams:       []string{s.team.Name},
		Apps:        []string{"app", "app2"},
		BoundUnits:  []service.Unit{{ID: units[0].ID, IP: units[0].IP}},
	}
	err = s.conn.ServiceInstances().Insert(instance)
	c.Assert(err, check.IsNil)
	recorder, request := makeRequestToRemoveServiceInstanceWithNoUnbind("mysqlremove", "my-mysql", c)
	err = removeServiceInstance(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Applications bound to the service \"my-mysql\": \"app,app2\"\n: This service instance is bound to at least one app. Unbind them before removing it")
}

func (s *ServiceInstanceSuite) TestRemoveServiceShouldCallTheServiceAPI(c *check.C) {
	var called bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = r.Method == "DELETE" && r.URL.Path == "/resources/purity-instance"
	}))
	defer ts.Close()
	se := service.Service{Name: "purity", Endpoint: map[string]string{"production": ts.URL}, Password: "abcde", OwnerTeams: []string{s.team.Name}}
	err := se.Create()
	c.Assert(err, check.IsNil)
	si := service.ServiceInstance{Name: "purity-instance", ServiceName: "purity", Teams: []string{s.team.Name}}
	err = s.conn.ServiceInstances().Insert(si)
	c.Assert(err, check.IsNil)
	recorder, request := makeRequestToRemoveServiceInstance("purity", "purity-instance", c)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(called, check.Equals, true)
}

type ServiceModelList []service.ServiceModel

func (l ServiceModelList) Len() int           { return len(l) }
func (l ServiceModelList) Swap(i, j int)      { l[i], l[j] = l[j], l[i] }
func (l ServiceModelList) Less(i, j int) bool { return l[i].Service < l[j].Service }

func (s *ServiceInstanceSuite) TestListServiceInstances(c *check.C) {
	err := s.conn.Services().RemoveId(s.service.Name)
	c.Assert(err, check.IsNil)
	srv := service.Service{
		Name:       "redis",
		Teams:      []string{s.team.Name},
		OwnerTeams: []string{s.team.Name},
		Endpoint:   map[string]string{"production": "http://localhost:1234"},
		Password:   "abcde",
	}
	err = srv.Create()
	c.Assert(err, check.IsNil)
	srv2 := service.Service{
		Name:       "mongodb",
		Teams:      []string{s.team.Name},
		OwnerTeams: []string{s.team.Name},
		Endpoint:   map[string]string{"production": "http://localhost:1234"},
		Password:   "abcde",
	}
	err = srv2.Create()
	c.Assert(err, check.IsNil)
	instance := service.ServiceInstance{
		Name:        "redis-globo",
		ServiceName: "redis",
		Apps:        []string{"globo"},
		Teams:       []string{s.team.Name},
	}
	err = s.conn.ServiceInstances().Insert(instance)
	c.Assert(err, check.IsNil)
	instance2 := service.ServiceInstance{
		Name:        "mongodb-other",
		ServiceName: "mongodb",
		Apps:        []string{"other"},
		Teams:       []string{s.team.Name},
	}
	err = s.conn.ServiceInstances().Insert(instance2)
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
	c.Assert(instances, check.DeepEquals, expected)
}

func (s *ServiceInstanceSuite) TestListServiceInstancesAppFilter(c *check.C) {
	err := s.conn.Services().RemoveId(s.service.Name)
	c.Assert(err, check.IsNil)
	srv := service.Service{
		Name:       "redis",
		Teams:      []string{s.team.Name},
		OwnerTeams: []string{s.team.Name},
		Endpoint:   map[string]string{"production": "http://localhost:1234"},
		Password:   "abcde",
	}
	err = srv.Create()
	c.Assert(err, check.IsNil)
	srv2 := service.Service{
		Name:       "mongodb",
		Teams:      []string{s.team.Name},
		OwnerTeams: []string{s.team.Name},
		Endpoint:   map[string]string{"production": "http://localhost:1234"},
		Password:   "abcde",
	}
	err = srv2.Create()
	c.Assert(err, check.IsNil)
	instance := service.ServiceInstance{
		Name:        "redis-globo",
		ServiceName: "redis",
		Apps:        []string{"globo"},
		Teams:       []string{s.team.Name},
	}
	err = s.conn.ServiceInstances().Insert(instance)
	c.Assert(err, check.IsNil)
	instance2 := service.ServiceInstance{
		Name:        "mongodb-other",
		ServiceName: "mongodb",
		Apps:        []string{"other"},
		Teams:       []string{s.team.Name},
	}
	err = s.conn.ServiceInstances().Insert(instance2)
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

func (s *ServiceInstanceSuite) TestListServiceInstancesReturnsOnlyServicesThatTheUserHasAccess(c *check.C) {
	err := s.conn.Services().RemoveId(s.service.Name)
	c.Assert(err, check.IsNil)
	u := &auth.User{Email: "me@globo.com", Password: "123456"}
	_, err = nativeScheme.Create(u)
	c.Assert(err, check.IsNil)
	srv := service.Service{Name: "redis", IsRestricted: true, Endpoint: map[string]string{"production": "http://localhost:1234"}, Password: "abcde", OwnerTeams: []string{s.team.Name}}
	err = s.conn.Services().Insert(srv)
	c.Assert(err, check.IsNil)
	instance := service.ServiceInstance{
		Name:        "redis-globo",
		ServiceName: "redis",
		Apps:        []string{"globo"},
	}
	err = s.conn.ServiceInstances().Insert(instance)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("GET", "/services/instances", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNoContent)
}

func (s *ServiceInstanceSuite) TestListServiceInstancesFilterInstancesPerServiceIncludingServicesThatDoesNotHaveInstances(c *check.C) {
	serviceNames := []string{"redis", "pgsql", "memcached"}
	for _, name := range serviceNames {
		srv := service.Service{
			Name:       name,
			Teams:      []string{s.team.Name},
			OwnerTeams: []string{s.team.Name},
			Endpoint:   map[string]string{"production": "http://localhost:1234"},
			Password:   "abcde",
		}
		err := srv.Create()
		c.Assert(err, check.IsNil)
		instance := service.ServiceInstance{
			Name:        srv.Name + "1",
			ServiceName: srv.Name,
			Teams:       []string{s.team.Name},
		}
		err = s.conn.ServiceInstances().Insert(instance)
		c.Assert(err, check.IsNil)
		instance = service.ServiceInstance{
			Name:        srv.Name + "2",
			ServiceName: srv.Name,
			Teams:       []string{s.team.Name},
		}
		err = s.conn.ServiceInstances().Insert(instance)
		c.Assert(err, check.IsNil)
	}
	srv := service.Service{
		Name:       "oracle",
		Teams:      []string{s.team.Name},
		OwnerTeams: []string{s.team.Name},
		Endpoint:   map[string]string{"production": "http://localhost:1234"},
		Password:   "abcde",
	}
	err := srv.Create()
	c.Assert(err, check.IsNil)
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

func makeRequestToServiceInstanceStatus(service string, instance string, c *check.C) (*httptest.ResponseRecorder, *http.Request) {
	url := fmt.Sprintf("/services/%s/instances/%s/status/?:instance=%s&:service=%s", service, instance, instance, service)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	return recorder, request
}

func (s *ServiceInstanceSuite) TestServiceInstanceStatus(c *check.C) {
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
	srv := service.Service{
		Name:       "mongodb",
		OwnerTeams: []string{s.team.Name},
		Endpoint:   map[string]string{"production": ts.URL},
		Password:   "abcde",
	}
	err := srv.Create()
	c.Assert(err, check.IsNil)
	si := service.ServiceInstance{Name: "my_nosql", ServiceName: srv.Name, Teams: []string{s.team.Name}}
	err = s.conn.ServiceInstances().Insert(si)
	c.Assert(err, check.IsNil)
	recorder, request := makeRequestToServiceInstanceStatus("mongodb", "my_nosql", c)
	context.SetRequestID(request, requestIDHeader, "test")
	err = serviceInstanceStatus(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Body.String(), check.Equals, "Service instance \"my_nosql\" is up")
}

func (s *ServiceInstanceSuite) TestServiceInstanceStatusWithSameInstanceName(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
		w.Write([]byte(`Service instance "my_nosql" is up`))
	}))
	ts1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`Service instance "my_nosql" is down`))
	}))

	defer ts.Close()
	srv := service.Service{
		Name:       "mongodb",
		OwnerTeams: []string{s.team.Name},
		Endpoint:   map[string]string{"production": ts.URL},
		Password:   "abcde",
	}
	err := srv.Create()
	c.Assert(err, check.IsNil)
	srv2 := service.Service{
		Name:       "mongodb2",
		OwnerTeams: []string{s.team.Name},
		Endpoint:   map[string]string{"production": ts1.URL},
		Password:   "abcde",
	}
	err = srv2.Create()
	c.Assert(err, check.IsNil)
	si := service.ServiceInstance{Name: "my_nosql", ServiceName: srv.Name, Teams: []string{s.team.Name}}
	err = s.conn.ServiceInstances().Insert(si)
	c.Assert(err, check.IsNil)
	si2 := service.ServiceInstance{Name: "my_nosql", ServiceName: srv2.Name, Teams: []string{s.team.Name}}
	err = s.conn.ServiceInstances().Insert(si2)
	c.Assert(err, check.IsNil)
	recorder, request := makeRequestToServiceInstanceStatus("mongodb2", "my_nosql", c)
	err = serviceInstanceStatus(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Body.String(), check.Equals, "Service instance \"my_nosql\" is down")
}

func (s *ServiceInstanceSuite) TestServiceInstanceStatusShouldReturnErrorWhenServiceInstanceNotExists(c *check.C) {
	recorder, request := makeRequestToServiceInstanceStatus("service", "inexistent-instance", c)
	err := serviceInstanceStatus(recorder, request, s.token)
	c.Assert(err, check.ErrorMatches, "^service instance not found$")
}

func (s *ServiceInstanceSuite) TestServiceInstanceStatusShouldReturnForbiddenWhenUserDontHaveAccess(c *check.C) {
	srv := service.Service{
		Name:       "mongodb",
		OwnerTeams: []string{s.team.Name},
		Endpoint:   map[string]string{"production": "http://localhost:1234"},
		Password:   "abcde",
	}
	err := srv.Create()
	c.Assert(err, check.IsNil)
	si := service.ServiceInstance{Name: "my_nosql", ServiceName: srv.Name}
	err = s.conn.ServiceInstances().Insert(si)
	c.Assert(err, check.IsNil)
	recorder, request := makeRequestToServiceInstanceStatus("mongodb", "my_nosql", c)
	err = serviceInstanceStatus(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusForbidden)
}

func makeRequestToServiceInstanceInfo(service, instance, token string, c *check.C) (*httptest.ResponseRecorder, *http.Request) {
	url := fmt.Sprintf("/services/%s/instances/%s", service, instance)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+token)
	recorder := httptest.NewRecorder()
	return recorder, request
}

func (s *ServiceInstanceSuite) TestServiceInstanceInfo(c *check.C) {
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
	srv := service.Service{
		Name:       "mongodb",
		Teams:      []string{s.team.Name},
		OwnerTeams: []string{s.team.Name},
		Endpoint:   map[string]string{"production": ts.URL},
		Password:   "abcde",
	}
	err := srv.Create()
	c.Assert(err, check.IsNil)
	si := service.ServiceInstance{
		Name:        "my_nosql",
		ServiceName: srv.Name,
		Apps:        []string{"app1", "app2"},
		Teams:       []string{s.team.Name},
		TeamOwner:   s.team.Name,
		PlanName:    "small",
		Description: "desc",
		Tags:        []string{"tag 1"},
	}
	err = s.conn.ServiceInstances().Insert(si)
	c.Assert(err, check.IsNil)
	recorder, request := makeRequestToServiceInstanceInfo("mongodb", "my_nosql", s.token.GetValue(), c)
	request.Header.Set(requestIDHeader, "test")
	s.testServer.ServeHTTP(recorder, request)
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
		Tags:            []string{"tag 1"},
	}
	c.Assert(instances, check.DeepEquals, expected)
}

func (s *ServiceInstanceSuite) TestServiceInstanceInfoNoPlanAndNoCustomInfo(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[]`))
	}))
	defer ts.Close()
	srv := service.Service{
		Name:       "mongodb",
		Teams:      []string{s.team.Name},
		OwnerTeams: []string{s.team.Name},
		Endpoint:   map[string]string{"production": ts.URL},
		Password:   "abcde",
	}
	err := srv.Create()
	c.Assert(err, check.IsNil)
	si := service.ServiceInstance{
		Name:        "my_nosql",
		ServiceName: srv.Name,
		Apps:        []string{"app1", "app2"},
		Teams:       []string{s.team.Name},
		TeamOwner:   s.team.Name,
		Tags:        []string{"tag 1", "tag 2"},
	}
	err = s.conn.ServiceInstances().Insert(si)
	c.Assert(err, check.IsNil)
	recorder, request := makeRequestToServiceInstanceInfo("mongodb", "my_nosql", s.token.GetValue(), c)
	s.testServer.ServeHTTP(recorder, request)
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
		Tags:            []string{"tag 1", "tag 2"},
	}
	c.Assert(instances, check.DeepEquals, expected)
}

func (s *ServiceInstanceSuite) TestServiceInstanceInfoShouldReturnErrorWhenServiceInstanceDoesNotExist(c *check.C) {
	recorder, request := makeRequestToServiceInstanceInfo("mongodb", "inexistent-instance", s.token.GetValue(), c)
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
}

func (s *ServiceInstanceSuite) TestServiceInstanceInfoShouldReturnForbiddenWhenUserDontHaveAccess(c *check.C) {
	srv := service.Service{
		Name:       "mongodb",
		OwnerTeams: []string{s.team.Name},
		Endpoint:   map[string]string{"production": "http://localhost:1234"},
		Password:   "abcde",
	}
	err := srv.Create()
	c.Assert(err, check.IsNil)
	si := service.ServiceInstance{Name: "my_nosql", ServiceName: srv.Name}
	err = s.conn.ServiceInstances().Insert(si)
	c.Assert(err, check.IsNil)
	recorder, request := makeRequestToServiceInstanceInfo("mongodb", "my_nosql", s.token.GetValue(), c)
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}

func (s *ServiceInstanceSuite) TestServiceInfo(c *check.C) {
	srv := service.Service{
		Name:       "mongodb",
		OwnerTeams: []string{s.team.Name},
		Endpoint:   map[string]string{"production": "http://localhost:1234"},
		Password:   "abcde",
	}
	err := srv.Create()
	c.Assert(err, check.IsNil)
	si1 := service.ServiceInstance{
		Name:        "my_nosql",
		ServiceName: srv.Name,
		Apps:        []string{},
		Teams:       []string{s.team.Name},
	}
	err = s.conn.ServiceInstances().Insert(si1)
	c.Assert(err, check.IsNil)
	si2 := service.ServiceInstance{
		Name:        "your_nosql",
		ServiceName: srv.Name,
		Apps:        []string{"wordpress"},
		Teams:       []string{s.team.Name},
	}
	err = s.conn.ServiceInstances().Insert(si2)
	c.Assert(err, check.IsNil)
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

func (s *ServiceInstanceSuite) TestServiceInfoShouldReturnOnlyInstancesOfTheSameTeamOfTheUser(c *check.C) {
	srv := service.Service{
		Name:       "mongodb",
		OwnerTeams: []string{s.team.Name},
		Endpoint:   map[string]string{"production": "http://localhost:1234"},
		Password:   "abcde",
	}
	err := srv.Create()
	c.Assert(err, check.IsNil)
	si1 := service.ServiceInstance{
		Name:        "my_nosql",
		ServiceName: srv.Name,
		Apps:        []string{},
		Teams:       []string{s.team.Name},
	}
	err = s.conn.ServiceInstances().Insert(si1)
	c.Assert(err, check.IsNil)
	si2 := service.ServiceInstance{
		Name:        "your_nosql",
		ServiceName: srv.Name,
		Apps:        []string{"wordpress"},
		Teams:       []string{},
	}
	err = s.conn.ServiceInstances().Insert(si2)
	c.Assert(err, check.IsNil)
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

func (s *ServiceInstanceSuite) TestServiceInfoReturns404WhenTheServiceDoesNotExist(c *check.C) {
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

func (s *ServiceInstanceSuite) makeRequestToGetServiceDoc(name string, c *check.C) (*httptest.ResponseRecorder, *http.Request) {
	url := fmt.Sprintf("/services/%s/doc/?:name=%s", name, name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	return recorder, request
}

func (s *ServiceInstanceSuite) TestServiceDoc(c *check.C) {
	doc := `Doc for coolnosql
Collnosql is a really really cool nosql`
	srv := service.Service{
		Name:       "coolnosql",
		Doc:        doc,
		Teams:      []string{s.team.Name},
		OwnerTeams: []string{s.team.Name},
		Endpoint:   map[string]string{"production": "http://localhost:1234"},
		Password:   "abcde",
	}
	err := srv.Create()
	c.Assert(err, check.IsNil)
	recorder, request := s.makeRequestToGetServiceDoc("coolnosql", c)
	err = serviceDoc(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Body.String(), check.Equals, doc)
}

func (s *ServiceInstanceSuite) TestServiceDocReturns401WhenUserHasNoAccessToService(c *check.C) {
	srv := service.Service{
		Name:         "coolnosql",
		Doc:          "some doc...",
		IsRestricted: true,
		Endpoint:     map[string]string{"production": "http://localhost:1234"},
		Password:     "abcde",
		OwnerTeams:   []string{s.team.Name},
	}
	err := srv.Create()
	c.Assert(err, check.IsNil)
	recorder, request := s.makeRequestToGetServiceDoc("coolnosql", c)
	err = serviceDoc(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusForbidden)
}

func (s *ServiceInstanceSuite) TestServiceDocReturns404WhenServiceDoesNotExists(c *check.C) {
	recorder, request := s.makeRequestToGetServiceDoc("inexistentsql", c)
	err := serviceDoc(recorder, request, s.token)
	c.Assert(err, check.ErrorMatches, "^Service not found$")
}

func (s *ServiceInstanceSuite) TestGetServiceInstanceOrError(c *check.C) {
	err := s.conn.Services().RemoveId(s.service.Name)
	c.Assert(err, check.IsNil)
	si := service.ServiceInstance{Name: "foo", ServiceName: "foo-service", Teams: []string{s.team.Name}}
	err = s.conn.ServiceInstances().Insert(si)
	c.Assert(err, check.IsNil)
	rSi, err := getServiceInstanceOrError("foo-service", "foo")
	c.Assert(err, check.IsNil)
	c.Assert(rSi.Name, check.Equals, si.Name)
}

func (s *ServiceInstanceSuite) TestServicePlans(c *check.C) {
	requestIDHeader := "RequestID"
	config.Set("request-id-header", requestIDHeader)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.Assert(r.Header.Get(requestIDHeader), check.Equals, "test")
		content := `[{"name": "ignite", "description": "some value"}, {"name": "small", "description": "not space left for you"}]`
		w.Write([]byte(content))
	}))
	defer ts.Close()
	srvc := service.Service{Name: "mysqlplan", Endpoint: map[string]string{"production": ts.URL}, Password: "abcde", OwnerTeams: []string{s.team.Name}}
	err := srvc.Create()
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("GET", "/services/mysqlplan/plans", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	request.Header.Set(requestIDHeader, "test")
	s.testServer.ServeHTTP(recorder, request)
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

func (s *ServiceInstanceSuite) TestServiceInstanceProxy(c *check.C) {
	var proxyedRequest *http.Request
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyedRequest = r
		w.Header().Set("X-Response-Custom", "custom response header")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte("a message"))
	}))
	defer ts.Close()
	se := service.Service{Name: "foo", Endpoint: map[string]string{"production": ts.URL}, Password: "abcde", OwnerTeams: []string{s.team.Name}}
	err := se.Create()
	c.Assert(err, check.IsNil)
	si := service.ServiceInstance{Name: "foo-instance", ServiceName: "foo", Teams: []string{s.team.Name}}
	err = s.conn.ServiceInstances().Insert(si)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/services/%s/proxy/%s?callback=/resources/foo-instance/mypath", si.ServiceName, si.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	reqAuth := "bearer " + s.token.GetValue()
	request.Header.Set("Authorization", reqAuth)
	request.Header.Set("X-Custom", "my request header")
	recorder := &closeNotifierResponseRecorder{httptest.NewRecorder()}
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
	c.Assert(recorder.Header().Get("X-Response-Custom"), check.Equals, "custom response header")
	c.Assert(recorder.Body.String(), check.Equals, "a message")
	c.Assert(proxyedRequest, check.NotNil)
	c.Assert(proxyedRequest.Header.Get("X-Custom"), check.Equals, "my request header")
	c.Assert(proxyedRequest.Header.Get("Authorization"), check.Not(check.Equals), reqAuth)
	c.Assert(proxyedRequest.URL.String(), check.Equals, "/resources/foo-instance/mypath")
	c.Assert(eventtest.EventDesc{
		IsEmpty: true,
	}, eventtest.HasEvent)
}

func (s *ServiceInstanceSuite) TestServiceInstanceProxyPost(c *check.C) {
	var (
		proxyedRequest *http.Request
		proxyedBody    []byte
	)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		proxyedBody, err = ioutil.ReadAll(r.Body)
		c.Assert(err, check.IsNil)
		proxyedRequest = r
		w.Header().Set("X-Response-Custom", "custom response header")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte("a message"))
	}))
	defer ts.Close()
	se := service.Service{Name: "foo", Endpoint: map[string]string{"production": ts.URL}, Password: "abcde", OwnerTeams: []string{s.team.Name}}
	err := se.Create()
	c.Assert(err, check.IsNil)
	si := service.ServiceInstance{Name: "foo-instance", ServiceName: "foo", Teams: []string{s.team.Name}}
	err = s.conn.ServiceInstances().Insert(si)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/services/%s/proxy/%s?callback=/resources/foo-instance/mypath", si.ServiceName, si.Name)
	body := strings.NewReader("my=awesome&body=1")
	request, err := http.NewRequest("POST", url, body)
	c.Assert(err, check.IsNil)
	reqAuth := "bearer " + s.token.GetValue()
	request.Header.Set("Authorization", reqAuth)
	request.Header.Set("X-Custom", "my request header")
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := &closeNotifierResponseRecorder{httptest.NewRecorder()}
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
	c.Assert(recorder.Header().Get("X-Response-Custom"), check.Equals, "custom response header")
	c.Assert(recorder.Body.String(), check.Equals, "a message")
	c.Assert(proxyedRequest, check.NotNil)
	c.Assert(proxyedRequest.Header.Get("X-Custom"), check.Equals, "my request header")
	c.Assert(proxyedRequest.Header.Get("Authorization"), check.Not(check.Equals), reqAuth)
	c.Assert(proxyedRequest.URL.String(), check.Equals, "/resources/foo-instance/mypath")
	c.Assert(string(proxyedBody), check.Equals, "my=awesome&body=1")
	c.Assert(eventtest.EventDesc{
		Target: serviceInstanceTarget("foo", "foo-instance"),
		Owner:  s.token.GetUserName(),
		Kind:   "service-instance.update.proxy",
		StartCustomData: []map[string]interface{}{
			{"name": "callback", "value": "/resources/foo-instance/mypath"},
			{"name": "method", "value": "POST"},
			{"name": "my", "value": "awesome"},
			{"name": "body", "value": "1"},
		},
	}, eventtest.HasEvent)
}

func (s *ServiceInstanceSuite) TestServiceInstanceProxyPostRawBody(c *check.C) {
	var (
		proxyedRequest *http.Request
		proxyedBody    []byte
	)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		proxyedBody, err = ioutil.ReadAll(r.Body)
		c.Assert(err, check.IsNil)
		proxyedRequest = r
		w.Header().Set("X-Response-Custom", "custom response header")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte("a message"))
	}))
	defer ts.Close()
	se := service.Service{Name: "foo", Endpoint: map[string]string{"production": ts.URL}, Password: "abcde", OwnerTeams: []string{s.team.Name}}
	err := se.Create()
	c.Assert(err, check.IsNil)
	si := service.ServiceInstance{Name: "foo-instance", ServiceName: "foo", Teams: []string{s.team.Name}}
	err = s.conn.ServiceInstances().Insert(si)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/services/%s/proxy/%s?callback=/resources/foo-instance/mypath", si.ServiceName, si.Name)
	body := strings.NewReader("something-something")
	request, err := http.NewRequest("POST", url, body)
	c.Assert(err, check.IsNil)
	reqAuth := "bearer " + s.token.GetValue()
	request.Header.Set("Authorization", reqAuth)
	request.Header.Set("X-Custom", "my request header")
	request.Header.Set("Content-Type", "text/plain")
	recorder := &closeNotifierResponseRecorder{httptest.NewRecorder()}
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
	c.Assert(recorder.Header().Get("X-Response-Custom"), check.Equals, "custom response header")
	c.Assert(recorder.Body.String(), check.Equals, "a message")
	c.Assert(proxyedRequest, check.NotNil)
	c.Assert(proxyedRequest.Header.Get("X-Custom"), check.Equals, "my request header")
	c.Assert(proxyedRequest.Header.Get("Authorization"), check.Not(check.Equals), reqAuth)
	c.Assert(proxyedRequest.URL.String(), check.Equals, "/resources/foo-instance/mypath")
	c.Assert(string(proxyedBody), check.Equals, "something-something")
	c.Assert(eventtest.EventDesc{
		Target: serviceInstanceTarget("foo", "foo-instance"),
		Owner:  s.token.GetUserName(),
		Kind:   "service-instance.update.proxy",
		StartCustomData: []map[string]interface{}{
			{"name": "callback", "value": "/resources/foo-instance/mypath"},
			{"name": "method", "value": "POST"},
		},
	}, eventtest.HasEvent)
}

func (s *ServiceInstanceSuite) TestServiceInstanceProxyNoContent(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()
	se := service.Service{Name: "foo", Endpoint: map[string]string{"production": ts.URL}, Password: "abcde", OwnerTeams: []string{s.team.Name}}
	err := se.Create()
	c.Assert(err, check.IsNil)
	si := service.ServiceInstance{Name: "foo-instance", ServiceName: "foo", Teams: []string{s.team.Name}}
	err = s.conn.ServiceInstances().Insert(si)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/services/%s/proxy/%s?callback=/resources/foo-instance/mypath", si.ServiceName, si.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	reqAuth := "bearer " + s.token.GetValue()
	request.Header.Set("Authorization", reqAuth)
	recorder := &closeNotifierResponseRecorder{httptest.NewRecorder()}
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNoContent)
}

func (s *ServiceInstanceSuite) TestServiceInstanceProxyError(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte("some error"))
	}))
	defer ts.Close()
	se := service.Service{Name: "foo", Endpoint: map[string]string{"production": ts.URL}, Password: "abcde", OwnerTeams: []string{s.team.Name}}
	err := se.Create()
	c.Assert(err, check.IsNil)
	si := service.ServiceInstance{Name: "foo-instance", ServiceName: "foo", Teams: []string{s.team.Name}}
	err = s.conn.ServiceInstances().Insert(si)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/services/%s/proxy/%s?callback=/resources/foo-instance/mypath", si.ServiceName, si.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	reqAuth := "bearer " + s.token.GetValue()
	request.Header.Set("Authorization", reqAuth)
	recorder := &closeNotifierResponseRecorder{httptest.NewRecorder()}
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadGateway)
	c.Assert(recorder.Body.Bytes(), check.DeepEquals, []byte("some error"))
}

func (s *ServiceInstanceSuite) TestServiceInstanceProxyOnlyPath(c *check.C) {
	var proxyedRequest *http.Request
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyedRequest = r
		w.WriteHeader(http.StatusCreated)
	}))
	defer ts.Close()
	se := service.Service{Name: "foo", Endpoint: map[string]string{"production": ts.URL}, Password: "abcde", OwnerTeams: []string{s.team.Name}}
	err := se.Create()
	c.Assert(err, check.IsNil)
	si := service.ServiceInstance{Name: "foo-instance", ServiceName: "foo", Teams: []string{s.team.Name}}
	err = s.conn.ServiceInstances().Insert(si)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/services/%s/proxy/%s?callback=/mypath", si.ServiceName, si.Name)
	request, err := http.NewRequest("POST", url, nil)
	c.Assert(err, check.IsNil)
	reqAuth := "bearer " + s.token.GetValue()
	request.Header.Set("Authorization", reqAuth)
	request.Header.Set("X-Custom", "my request header")
	recorder := &closeNotifierResponseRecorder{httptest.NewRecorder()}
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
	c.Assert(proxyedRequest, check.NotNil)
	c.Assert(proxyedRequest.Header.Get("X-Custom"), check.Equals, "my request header")
	c.Assert(proxyedRequest.Header.Get("Authorization"), check.Not(check.Equals), reqAuth)
	c.Assert(proxyedRequest.URL.String(), check.Equals, "/resources/foo-instance/mypath")
	c.Assert(eventtest.EventDesc{
		Target: serviceInstanceTarget("foo", "foo-instance"),
		Owner:  s.token.GetUserName(),
		Kind:   "service-instance.update.proxy",
		StartCustomData: []map[string]interface{}{
			{"name": "callback", "value": "/mypath"},
			{"name": "method", "value": "POST"},
		},
	}, eventtest.HasEvent)
}

func (s *ServiceInstanceSuite) TestServiceInstanceProxyForbiddenPath(c *check.C) {
	var proxyedRequest *http.Request
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyedRequest = r
		w.WriteHeader(http.StatusCreated)
	}))
	defer ts.Close()
	se := service.Service{Name: "foo", Endpoint: map[string]string{"production": ts.URL}, Password: "abcde", OwnerTeams: []string{s.team.Name}}
	err := se.Create()
	c.Assert(err, check.IsNil)
	si := service.ServiceInstance{Name: "foo-instance", ServiceName: "foo", Teams: []string{s.team.Name}}
	err = s.conn.ServiceInstances().Insert(si)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/services/%s/proxy/%s?callback=/", si.ServiceName, si.Name)
	request, err := http.NewRequest("POST", url, nil)
	c.Assert(err, check.IsNil)
	reqAuth := "bearer " + s.token.GetValue()
	request.Header.Set("Authorization", reqAuth)
	request.Header.Set("X-Custom", "my request header")
	recorder := &closeNotifierResponseRecorder{httptest.NewRecorder()}
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Equals, "proxy request POST \"\" is forbidden\n")
	c.Assert(proxyedRequest, check.IsNil)
	c.Assert(eventtest.EventDesc{
		Target: serviceInstanceTarget("foo", "foo-instance"),
		Owner:  s.token.GetUserName(),
		Kind:   "service-instance.update.proxy",
		StartCustomData: []map[string]interface{}{
			{"name": "callback", "value": "/"},
			{"name": "method", "value": "POST"},
		},
		ErrorMatches: "proxy request POST \"\" is forbidden",
	}, eventtest.HasEvent)
}

func (s *ServiceInstanceSuite) TestGrantRevokeServiceToTeam(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("{'AA': 2}"))
	}))
	defer ts.Close()
	se := service.Service{Name: "go", Endpoint: map[string]string{"production": ts.URL}, Password: "abcde", OwnerTeams: []string{s.team.Name}}
	err := se.Create()
	c.Assert(err, check.IsNil)
	si := service.ServiceInstance{Name: "si-test", ServiceName: "go", Teams: []string{s.team.Name}}
	err = s.conn.ServiceInstances().Insert(si)
	c.Assert(err, check.IsNil)
	teamName := "test"
	url := fmt.Sprintf("/services/%s/instances/permission/%s/%s?:instance=%s&:team=%s&:service=%s", si.ServiceName, si.Name,
		teamName, si.Name, teamName, si.ServiceName)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = serviceInstanceGrantTeam(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	c.Assert(eventtest.EventDesc{
		Target: serviceInstanceTarget("go", "si-test"),
		Owner:  s.token.GetUserName(),
		Kind:   "service-instance.update.grant",
		StartCustomData: []map[string]interface{}{
			{"name": ":team", "value": "test"},
		},
	}, eventtest.HasEvent)
	sinst, err := service.GetServiceInstance(si.ServiceName, si.Name)
	c.Assert(err, check.IsNil)
	c.Assert(sinst.Teams, check.DeepEquals, []string{s.team.Name, teamName})
	request, err = http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	err = serviceInstanceRevokeTeam(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	sinst, err = service.GetServiceInstance(si.ServiceName, si.Name)
	c.Assert(err, check.IsNil)
	c.Assert(sinst.Teams, check.DeepEquals, []string{s.team.Name})
	c.Assert(eventtest.EventDesc{
		Target: serviceInstanceTarget("go", "si-test"),
		Owner:  s.token.GetUserName(),
		Kind:   "service-instance.update.revoke",
		StartCustomData: []map[string]interface{}{
			{"name": ":team", "value": "test"},
		},
	}, eventtest.HasEvent)
}

func (s *ServiceInstanceSuite) TestGrantRevokeServiceToTeamWithManyInstanceName(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("{'AA': 2}"))
	}))
	defer ts.Close()
	se := []service.Service{
		{Name: "go", Endpoint: map[string]string{"production": ts.URL}, Password: "abcde", OwnerTeams: []string{s.team.Name}},
		{Name: "go2", Endpoint: map[string]string{"production": ts.URL}, Password: "abcde", OwnerTeams: []string{s.team.Name}},
	}
	for _, service := range se {
		err := service.Create()
		c.Assert(err, check.IsNil)
	}
	si := service.ServiceInstance{Name: "si-test", ServiceName: se[0].Name, Teams: []string{s.team.Name}}
	err := s.conn.ServiceInstances().Insert(si)
	c.Assert(err, check.IsNil)
	si2 := service.ServiceInstance{Name: "si-test", ServiceName: se[1].Name, Teams: []string{s.team.Name}}
	err = s.conn.ServiceInstances().Insert(si2)
	c.Assert(err, check.IsNil)
	teamName := "test"
	url := fmt.Sprintf("/services/%s/instances/permission/%s/%s?:instance=%s&:team=%s&:service=%s", si2.ServiceName, si2.Name,
		teamName, si2.Name, teamName, si2.ServiceName)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = serviceInstanceGrantTeam(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	sinst, err := service.GetServiceInstance(si2.ServiceName, si2.Name)
	c.Assert(err, check.IsNil)
	c.Assert(sinst.Teams, check.DeepEquals, []string{s.team.Name, teamName})
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
