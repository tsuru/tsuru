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
	"strings"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/event/eventtest"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/permission/permissiontest"
	"github.com/tsuru/tsuru/repository/repositorytest"
	"github.com/tsuru/tsuru/service"
	_ "github.com/tsuru/tsuru/storage/mongodb"
	authTypes "github.com/tsuru/tsuru/types/auth"
	serviceTypes "github.com/tsuru/tsuru/types/service"
	"golang.org/x/crypto/bcrypt"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

type ProvisionSuite struct {
	conn       *db.Storage
	team       *authTypes.Team
	user       *auth.User
	token      auth.Token
	testServer http.Handler
}

var _ = check.Suite(&ProvisionSuite{})

func (s *ProvisionSuite) SetUpTest(c *check.C) {
	app.AuthScheme = nativeScheme
	repositorytest.Reset()
	var err error
	config.Set("database:driver", "mongodb")
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "tsuru_api_service_test")
	config.Set("auth:hash-cost", bcrypt.MinCost)
	config.Set("repo-manager", "fake")
	s.conn, err = db.Conn()
	c.Assert(err, check.IsNil)
	dbtest.ClearAllCollections(s.conn.Apps().Database)
	s.createUserAndTeam(c)
	s.testServer = RunServer(true)
}

func (s *ProvisionSuite) TearDownTest(c *check.C) {
	s.conn.Close()
}

func (s *ProvisionSuite) TearDownSuite(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	conn.Apps().Database.DropDatabase()
}

func (s *ProvisionSuite) makeRequestToServicesHandler(c *check.C) (*httptest.ResponseRecorder, *http.Request) {
	recorder, request := s.makeRequest("GET", "/services", "", c)
	return recorder, request
}

func (s *ProvisionSuite) createUserAndTeam(c *check.C) {
	s.team = &authTypes.Team{Name: "tsuruteam"}
	err := serviceTypes.Team().Insert(*s.team)
	c.Assert(err, check.IsNil)
	_, s.token = permissiontest.CustomUserWithPermission(c, nativeScheme, "provision-master-user", permission.Permission{
		Scheme:  permission.PermService,
		Context: permission.Context(permission.CtxTeam, s.team.Name),
	})
	s.user, err = s.token.User()
	c.Assert(err, check.IsNil)
}

func (s *ProvisionSuite) TestServiceListGetAllServicesFromUsersTeam(c *check.C) {
	srv := service.Service{
		Name:       "mongodb",
		OwnerTeams: []string{s.team.Name},
		Endpoint:   map[string]string{"production": "http://localhost:1234"},
		Password:   "abcde",
	}
	err := srv.Create()
	c.Assert(err, check.IsNil)
	si := service.ServiceInstance{Name: "my_nosql", ServiceName: srv.Name, Teams: []string{s.team.Name}, Tags: []string{"tag 1"}}
	err = s.conn.ServiceInstances().Insert(si)
	c.Assert(err, check.IsNil)
	recorder, request := s.makeRequestToServicesHandler(c)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	services := make([]service.ServiceModel, 1)
	err = json.Unmarshal(recorder.Body.Bytes(), &services)
	c.Assert(err, check.IsNil)
	expected := []service.ServiceModel{{
		Service:   "mongodb",
		Instances: []string{"my_nosql"},
		ServiceInstances: []service.ServiceInstanceModel{
			{Name: "my_nosql", Tags: []string{"tag 1"}},
		},
	}}
	c.Assert(services, check.DeepEquals, expected)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
}

func (s *ProvisionSuite) TestServiceListEmptyList(c *check.C) {
	recorder, request := s.makeRequestToServicesHandler(c)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNoContent)
}

func (s *ProvisionSuite) makeRequestToCreateHandler(c *check.C) (*httptest.ResponseRecorder, *http.Request) {
	v := url.Values{}
	v.Set("id", "some-service")
	v.Set("username", "test")
	v.Set("password", "xxxx")
	v.Set("team", "tsuruteam")
	v.Set("endpoint", "someservice.com")
	recorder, request := s.makeRequest("POST", "/services", v.Encode(), c)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return recorder, request
}

func (s *ProvisionSuite) makeRequest(method, url, body string, c *check.C) (*httptest.ResponseRecorder, *http.Request) {
	request, err := http.NewRequest(method, url, strings.NewReader(body))
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	return recorder, request
}

