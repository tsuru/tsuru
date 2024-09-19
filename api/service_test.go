// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db/storagev2"
	"github.com/tsuru/tsuru/event/eventtest"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/permission/permissiontest"
	"github.com/tsuru/tsuru/service"
	"github.com/tsuru/tsuru/servicemanager"
	_ "github.com/tsuru/tsuru/storage/mongodb"
	authTypes "github.com/tsuru/tsuru/types/auth"
	permTypes "github.com/tsuru/tsuru/types/permission"
	mongoBSON "go.mongodb.org/mongo-driver/bson"
	"golang.org/x/crypto/bcrypt"
	check "gopkg.in/check.v1"
)

type ProvisionSuite struct {
	team            *authTypes.Team
	user            *auth.User
	token           auth.Token
	testServer      http.Handler
	mockTeamService *authTypes.MockTeamService
}

var _ = check.Suite(&ProvisionSuite{})

func (s *ProvisionSuite) SetUpTest(c *check.C) {
	app.AuthScheme = nativeScheme
	config.Set("database:driver", "mongodb")
	config.Set("database:url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("database:name", "tsuru_api_service_test")
	config.Set("auth:hash-cost", bcrypt.MinCost)

	storagev2.Reset()

	storagev2.ClearAllCollections(nil)
	s.createUserAndTeam(c)
	s.testServer = RunServer(true)
	s.mockTeamService = &authTypes.MockTeamService{}
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
	servicemanager.Team = s.mockTeamService
}

func (s *ProvisionSuite) TearDownTest(c *check.C) {
}

func (s *ProvisionSuite) TearDownSuite(c *check.C) {
	storagev2.ClearAllCollections(nil)
}

func (s *ProvisionSuite) makeRequestToServicesHandler(c *check.C) (*httptest.ResponseRecorder, *http.Request) {
	recorder, request := s.makeRequest(http.MethodGet, "/services", "", c)
	return recorder, request
}

func (s *ProvisionSuite) createUserAndTeam(c *check.C) {
	s.team = &authTypes.Team{Name: "tsuruteam"}
	_, s.token = permissiontest.CustomUserWithPermission(c, nativeScheme, "provision-master-user", permission.Permission{
		Scheme:  permission.PermService,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	var err error
	s.user, err = auth.ConvertNewUser(s.token.User(context.TODO()))
	c.Assert(err, check.IsNil)
}

func (s *ProvisionSuite) TestServiceListGetAllServicesFromUsersTeam(c *check.C) {
	srv := service.Service{
		Name:       "mongodb",
		OwnerTeams: []string{s.team.Name},
		Endpoint:   map[string]string{"production": "http://localhost:1234"},
		Password:   "abcde",
	}
	err := service.Create(context.TODO(), srv)
	c.Assert(err, check.IsNil)
	si := service.ServiceInstance{Name: "my_nosql", ServiceName: srv.Name, Teams: []string{s.team.Name}, Tags: []string{"tag 1"}}
	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), si)
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
		ServiceInstances: []service.ServiceInstance{
			{Name: "my_nosql", Tags: []string{"tag 1"}, ServiceName: "mongodb"},
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
	recorder, request := s.makeRequest(http.MethodPost, "/services", v.Encode(), c)
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
	query := mongoBSON.M{"_id": "some-service"}
	var rService service.Service

	servicesCollection, err := storagev2.ServicesCollection()
	c.Assert(err, check.IsNil)

	err = servicesCollection.FindOne(context.TODO(), query).Decode(&rService)
	c.Assert(err, check.IsNil)

	c.Assert(rService.Name, check.Equals, "some-service")
	c.Assert(rService.Endpoint["production"], check.Equals, "someservice.com")
	c.Assert(rService.Password, check.Equals, "xxxx")
	c.Assert(rService.Username, check.Equals, "test")
	c.Assert(rService.OwnerTeams, check.DeepEquals, []string{s.team.Name})
	c.Assert(rService.IsMultiCluster, check.Equals, false)
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

func (s *ProvisionSuite) TestServiceCreateMultipleEndpoints(c *check.C) {
	v := url.Values{}
	v.Set("id", "some-service")
	v.Set("username", "test")
	v.Set("password", "xxxx")
	v.Set("team", "tsuruteam")
	v.Set("endpoints.cluster1", "cluster1.com")
	v.Set("endpoints.cluster2", "cluster2.com")
	recorder, request := s.makeRequest(http.MethodPost, "/services", v.Encode(), c)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
	query := mongoBSON.M{"_id": "some-service"}
	var rService service.Service

	servicesCollection, err := storagev2.ServicesCollection()
	c.Assert(err, check.IsNil)

	err = servicesCollection.FindOne(context.TODO(), query).Decode(&rService)
	c.Assert(err, check.IsNil)

	c.Assert(rService.Name, check.Equals, "some-service")
	c.Assert(rService.Endpoint["cluster1"], check.Equals, "cluster1.com")
	c.Assert(rService.Endpoint["cluster2"], check.Equals, "cluster2.com")
	c.Assert(rService.Password, check.Equals, "xxxx")
	c.Assert(rService.Username, check.Equals, "test")
	c.Assert(rService.OwnerTeams, check.DeepEquals, []string{s.team.Name})
	c.Assert(rService.IsMultiCluster, check.Equals, false)
	c.Assert(eventtest.EventDesc{
		Target: serviceTarget("some-service"),
		Owner:  s.token.GetUserName(),
		Kind:   "service.create",
		StartCustomData: []map[string]interface{}{
			{"name": "team", "value": "tsuruteam"},
			{"name": "username", "value": "test"},
			{"name": "endpoints.cluster1", "value": "cluster1.com"},
			{"name": "endpoints.cluster2", "value": "cluster2.com"},
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
	recorder, request := s.makeRequest(http.MethodPost, "/services", v.Encode(), c)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
	query := mongoBSON.M{"_id": "some-service"}
	var rService service.Service

	servicesCollection, err := storagev2.ServicesCollection()
	c.Assert(err, check.IsNil)

	err = servicesCollection.FindOne(context.TODO(), query).Decode(&rService)
	c.Assert(err, check.IsNil)

	c.Assert(rService.Endpoint["production"], check.Equals, "someservices.com")
	c.Assert(rService.Password, check.Equals, "xxxx")
	c.Assert(rService.Username, check.Equals, "test")
	c.Assert(rService.IsMultiCluster, check.Equals, false)
}

func (s *ProvisionSuite) TestServiceCreateWithoutTeamUserWithMultiplePermissions(c *check.C) {
	v := url.Values{}
	v.Set("id", "some-service")
	v.Set("username", "test")
	v.Set("password", "xxxx")
	v.Set("endpoint", "someservices.com")
	recorder, request := s.makeRequest(http.MethodPost, "/services", v.Encode(), c)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	token := userWithPermission(c,
		permission.Permission{
			Scheme:  permission.PermService,
			Context: permission.Context(permTypes.CtxTeam, s.team.Name),
		},
		permission.Permission{
			Scheme:  permission.PermService,
			Context: permission.Context(permTypes.CtxTeam, "other-team"),
		},
	)
	request.Header.Set("Authorization", "b "+token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Equals, "You must provide a team responsible for this service in the manifest file.\n")
}

func (s *ProvisionSuite) TestServiceCreateReturnsBadRequestIfTheServiceDoesNotHaveAnyEndpoints(c *check.C) {
	v := url.Values{}
	v.Set("id", "some-service")
	v.Set("password", "xxxx")
	recorder, request := s.makeRequest(http.MethodPost, "/services", v.Encode(), c)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Equals, "At least one endpoint is required\n")
}

func (s *ProvisionSuite) TestServiceCreateReturnsBadRequestWithoutPassword(c *check.C) {
	v := url.Values{}
	v.Set("id", "some-service")
	v.Set("team", "tsuruteam")
	v.Set("endpoint", "someservice.com")
	recorder, request := s.makeRequest(http.MethodPost, "/services", v.Encode(), c)
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Equals, "Service id is required\n")
}

func (s *ProvisionSuite) TestServiceCreateReturnsBadRequestWithoutId(c *check.C) {
	v := url.Values{}
	v.Set("password", "000000")
	v.Set("team", "tsuruteam")
	v.Set("endpoint", "someservice.com")
	recorder, request := s.makeRequest(http.MethodPost, "/services", v.Encode(), c)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Equals, "Service id is required\n")
}

func (s *ProvisionSuite) TestServiceCreateWithMultiClusterEnabled(c *check.C) {
	v := url.Values{}
	v.Set("id", "multi-cluster-service")
	v.Set("username", "user")
	v.Set("password", "password")
	v.Set("endpoint", "http://multicluster.service.example.com")
	v.Set("multi-cluster", "true")
	recorder, request := s.makeRequest(http.MethodPost, "/services", v.Encode(), c)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
	query := mongoBSON.M{"_id": "multi-cluster-service"}
	var rService service.Service

	servicesCollection, err := storagev2.ServicesCollection()
	c.Assert(err, check.IsNil)

	err = servicesCollection.FindOne(context.TODO(), query).Decode(&rService)
	c.Assert(err, check.IsNil)
	c.Assert(rService.Endpoint["production"], check.Equals, "http://multicluster.service.example.com")
	c.Assert(rService.Username, check.Equals, "user")
	c.Assert(rService.Password, check.Equals, "password")
	c.Assert(rService.IsMultiCluster, check.Equals, true)
}

func (s *ProvisionSuite) TestServiceUpdate(c *check.C) {
	srv := service.Service{
		Name:       "mysqlapi",
		Endpoint:   map[string]string{"production": "sqlapi.com"},
		OwnerTeams: []string{s.team.Name},
		Password:   "oldold",
	}
	err := service.Create(context.TODO(), srv)
	c.Assert(err, check.IsNil)
	t := authTypes.Team{Name: "myteam"}
	v := url.Values{}
	v.Set("username", "mysqltest")
	v.Set("password", "yyyy")
	v.Set("endpoint", "mysqlapi.com")
	v.Set("team", t.Name)
	recorder, request := s.makeRequest(http.MethodPut, "/services/mysqlapi", v.Encode(), c)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)

	servicesCollection, err := storagev2.ServicesCollection()
	c.Assert(err, check.IsNil)

	err = servicesCollection.FindOne(context.TODO(), mongoBSON.M{"_id": srv.Name}).Decode(&srv)
	c.Assert(err, check.IsNil)

	c.Assert(srv.Endpoint["production"], check.Equals, "mysqlapi.com")
	c.Assert(srv.Password, check.Equals, "yyyy")
	c.Assert(srv.Username, check.Equals, "mysqltest")
	c.Assert(srv.OwnerTeams, check.DeepEquals, []string{t.Name})
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
	srv := service.Service{
		Name:       "mysqlapi",
		Endpoint:   map[string]string{"production": "sqlapi.com"},
		OwnerTeams: []string{s.team.Name},
		Password:   "oldold",
	}
	err := service.Create(context.TODO(), srv)
	c.Assert(err, check.IsNil)
	v := url.Values{}
	v.Set("username", "mysqltest")
	v.Set("password", "yyyy")
	v.Set("endpoint", "mysqlapi.com")
	recorder, request := s.makeRequest(http.MethodPut, "/services/mysqlapi", v.Encode(), c)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)

	servicesCollection, err := storagev2.ServicesCollection()
	c.Assert(err, check.IsNil)

	err = servicesCollection.FindOne(context.TODO(), mongoBSON.M{"_id": srv.Name}).Decode(&srv)
	c.Assert(err, check.IsNil)

	c.Assert(srv.Endpoint["production"], check.Equals, "mysqlapi.com")
	c.Assert(srv.Password, check.Equals, "yyyy")
	c.Assert(srv.Username, check.Equals, "mysqltest")
	c.Assert(srv.OwnerTeams, check.DeepEquals, []string{s.team.Name})
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
	srv := service.Service{
		Name:       "some-service",
		Endpoint:   map[string]string{"production": "sqlapi.com"},
		OwnerTeams: []string{s.team.Name},
		Password:   "oldold",
	}
	err := service.Create(context.TODO(), srv)
	c.Assert(err, check.IsNil)
	v := url.Values{}
	v.Set("id", "some-service")
	v.Set("team", "tsuruteam")
	v.Set("endpoint", "someservice.com")
	recorder, request := s.makeRequest(http.MethodPut, "/services/some-service", v.Encode(), c)
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Equals, "Service password is required\n")
}