func (s *ProvisionSuite) TestServiceCreate(c *check.C) {
	recorder, request := s.makeRequestToCreateHandler(c)
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
	query := bson.M{"_id": "some-service"}
	var rService service.Service
	err := s.conn.Services().Find(query).One(&rService)
	c.Assert(err, check.IsNil)
	c.Assert(rService.Name, check.Equals, "some-service")
	c.Assert(rService.Endpoint["production"], check.Equals, "someservice.com")
	c.Assert(rService.Password, check.Equals, "xxxx")
	c.Assert(rService.Username, check.Equals, "test")
	c.Assert(rService.OwnerTeams, check.DeepEquals, []string{s.team.Name})
	c.Assert(eventtest.EventDesc{
		Target: serviceTarget("some-service"),
		Owner:  s.token.GetUserName(),
		Kind:   "service.create",
		StartCustomData: []map[string]interface{}{
			{"name": "team", "value": "tsuruteam"},
			{"name": "username", "value": "test"},
			{"name": "endpoint", "value": "someservice.com"},
			{"name": "id", "value": "some-service"},
		},
	}, eventtest.HasEvent)
}

func (s *ProvisionSuite) TestServiceCreateNameExists(c *check.C) {
	recorder, request := s.makeRequestToCreateHandler(c)
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
	recorder, request = s.makeRequestToCreateHandler(c)
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusConflict)
	c.Assert(recorder.Body.String(), check.Equals, "Service already exists.\n")
}

func (s *ProvisionSuite) TestServiceCreateWithoutTeam(c *check.C) {
	v := url.Values{}
	v.Set("id", "some-service")
	v.Set("username", "test")
	v.Set("password", "xxxx")
	v.Set("endpoint", "someservices.com")
	recorder, request := s.makeRequest("POST", "/services", v.Encode(), c)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
	query := bson.M{"_id": "some-service"}
	var rService service.Service
	err := s.conn.Services().Find(query).One(&rService)
	c.Assert(err, check.IsNil)
	c.Assert(rService.Endpoint["production"], check.Equals, "someservices.com")
	c.Assert(rService.Password, check.Equals, "xxxx")
	c.Assert(rService.Username, check.Equals, "test")
}

func (s *ProvisionSuite) TestServiceCreateWithoutTeamUserWithMultiplePermissions(c *check.C) {
	v := url.Values{}
	v.Set("id", "some-service")
	v.Set("username", "test")
	v.Set("password", "xxxx")
	v.Set("endpoint", "someservices.com")
	recorder, request := s.makeRequest("POST", "/services", v.Encode(), c)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	token := userWithPermission(c,
		permission.Permission{
			Scheme:  permission.PermService,
			Context: permission.Context(permission.CtxTeam, s.team.Name),
		},
		permission.Permission{
			Scheme:  permission.PermService,
			Context: permission.Context(permission.CtxTeam, "other-team"),
		},
	)
	request.Header.Set("Authorization", "b "+token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Equals, "You must provide a team responsible for this service in the manifest file.\n")
}

func (s *ProvisionSuite) TestServiceCreateReturnsBadRequestIfTheServiceDoesNotHaveAProductionEndpoint(c *check.C) {
	v := url.Values{}
	v.Set("id", "some-service")
	v.Set("password", "xxxx")
	recorder, request := s.makeRequest("POST", "/services", v.Encode(), c)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Equals, "Service production endpoint is required\n")
}

func (s *ProvisionSuite) TestServiceCreateReturnsBadRequestWithoutPassword(c *check.C) {
	v := url.Values{}
	v.Set("id", "some-service")
	v.Set("team", "tsuruteam")
	v.Set("endpoint", "someservice.com")
	recorder, request := s.makeRequest("POST", "/services", v.Encode(), c)
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Equals, "Service id is required\n")
}

func (s *ProvisionSuite) TestServiceCreateReturnsBadRequestWithoutId(c *check.C) {
	v := url.Values{}
	v.Set("password", "000000")
	v.Set("team", "tsuruteam")
	v.Set("endpoint", "someservice.com")
	recorder, request := s.makeRequest("POST", "/services", v.Encode(), c)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Equals, "Service id is required\n")
}