func (s *ProvisionSuite) TestServiceUpdateReturnsBadRequestWithoutProductionEndpoint(c *check.C) {
	srv := service.Service{
		Name:       "mysqlapi",
		Endpoint:   map[string]string{"production": "sqlapi.com"},
		OwnerTeams: []string{s.team.Name},
		Password:   "oldold",
	}
	err := service.Create(context.TODO(), srv)
	c.Assert(err, check.IsNil)
	v := url.Values{}
	v.Set("id", "mysqlapi")
	v.Set("password", "zzzz")
	recorder, request := s.makeRequest(http.MethodPut, "/services/mysqlapi", v.Encode(), c)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Equals, "At least one endpoint is required\n")
}

func (s *ProvisionSuite) TestServiceUpdateReturns404WhenTheServiceDoesNotExist(c *check.C) {
	v := url.Values{}
	v.Set("id", "mysqlapi")
	v.Set("password", "zzzz")
	v.Set("username", "mysqlapi")
	v.Set("endpoint", "mysqlapi.com")
	recorder, request := s.makeRequest(http.MethodPut, "/services/mysqlapi", v.Encode(), c)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, "Service not found\n")
}

func (s *ProvisionSuite) TestServiceUpdateReturns403WhenTheUserIsNotOwnerOfTheTeam(c *check.C) {
	t := authTypes.Team{Name: "some-other-team"}
	se := service.Service{
		Name:       "mysqlapi",
		OwnerTeams: []string{t.Name},
		Endpoint:   map[string]string{"production": "http://localhost:1234"},
		Password:   "abcde",
	}
	err := service.Create(context.TODO(), se)
	c.Assert(err, check.IsNil)
	v := url.Values{}
	v.Set("id", "mysqlapi")
	v.Set("password", "zzzz")
	v.Set("username", "mysqltest")
	v.Set("endpoint", "mysqlapi.com")
	recorder, request := s.makeRequest(http.MethodPut, "/services/mysqlapi", v.Encode(), c)
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
	err := service.Create(context.TODO(), se)
	c.Assert(err, check.IsNil)
	u := fmt.Sprintf("/services/%s", se.Name)
	recorder, request := s.makeRequest(http.MethodDelete, u, "", c)
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	query := mongoBSON.M{"_id": se.Name}

	servicesCollection, err := storagev2.ServicesCollection()
	c.Assert(err, check.IsNil)

	count, err := servicesCollection.CountDocuments(context.TODO(), query)
	c.Assert(err, check.IsNil)
	c.Assert(count, check.Equals, int64(0))
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
	recorder, request := s.makeRequest(http.MethodDelete, u, "", c)
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, "Service not found\n")
}

func (s *ProvisionSuite) TestDeleteHandlerReturns403WhenTheUserIsNotOwnerOfTheTeam(c *check.C) {
	t := authTypes.Team{Name: "some-team"}
	se := service.Service{
		Name:       "mysql",
		Teams:      []string{s.team.Name},
		OwnerTeams: []string{t.Name},
		Endpoint:   map[string]string{"production": "http://localhost:1234"},
		Password:   "abcde",
	}
	err := service.Create(context.TODO(), se)
	c.Assert(err, check.IsNil)
	u := fmt.Sprintf("/services/%s", se.Name)
	recorder, request := s.makeRequest(http.MethodDelete, u, "", c)
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
	err := service.Create(context.TODO(), se)
	c.Assert(err, check.IsNil)
	instance := service.ServiceInstance{Name: "my-mysql", ServiceName: se.Name}
	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), instance)
	c.Assert(err, check.IsNil)
	u := fmt.Sprintf("/services/%s", se.Name)
	recorder, request := s.makeRequest(http.MethodDelete, u, "", c)
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
	err := service.Create(context.TODO(), se)
	c.Assert(err, check.IsNil)
	si := service.ServiceInstance{Name: "foo-instance", ServiceName: "foo", Teams: []string{s.team.Name}}
	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), si)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/services/proxy/service/%s?callback=/mypath", se.Name)
	request, err := http.NewRequest(http.MethodGet, url, nil)
	c.Assert(err, check.IsNil)
	reqAuth := "bearer " + s.token.GetValue()
	request.Header.Set("Authorization", reqAuth)
	request.Header.Set("X-Custom", "my request header")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
	c.Assert(recorder.Header().Get("X-Response-Custom"), check.Equals, "custom response header")
	c.Assert(recorder.Body.String(), check.Equals, "a message")
	c.Assert(proxyedRequest, check.NotNil)
	c.Assert(proxyedRequest.Method, check.Equals, http.MethodGet)
	c.Assert(proxyedRequest.Header.Get("X-Custom"), check.Equals, "my request header")
	c.Assert(proxyedRequest.Header.Get("Authorization"), check.Not(check.Equals), reqAuth)
	c.Assert(proxyedRequest.URL.String(), check.Equals, "/mypath")
	c.Assert(eventtest.EventDesc{
		IsEmpty: true,
	}, eventtest.HasEvent)
}