func (s *ProvisionSuite) TestServiceUpdate(c *check.C) {
	service := service.Service{
		Name:       "mysqlapi",
		Endpoint:   map[string]string{"production": "sqlapi.com"},
		OwnerTeams: []string{s.team.Name},
		Password:   "oldold",
	}
	err := service.Create()
	c.Assert(err, check.IsNil)
	t := authTypes.Team{Name: "myteam"}
	err = serviceTypes.Team().Insert(t)
	c.Assert(err, check.IsNil)
	v := url.Values{}
	v.Set("username", "mysqltest")
	v.Set("password", "yyyy")
	v.Set("endpoint", "mysqlapi.com")
	v.Set("team", t.Name)
	recorder, request := s.makeRequest("PUT", "/services/mysqlapi", v.Encode(), c)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	err = s.conn.Services().Find(bson.M{"_id": service.Name}).One(&service)
	c.Assert(err, check.IsNil)
	c.Assert(service.Endpoint["production"], check.Equals, "mysqlapi.com")
	c.Assert(service.Password, check.Equals, "yyyy")
	c.Assert(service.Username, check.Equals, "mysqltest")
	c.Assert(service.OwnerTeams, check.DeepEquals, []string{t.Name})
	c.Assert(eventtest.EventDesc{
		Target: serviceTarget("mysqlapi"),
		Owner:  s.token.GetUserName(),
		Kind:   "service.update",
		StartCustomData: []map[string]interface{}{
			{"name": "username", "value": "mysqltest"},
			{"name": "endpoint", "value": "mysqlapi.com"},
		},
	}, eventtest.HasEvent)
}

func (s *ProvisionSuite) TestServiceUpdateWithoutTeamIgnoresOwnerTeams(c *check.C) {
	service := service.Service{
		Name:       "mysqlapi",
		Endpoint:   map[string]string{"production": "sqlapi.com"},
		OwnerTeams: []string{s.team.Name},
		Password:   "oldold",
	}
	err := service.Create()
	c.Assert(err, check.IsNil)
	v := url.Values{}
	v.Set("username", "mysqltest")
	v.Set("password", "yyyy")
	v.Set("endpoint", "mysqlapi.com")
	recorder, request := s.makeRequest("PUT", "/services/mysqlapi", v.Encode(), c)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	err = s.conn.Services().Find(bson.M{"_id": service.Name}).One(&service)
	c.Assert(err, check.IsNil)
	c.Assert(service.Endpoint["production"], check.Equals, "mysqlapi.com")
	c.Assert(service.Password, check.Equals, "yyyy")
	c.Assert(service.Username, check.Equals, "mysqltest")
	c.Assert(service.OwnerTeams, check.DeepEquals, []string{s.team.Name})
	c.Assert(eventtest.EventDesc{
		Target: serviceTarget("mysqlapi"),
		Owner:  s.token.GetUserName(),
		Kind:   "service.update",
		StartCustomData: []map[string]interface{}{
			{"name": "username", "value": "mysqltest"},
			{"name": "endpoint", "value": "mysqlapi.com"},
		},
	}, eventtest.HasEvent)
}

func (s *ProvisionSuite) TestServiceUpdateReturnsBadRequestWithoutPassword(c *check.C) {
	service := service.Service{
		Name:       "some-service",
		Endpoint:   map[string]string{"production": "sqlapi.com"},
		OwnerTeams: []string{s.team.Name},
		Password:   "oldold",
	}
	err := service.Create()
	c.Assert(err, check.IsNil)
	v := url.Values{}
	v.Set("id", "some-service")
	v.Set("team", "tsuruteam")
	v.Set("endpoint", "someservice.com")
	recorder, request := s.makeRequest("PUT", "/services/some-service", v.Encode(), c)
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Equals, "Service password is required\n")
}

func (s *ProvisionSuite) TestServiceUpdateReturnsBadRequestWithoutProductionEndpoint(c *check.C) {
	service := service.Service{
		Name:       "mysqlapi",
		Endpoint:   map[string]string{"production": "sqlapi.com"},
		OwnerTeams: []string{s.team.Name},
		Password:   "oldold",
	}
	err := service.Create()
	c.Assert(err, check.IsNil)
	v := url.Values{}
	v.Set("id", "mysqlapi")
	v.Set("password", "zzzz")
	recorder, request := s.makeRequest("PUT", "/services/mysqlapi", v.Encode(), c)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Equals, "Service production endpoint is required\n")
}