func (s *ProvisionSuite) TestServiceAuthenticatedResources(c *check.C) {
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
		Endpoint:   map[string]string{"production": ts.URL + "/v2"},
		OwnerTeams: []string{s.team.Name},
		Password:   "abcde",
	}
	err := service.Create(context.TODO(), se)
	c.Assert(err, check.IsNil)
	si := service.ServiceInstance{Name: "foo-instance", ServiceName: "foo", Teams: []string{s.team.Name}}
	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), si)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/services/%s/authenticated-resources/mypath", se.Name)
	request, err := http.NewRequest(http.MethodGet, url, nil)
	c.Assert(err, check.IsNil)
	reqAuth := "bearer " + s.token.GetValue()
	request.Header.Set("Authorization", reqAuth)
	request.Header.Set("X-Custom", "my request header")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
	c.Assert(recorder.Header().Get("X-Response-Custom"), check.Equals, "custom response header")
	c.Assert(recorder.Body.String(), check.Equals, "a message")
	c.Assert(proxyedRequest, check.NotNil)
	c.Assert(proxyedRequest.Method, check.Equals, http.MethodGet)
	c.Assert(proxyedRequest.Header.Get("X-Custom"), check.Equals, "my request header")
	c.Assert(proxyedRequest.Header.Get("Authorization"), check.Not(check.Equals), reqAuth)
	c.Assert(proxyedRequest.URL.String(), check.Equals, "/v2/authenticated-resources/mypath")
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
		proxyedBody, err = io.ReadAll(r.Body)
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
	err := service.Create(context.TODO(), se)
	c.Assert(err, check.IsNil)
	si := service.ServiceInstance{Name: "foo-instance", ServiceName: "foo", Teams: []string{s.team.Name}}
	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), si)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/services/proxy/service/%s?callback=/mypath", se.Name)
	body := strings.NewReader("my=awesome&body=1")
	request, err := http.NewRequest(http.MethodPost, url, body)
	c.Assert(err, check.IsNil)
	reqAuth := "bearer " + s.token.GetValue()
	request.Header.Set("Authorization", reqAuth)
	request.Header.Set("X-Custom", "my request header")
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
	c.Assert(recorder.Header().Get("X-Response-Custom"), check.Equals, "custom response header")
	c.Assert(recorder.Body.String(), check.Equals, "a message")
	c.Assert(proxyedRequest, check.NotNil)
	c.Assert(proxyedRequest.Method, check.Equals, http.MethodPost)
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
			{"name": "method", "value": http.MethodPost},
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
		proxyedBody, err = io.ReadAll(r.Body)
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
	err := service.Create(context.TODO(), se)
	c.Assert(err, check.IsNil)
	si := service.ServiceInstance{Name: "foo-instance", ServiceName: "foo", Teams: []string{s.team.Name}}
	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), si)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/services/proxy/service/%s?callback=/mypath", se.Name)
	body := strings.NewReader("something-something")
	request, err := http.NewRequest(http.MethodPost, url, body)
	c.Assert(err, check.IsNil)
	reqAuth := "bearer " + s.token.GetValue()
	request.Header.Set("Authorization", reqAuth)
	request.Header.Set("X-Custom", "my request header")
	request.Header.Set("Content-Type", "text/plain")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
	c.Assert(recorder.Header().Get("X-Response-Custom"), check.Equals, "custom response header")
	c.Assert(recorder.Body.String(), check.Equals, "a message")
	c.Assert(proxyedRequest, check.NotNil)
	c.Assert(proxyedRequest.Method, check.Equals, http.MethodPost)
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
			{"name": "method", "value": http.MethodPost},
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
	err := service.Create(context.TODO(), se)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/services/proxy/service/%s?callback=/mypath", se.Name)
	request, err := http.NewRequest(http.MethodGet, url, nil)
	c.Assert(err, check.IsNil)
	reqAuth := "bearer " + s.token.GetValue()
	request.Header.Set("Authorization", reqAuth)
	recorder := httptest.NewRecorder()
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
	err := service.Create(context.TODO(), se)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/services/proxy/service/%s?callback=/mypath", se.Name)
	request, err := http.NewRequest(http.MethodGet, url, nil)
	c.Assert(err, check.IsNil)
	reqAuth := "bearer " + s.token.GetValue()
	request.Header.Set("Authorization", reqAuth)
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadGateway)
	c.Assert(recorder.Body.String(), check.Equals, "some error")
}

func (s *ProvisionSuite) TestServiceProxyNotFound(c *check.C) {
	url := "/services/proxy/service/some-service?callback=/mypath"
	request, err := http.NewRequest(http.MethodGet, url, nil)
	c.Assert(err, check.IsNil)
	reqAuth := "bearer " + s.token.GetValue()
	request.Header.Set("Authorization", reqAuth)
	recorder := httptest.NewRecorder()
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
	se := service.Service{Name: "foo", Endpoint: map[string]string{"production": ts.URL}, Password: "abcde", OwnerTeams: []string{t.Name}}
	err := service.Create(context.TODO(), se)
	c.Assert(err, check.IsNil)
	si := service.ServiceInstance{Name: "foo-instance", ServiceName: "foo", Teams: []string{s.team.Name}}
	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), si)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/services/proxy/service/%s?callback=/mypath", se.Name)
	request, err := http.NewRequest(http.MethodGet, url, nil)
	c.Assert(err, check.IsNil)
	reqAuth := "bearer " + s.token.GetValue()
	request.Header.Set("Authorization", reqAuth)
	request.Header.Set("X-Custom", "my request header")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
	c.Assert(proxyedRequest, check.IsNil)
}

func (s *ProvisionSuite) TestGrantServiceAccessToTeam(c *check.C) {
	t := &authTypes.Team{Name: "blaaaa"}
	se := service.Service{
		Name:       "my-service",
		OwnerTeams: []string{s.team.Name},
		Endpoint:   map[string]string{"production": "http://localhost:1234"},
		Password:   "abcde",
	}
	err := service.Create(context.TODO(), se)
	c.Assert(err, check.IsNil)
	u := fmt.Sprintf("/services/%s/team/%s", se.Name, t.Name)
	recorder, request := s.makeRequest(http.MethodPut, u, "", c)
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	se, err = service.Get(context.TODO(), "my-service")
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
	recorder, request := s.makeRequest(http.MethodPut, u, "", c)
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, "Service not found\n")
}