func (s *ProvisionSuite) TestServiceUpdateReturns404WhenTheServiceDoesNotExist(c *check.C) {
	v := url.Values{}
	v.Set("id", "mysqlapi")
	v.Set("password", "zzzz")
	v.Set("username", "mysqlapi")
	v.Set("endpoint", "mysqlapi.com")
	recorder, request := s.makeRequest("PUT", "/services/mysqlapi", v.Encode(), c)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, "Service not found\n")
}

func (s *ProvisionSuite) TestServiceUpdateReturns403WhenTheUserIsNotOwnerOfTheTeam(c *check.C) {
	t := authTypes.Team{Name: "some-other-team"}
	err := serviceTypes.Team().Insert(t)
	c.Assert(err, check.IsNil)
	se := service.Service{
		Name:       "mysqlapi",
		OwnerTeams: []string{t.Name},
		Endpoint:   map[string]string{"production": "http://localhost:1234"},
		Password:   "abcde",
	}
	err = se.Create()
	c.Assert(err, check.IsNil)
	v := url.Values{}
	v.Set("id", "mysqlapi")
	v.Set("password", "zzzz")
	v.Set("username", "mysqltest")
	v.Set("endpoint", "mysqlapi.com")
	recorder, request := s.makeRequest("PUT", "/services/mysqlapi", v.Encode(), c)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}

func (s *ProvisionSuite) TestDeleteHandler(c *check.C) {
	se := service.Service{
		Name:       "mysql",
		OwnerTeams: []string{s.team.Name},
		Endpoint:   map[string]string{"production": "http://localhost:1234"},
		Password:   "abcde",
	}
	se.Create()
	u := fmt.Sprintf("/services/%s", se.Name)
	recorder, request := s.makeRequest("DELETE", u, "", c)
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	query := bson.M{"_id": se.Name}
	count, err := s.conn.Services().Find(query).Count()
	c.Assert(err, check.IsNil)
	c.Assert(count, check.Equals, 0)
	c.Assert(eventtest.EventDesc{
		Target: serviceTarget("mysql"),
		Owner:  s.token.GetUserName(),
		Kind:   "service.delete",
		StartCustomData: []map[string]interface{}{
			{"name": ":name", "value": "mysql"},
		},
	}, eventtest.HasEvent)
}

func (s *ProvisionSuite) TestDeleteHandlerReturns404WhenTheServiceDoesNotExist(c *check.C) {
	u := fmt.Sprintf("/services/%s", "mongodb")
	recorder, request := s.makeRequest("DELETE", u, "", c)
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, "Service not found\n")
}

func (s *ProvisionSuite) TestDeleteHandlerReturns403WhenTheUserIsNotOwnerOfTheTeam(c *check.C) {
	t := authTypes.Team{Name: "some-team"}
	err := serviceTypes.Team().Insert(t)
	c.Assert(err, check.IsNil)
	se := service.Service{
		Name:       "mysql",
		Teams:      []string{s.team.Name},
		OwnerTeams: []string{t.Name},
		Endpoint:   map[string]string{"production": "http://localhost:1234"},
		Password:   "abcde",
	}
	err = se.Create()
	c.Assert(err, check.IsNil)
	u := fmt.Sprintf("/services/%s", se.Name)
	recorder, request := s.makeRequest("DELETE", u, "", c)
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}

func (s *ProvisionSuite) TestDeleteHandlerReturns403WhenTheServiceHasInstance(c *check.C) {
	se := service.Service{
		Name:       "mysql",
		OwnerTeams: []string{s.team.Name},
		Endpoint:   map[string]string{"production": "http://localhost:1234"},
		Password:   "abcde",
	}
	err := se.Create()
	c.Assert(err, check.IsNil)
	instance := service.ServiceInstance{Name: "my-mysql", ServiceName: se.Name}
	err = s.conn.ServiceInstances().Insert(instance)
	c.Assert(err, check.IsNil)
	u := fmt.Sprintf("/services/%s", se.Name)
	recorder, request := s.makeRequest("DELETE", u, "", c)
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
	c.Assert(recorder.Body.String(), check.Equals, "This service cannot be removed because it has instances.\nPlease remove these instances before removing the service.\n")
}