func (s *ProvisionSuite) TestGrantServiceAccessToTeamNoAccess(c *check.C) {
	t := authTypes.Team{Name: "my-team"}
	se := service.Service{Name: "my-service", Endpoint: map[string]string{"production": "http://localhost:1234"}, Password: "abcde", OwnerTeams: []string{t.Name}}
	err := service.Create(context.TODO(), se)
	c.Assert(err, check.IsNil)
	u := fmt.Sprintf("/services/%s/team/%s", se.Name, s.team.Name)
	recorder, request := s.makeRequest(http.MethodPut, u, "", c)
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}

func (s *ProvisionSuite) TestGrantServiceAccessToTeamReturnNotFoundIfTheTeamDoesNotExist(c *check.C) {
	s.mockTeamService.OnFindByName = func(_ string) (*authTypes.Team, error) {
		return nil, authTypes.ErrTeamNotFound
	}
	se := service.Service{
		Name:       "my-service",
		OwnerTeams: []string{s.team.Name},
		Endpoint:   map[string]string{"production": "http://localhost:1234"},
		Password:   "abcde",
	}
	err := service.Create(context.TODO(), se)
	c.Assert(err, check.IsNil)
	u := fmt.Sprintf("/services/%s/team/nonono", se.Name)
	recorder, request := s.makeRequest(http.MethodPut, u, "", c)
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
	err := service.Create(context.TODO(), se)
	c.Assert(err, check.IsNil)
	u := fmt.Sprintf("/services/%s/team/%s", se.Name, s.team.Name)
	recorder, request := s.makeRequest(http.MethodPut, u, "", c)
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
	err := service.Create(context.TODO(), se)
	c.Assert(err, check.IsNil)
	u := fmt.Sprintf("/services/%s/team/%s", se.Name, s.team.Name)
	recorder, request := s.makeRequest(http.MethodDelete, u, "", c)
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	se, err = service.Get(context.TODO(), "my-service")
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
	recorder, request := s.makeRequest(http.MethodDelete, u, "", c)
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, "Service not found\n")
}