func (s *ProvisionSuite) TestServiceProxy(c *check.C) {
	var proxyedRequest *http.Request
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyedRequest = r
		w.Header().Set("X-Response-Custom", "custom response header")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte("a message"))
	}))
	defer ts.Close()
	se := service.Service{
		Name:       "foo",
		Endpoint:   map[string]string{"production": ts.URL},
		OwnerTeams: []string{s.team.Name},
		Password:   "abcde",
	}
	err := se.Create()
	c.Assert(err, check.IsNil)
	si := service.ServiceInstance{Name: "foo-instance", ServiceName: "foo", Teams: []string{s.team.Name}}
	err = s.conn.ServiceInstances().Insert(si)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/services/proxy/service/%s?callback=/mypath", se.Name)
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
	c.Assert(proxyedRequest.Method, check.Equals, "GET")
	c.Assert(proxyedRequest.Header.Get("X-Custom"), check.Equals, "my request header")
	c.Assert(proxyedRequest.Header.Get("Authorization"), check.Not(check.Equals), reqAuth)
	c.Assert(proxyedRequest.URL.String(), check.Equals, "/mypath")
	c.Assert(eventtest.EventDesc{
		IsEmpty: true,
	}, eventtest.HasEvent)
}

func (s *ProvisionSuite) TestServiceProxyPost(c *check.C) {
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
	se := service.Service{
		Name:       "foo",
		Endpoint:   map[string]string{"production": ts.URL},
		OwnerTeams: []string{s.team.Name},
		Password:   "abcde",
	}
	err := se.Create()
	c.Assert(err, check.IsNil)
	si := service.ServiceInstance{Name: "foo-instance", ServiceName: "foo", Teams: []string{s.team.Name}}
	err = s.conn.ServiceInstances().Insert(si)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/services/proxy/service/%s?callback=/mypath", se.Name)
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
	c.Assert(proxyedRequest.Method, check.Equals, "POST")
	c.Assert(proxyedRequest.Header.Get("X-Custom"), check.Equals, "my request header")
	c.Assert(proxyedRequest.Header.Get("Authorization"), check.Not(check.Equals), reqAuth)
	c.Assert(proxyedRequest.URL.String(), check.Equals, "/mypath")
	c.Assert(string(proxyedBody), check.Equals, "my=awesome&body=1")
	c.Assert(eventtest.EventDesc{
		Target: serviceTarget("foo"),
		Owner:  s.token.GetUserName(),
		Kind:   "service.update.proxy",
		StartCustomData: []map[string]interface{}{
			{"name": ":service", "value": "foo"},
			{"name": "callback", "value": "/mypath"},
			{"name": "method", "value": "POST"},
			{"name": "my", "value": "awesome"},
			{"name": "body", "value": "1"},
		},
	}, eventtest.HasEvent)
}

func (s *ProvisionSuite) TestServiceProxyPostRawBody(c *check.C) {
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
	se := service.Service{
		Name:       "foo",
		Endpoint:   map[string]string{"production": ts.URL},
		OwnerTeams: []string{s.team.Name},
		Password:   "abcde",
	}
	err := se.Create()
	c.Assert(err, check.IsNil)
	si := service.ServiceInstance{Name: "foo-instance", ServiceName: "foo", Teams: []string{s.team.Name}}
	err = s.conn.ServiceInstances().Insert(si)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/services/proxy/service/%s?callback=/mypath", se.Name)
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
	c.Assert(proxyedRequest.Method, check.Equals, "POST")
	c.Assert(proxyedRequest.Header.Get("X-Custom"), check.Equals, "my request header")
	c.Assert(proxyedRequest.Header.Get("Authorization"), check.Not(check.Equals), reqAuth)
	c.Assert(proxyedRequest.URL.String(), check.Equals, "/mypath")
	c.Assert(string(proxyedBody), check.Equals, "something-something")
	c.Assert(eventtest.EventDesc{
		Target: serviceTarget("foo"),
		Owner:  s.token.GetUserName(),
		Kind:   "service.update.proxy",
		StartCustomData: []map[string]interface{}{
			{"name": ":service", "value": "foo"},
			{"name": "callback", "value": "/mypath"},
			{"name": "method", "value": "POST"},
		},
	}, eventtest.HasEvent)
}

func (s *ProvisionSuite) TestServiceProxyNoContent(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()
	se := service.Service{
		Name:       "foo",
		Endpoint:   map[string]string{"production": ts.URL},
		OwnerTeams: []string{s.team.Name},
		Password:   "abcde",
	}
	err := se.Create()
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/services/proxy/service/%s?callback=/mypath", se.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	reqAuth := "bearer " + s.token.GetValue()
	request.Header.Set("Authorization", reqAuth)
	recorder := &closeNotifierResponseRecorder{httptest.NewRecorder()}
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNoContent)
}

func (s *ProvisionSuite) TestServiceProxyError(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte("some error"))
	}))
	defer ts.Close()
	se := service.Service{
		Name:       "foo",
		Endpoint:   map[string]string{"production": ts.URL},
		OwnerTeams: []string{s.team.Name},
		Password:   "abcde",
	}
	err := se.Create()
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/services/proxy/service/%s?callback=/mypath", se.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	reqAuth := "bearer " + s.token.GetValue()
	request.Header.Set("Authorization", reqAuth)
	recorder := &closeNotifierResponseRecorder{httptest.NewRecorder()}
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadGateway)
	c.Assert(recorder.Body.String(), check.Equals, "some error")
}

func (s *ProvisionSuite) TestServiceProxyNotFound(c *check.C) {
	url := "/services/proxy/service/some-service?callback=/mypath"
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	reqAuth := "bearer " + s.token.GetValue()
	request.Header.Set("Authorization", reqAuth)
	recorder := &closeNotifierResponseRecorder{httptest.NewRecorder()}
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, "Service not found\n")
}

func (s *ProvisionSuite) TestServiceProxyAccessDenied(c *check.C) {
	var proxyedRequest *http.Request
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyedRequest = r
		w.Header().Set("X-Response-Custom", "custom response header")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte("a message"))
	}))
	defer ts.Close()
	t := authTypes.Team{Name: "newteam"}
	err := serviceTypes.Team().Insert(t)
	c.Assert(err, check.IsNil)
	se := service.Service{Name: "foo", Endpoint: map[string]string{"production": ts.URL}, Password: "abcde", OwnerTeams: []string{t.Name}}
	err = se.Create()
	c.Assert(err, check.IsNil)
	si := service.ServiceInstance{Name: "foo-instance", ServiceName: "foo", Teams: []string{s.team.Name}}
	err = s.conn.ServiceInstances().Insert(si)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/services/proxy/service/%s?callback=/mypath", se.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	reqAuth := "bearer " + s.token.GetValue()
	request.Header.Set("Authorization", reqAuth)
	request.Header.Set("X-Custom", "my request header")
	recorder := &closeNotifierResponseRecorder{httptest.NewRecorder()}
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
	c.Assert(proxyedRequest, check.IsNil)
}

func (s *ProvisionSuite) TestGrantServiceAccessToTeam(c *check.C) {
	t := &authTypes.Team{Name: "blaaaa"}
	serviceTypes.Team().Insert(*t)
	se := service.Service{
		Name:       "my-service",
		OwnerTeams: []string{s.team.Name},
		Endpoint:   map[string]string{"production": "http://localhost:1234"},
		Password:   "abcde",
	}
	err := se.Create()
	c.Assert(err, check.IsNil)
	u := fmt.Sprintf("/services/%s/team/%s", se.Name, t.Name)
	recorder, request := s.makeRequest("PUT", u, "", c)
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	err = se.Get()
	c.Assert(err, check.IsNil)
	c.Assert(*t, HasAccessTo, se)
	c.Assert(eventtest.EventDesc{
		Target: serviceTarget("my-service"),
		Owner:  s.token.GetUserName(),
		Kind:   "service.update.grant-access",
		StartCustomData: []map[string]interface{}{
			{"name": ":service", "value": "my-service"},
			{"name": ":team", "value": t.Name},
		},
	}, eventtest.HasEvent)
}

func (s *ProvisionSuite) TestGrantAccessToTeamServiceNotFound(c *check.C) {
	u := fmt.Sprintf("/services/nononono/team/%s", s.team.Name)
	recorder, request := s.makeRequest("PUT", u, "", c)
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, "Service not found\n")
}