func (s *ProvisionSuite) TestRevokeAccessFromTeamReturnsForbiddenIfTheGivenUserDoesNotHasAccessToTheService(c *check.C) {
	t := authTypes.Team{Name: "alle-da"}
	se := service.Service{
		Name:       "my-service",
		OwnerTeams: []string{t.Name},
		Teams:      []string{t.Name},
		Endpoint:   map[string]string{"production": "http://localhost:1234"},
		Password:   "abcde",
	}
	err := service.Create(context.TODO(), se)
	c.Assert(err, check.IsNil)
	u := fmt.Sprintf("/services/%s/team/%s", se.Name, t.Name)
	recorder, request := s.makeRequest(http.MethodDelete, u, "", c)
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}

func (s *ProvisionSuite) TestRevokeServiceAccessFromTeamReturnsNotFoundIfTheTeamDoesNotExist(c *check.C) {
	s.mockTeamService.OnFindByName = func(_ string) (*authTypes.Team, error) {
		return nil, authTypes.ErrTeamNotFound
	}
	se := service.Service{
		Name:       "my-service",
		OwnerTeams: []string{s.team.Name},
		Teams:      []string{s.team.Name, "some-other"},
		Endpoint:   map[string]string{"production": "http://localhost:1234"},
		Password:   "abcde",
	}
	err := service.Create(context.TODO(), se)
	c.Assert(err, check.IsNil)
	u := fmt.Sprintf("/services/%s/team/nonono", se.Name)
	recorder, request := s.makeRequest(http.MethodDelete, u, "", c)
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
	err := service.Create(context.TODO(), se)
	c.Assert(err, check.IsNil)
	u := fmt.Sprintf("/services/%s/team/%s", se.Name, s.team.Name)
	recorder, request := s.makeRequest(http.MethodDelete, u, "", c)
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
	c.Assert(recorder.Body.String(), check.Equals, "You can not revoke the access from this team, because it is the unique team with access to this service, and a service can not be orphaned\n")
}

func (s *ProvisionSuite) TestRevokeServiceAccessFromTeamReturnNotFoundIfTheTeamDoesNotHasAccessToTheService(c *check.C) {
	t := authTypes.Team{Name: "rammlied"}
	se := service.Service{
		Name:       "my-service",
		OwnerTeams: []string{s.team.Name},
		Teams:      []string{s.team.Name, "other-team"},
		Endpoint:   map[string]string{"production": "http://localhost:1234"},
		Password:   "abcde",
	}
	err := service.Create(context.TODO(), se)
	c.Assert(err, check.IsNil)
	u := fmt.Sprintf("/services/%s/team/%s", se.Name, t.Name)
	recorder, request := s.makeRequest(http.MethodDelete, u, "", c)
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusConflict)
}

func (s *ProvisionSuite) TestAddDocServiceDoesNotExist(c *check.C) {
	v := url.Values{}
	v.Set("doc", "doc")
	recorder, request := s.makeRequest(http.MethodPut, "/services/mongodb/doc", v.Encode(), c)
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
	err := service.Create(context.TODO(), se)
	c.Assert(err, check.IsNil)
	v := url.Values{}
	v.Set("doc", "doc")
	recorder, request := s.makeRequest(http.MethodPut, "/services/some-service/doc", v.Encode(), c)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	query := mongoBSON.M{"_id": "some-service"}
	var serv service.Service

	servicesCollection, err := storagev2.ServicesCollection()
	c.Assert(err, check.IsNil)

	err = servicesCollection.FindOne(context.TODO(), query).Decode(&serv)
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
	se := service.Service{Name: "mysql", Endpoint: map[string]string{"production": "http://localhost:1234"}, Password: "abcde", OwnerTeams: []string{t.Name}}
	err := service.Create(context.TODO(), se)
	c.Assert(err, check.IsNil)
	v := url.Values{}
	v.Set("doc", "doc")
	recorder, request := s.makeRequest(http.MethodPut, "/services/mysql/doc", v.Encode(), c)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}