func (s *ProvisionSuite) TestGrantServiceAccessToTeamNoAccess(c *check.C) {
	t := authTypes.Team{Name: "my-team"}
	err := serviceTypes.Team().Insert(t)
	c.Assert(err, check.IsNil)
	se := service.Service{Name: "my-service", Endpoint: map[string]string{"production": "http://localhost:1234"}, Password: "abcde", OwnerTeams: []string{t.Name}}
	err = se.Create()
	c.Assert(err, check.IsNil)
	u := fmt.Sprintf("/services/%s/team/%s", se.Name, s.team.Name)
	recorder, request := s.makeRequest("PUT", u, "", c)
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}

func (s *ProvisionSuite) TestGrantServiceAccessToTeamReturnNotFoundIfTheTeamDoesNotExist(c *check.C) {
	se := service.Service{
		Name:       "my-service",
		OwnerTeams: []string{s.team.Name},
		Endpoint:   map[string]string{"production": "http://localhost:1234"},
		Password:   "abcde",
	}
	err := se.Create()
	c.Assert(err, check.IsNil)
	u := fmt.Sprintf("/services/%s/team/nonono", se.Name)
	recorder, request := s.makeRequest("PUT", u, "", c)
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Equals, "Team not found\n")
}

func (s *ProvisionSuite) TestGrantServiceAccessToTeamAlreadyAccess(c *check.C) {
	se := service.Service{
		Name:       "my-service",
		OwnerTeams: []string{s.team.Name},
		Teams:      []string{s.team.Name},
		Endpoint:   map[string]string{"production": "http://localhost:1234"},
		Password:   "abcde",
	}
	err := se.Create()
	c.Assert(err, check.IsNil)
	u := fmt.Sprintf("/services/%s/team/%s", se.Name, s.team.Name)
	recorder, request := s.makeRequest("PUT", u, "", c)
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusConflict)
}

func (s *ProvisionSuite) TestRevokeServiceAccessFromTeamRemovesTeamFromService(c *check.C) {
	se := service.Service{
		Name:       "my-service",
		OwnerTeams: []string{s.team.Name},
		Teams:      []string{s.team.Name, "other-team"},
		Endpoint:   map[string]string{"production": "http://localhost:1234"},
		Password:   "abcde",
	}
	err := se.Create()
	c.Assert(err, check.IsNil)
	u := fmt.Sprintf("/services/%s/team/%s", se.Name, s.team.Name)
	recorder, request := s.makeRequest("DELETE", u, "", c)
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	err = se.Get()
	c.Assert(err, check.IsNil)
	c.Assert(*s.team, check.Not(HasAccessTo), se)
	c.Assert(eventtest.EventDesc{
		Target: serviceTarget("my-service"),
		Owner:  s.token.GetUserName(),
		Kind:   "service.update.revoke-access",
		StartCustomData: []map[string]interface{}{
			{"name": ":service", "value": "my-service"},
			{"name": ":team", "value": s.team.Name},
		},
	}, eventtest.HasEvent)
}

func (s *ProvisionSuite) TestRevokeServiceAccessFromTeamReturnsNotFoundIfTheServiceDoesNotExist(c *check.C) {
	u := fmt.Sprintf("/services/nonono/team/%s", s.team.Name)
	recorder, request := s.makeRequest("DELETE", u, "", c)
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, "Service not found\n")
}

func (s *ProvisionSuite) TestRevokeAccessFromTeamReturnsForbiddenIfTheGivenUserDoesNotHasAccessToTheService(c *check.C) {
	t := authTypes.Team{Name: "alle-da"}
	err := serviceTypes.Team().Insert(t)
	c.Assert(err, check.IsNil)
	se := service.Service{
		Name:       "my-service",
		OwnerTeams: []string{t.Name},
		Teams:      []string{t.Name},
		Endpoint:   map[string]string{"production": "http://localhost:1234"},
		Password:   "abcde",
	}
	err = se.Create()
	c.Assert(err, check.IsNil)
	u := fmt.Sprintf("/services/%s/team/%s", se.Name, t.Name)
	recorder, request := s.makeRequest("DELETE", u, "", c)
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}

func (s *ProvisionSuite) TestRevokeServiceAccessFromTeamReturnsNotFoundIfTheTeamDoesNotExist(c *check.C) {
	se := service.Service{
		Name:       "my-service",
		OwnerTeams: []string{s.team.Name},
		Teams:      []string{s.team.Name, "some-other"},
		Endpoint:   map[string]string{"production": "http://localhost:1234"},
		Password:   "abcde",
	}
	err := se.Create()
	c.Assert(err, check.IsNil)
	u := fmt.Sprintf("/services/%s/team/nonono", se.Name)
	recorder, request := s.makeRequest("DELETE", u, "", c)
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Equals, "Team not found\n")
}

func (s *ProvisionSuite) TestRevokeServiceAccessFromTeamReturnsForbiddenIfTheTeamIsTheOnlyWithAccessToTheService(c *check.C) {
	se := service.Service{
		Name:       "my-service",
		OwnerTeams: []string{s.team.Name},
		Teams:      []string{s.team.Name},
		Endpoint:   map[string]string{"production": "http://localhost:1234"},
		Password:   "abcde",
	}
	err := se.Create()
	c.Assert(err, check.IsNil)
	u := fmt.Sprintf("/services/%s/team/%s", se.Name, s.team.Name)
	recorder, request := s.makeRequest("DELETE", u, "", c)
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
	c.Assert(recorder.Body.String(), check.Equals, "You can not revoke the access from this team, because it is the unique team with access to this service, and a service can not be orphaned\n")
}

func (s *ProvisionSuite) TestRevokeServiceAccessFromTeamReturnNotFoundIfTheTeamDoesNotHasAccessToTheService(c *check.C) {
	t := authTypes.Team{Name: "Rammlied"}
	err := serviceTypes.Team().Insert(t)
	c.Assert(err, check.IsNil)
	se := service.Service{
		Name:       "my-service",
		OwnerTeams: []string{s.team.Name},
		Teams:      []string{s.team.Name, "other-team"},
		Endpoint:   map[string]string{"production": "http://localhost:1234"},
		Password:   "abcde",
	}
	err = se.Create()
	c.Assert(err, check.IsNil)
	u := fmt.Sprintf("/services/%s/team/%s", se.Name, t.Name)
	recorder, request := s.makeRequest("DELETE", u, "", c)
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusConflict)
}

func (s *ProvisionSuite) TestAddDocServiceDoesNotExist(c *check.C) {
	v := url.Values{}
	v.Set("doc", "doc")
	recorder, request := s.makeRequest("PUT", "/services/mongodb/doc", v.Encode(), c)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
}

func (s *ProvisionSuite) TestAddDoc(c *check.C) {
	se := service.Service{
		Name:       "some-service",
		OwnerTeams: []string{s.team.Name},
		Endpoint:   map[string]string{"production": "http://localhost:1234"},
		Password:   "abcde",
	}
	se.Create()
	v := url.Values{}
	v.Set("doc", "doc")
	recorder, request := s.makeRequest("PUT", "/services/some-service/doc", v.Encode(), c)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	query := bson.M{"_id": "some-service"}
	var serv service.Service
	err := s.conn.Services().Find(query).One(&serv)
	c.Assert(err, check.IsNil)
	c.Assert(serv.Doc, check.Equals, "doc")
	c.Assert(eventtest.EventDesc{
		Target: serviceTarget("some-service"),
		Owner:  s.token.GetUserName(),
		Kind:   "service.update.doc",
		StartCustomData: []map[string]interface{}{
			{"name": ":name", "value": "some-service"},
			{"name": "doc", "value": "doc"},
		},
	}, eventtest.HasEvent)
}

func (s *ProvisionSuite) TestAddDocUserHasNoAccess(c *check.C) {
	t := authTypes.Team{Name: "new-team"}
	err := serviceTypes.Team().Insert(t)
	c.Assert(err, check.IsNil)
	se := service.Service{Name: "mysql", Endpoint: map[string]string{"production": "http://localhost:1234"}, Password: "abcde", OwnerTeams: []string{t.Name}}
	err = se.Create()
	c.Assert(err, check.IsNil)
	v := url.Values{}
	v.Set("doc", "doc")
	recorder, request := s.makeRequest("PUT", "/services/mysql/doc", v.Encode(), c)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}
