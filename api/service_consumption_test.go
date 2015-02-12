// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/rec/rectest"
	"github.com/tsuru/tsuru/repository/repositorytest"
	"github.com/tsuru/tsuru/service"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

type ConsumptionSuite struct {
	conn  *db.Storage
	team  *auth.Team
	user  *auth.User
	token auth.Token
}

var _ = check.Suite(&ConsumptionSuite{})

func (s *ConsumptionSuite) SetUpSuite(c *check.C) {
	repositorytest.Reset()
	var err error
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "tsuru_api_consumption_test")
	config.Set("auth:hash-cost", 4)
	config.Set("repo-manager", "fake")
	s.conn, err = db.Conn()
	c.Assert(err, check.IsNil)
	s.user = &auth.User{Email: "whydidifall@thewho.com", Password: "123456"}
	_, err = nativeScheme.Create(s.user)
	c.Assert(err, check.IsNil)
	s.team = &auth.Team{Name: "tsuruteam", Users: []string{s.user.Email}}
	err = s.conn.Teams().Insert(s.team)
	c.Assert(err, check.IsNil)
	s.token, err = nativeScheme.Login(map[string]string{"email": s.user.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
	app.AuthScheme = nativeScheme
}

func (s *ConsumptionSuite) TearDownSuite(c *check.C) {
	dbtest.ClearAllCollections(s.conn.Apps().Database)
}

func (s *ConsumptionSuite) TearDownTest(c *check.C) {
	repositorytest.Reset()
	_, err := s.conn.Services().RemoveAll(nil)
	c.Assert(err, check.IsNil)
	_, err = s.conn.ServiceInstances().RemoveAll(nil)
	c.Assert(err, check.IsNil)
}

func makeRequestToCreateInstanceHandler(params map[string]string, c *check.C) (*httptest.ResponseRecorder, *http.Request) {
	var buf bytes.Buffer
	err := json.NewEncoder(&buf).Encode(params)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("POST", "/services/instances", &buf)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	return recorder, request
}

func (s *ConsumptionSuite) TestCreateInstanceWithPlan(c *check.C) {
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
	}
	recorder, request := makeRequestToCreateInstanceHandler(params, c)
	request.Header.Set("Content-Type", "application/json")
	err := createServiceInstance(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	var si service.ServiceInstance
	err = s.conn.ServiceInstances().Find(bson.M{
		"name":         "brainSQL",
		"service_name": "mysql",
		"plan_name":    "small",
	}).One(&si)
	c.Assert(err, check.IsNil)
	s.conn.ServiceInstances().Update(bson.M{"name": si.Name}, si)
	c.Assert(si.Name, check.Equals, "brainSQL")
	c.Assert(si.ServiceName, check.Equals, "mysql")
}

func (s *ConsumptionSuite) TestCreateInstanceHandlerSavesServiceInstanceInDb(c *check.C) {
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
		"owner":        s.team.Name,
	}
	recorder, request := makeRequestToCreateInstanceHandler(params, c)
	err := createServiceInstance(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	var si service.ServiceInstance
	err = s.conn.ServiceInstances().Find(bson.M{"name": "brainSQL", "service_name": "mysql"}).One(&si)
	c.Assert(err, check.IsNil)
	s.conn.ServiceInstances().Update(bson.M{"name": si.Name}, si)
	c.Assert(si.Name, check.Equals, "brainSQL")
	c.Assert(si.ServiceName, check.Equals, "mysql")
	c.Assert(si.TeamOwner, check.Equals, s.team.Name)
}

func (s *ConsumptionSuite) TestCreateInstanceHandlerSavesAllTeamsThatTheGivenUserIsMemberAndHasAccessToTheServiceInTheInstance(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"DATABASE_HOST":"localhost"}`))
	}))
	defer ts.Close()
	t := auth.Team{Name: "judaspriest", Users: []string{s.user.Email}}
	err := s.conn.Teams().Insert(t)
	defer s.conn.Teams().Remove(bson.M{"name": t.Name})
	srv := service.Service{Name: "mysql", Teams: []string{s.team.Name}, IsRestricted: true, Endpoint: map[string]string{"production": ts.URL}}
	err = srv.Create()
	c.Assert(err, check.IsNil)
	params := map[string]string{
		"name":         "brainSQL",
		"service_name": "mysql",
		"owner":        s.team.Name,
	}
	recorder, request := makeRequestToCreateInstanceHandler(params, c)
	err = createServiceInstance(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	var si service.ServiceInstance
	err = s.conn.ServiceInstances().Find(bson.M{"name": "brainSQL"}).One(&si)
	c.Assert(err, check.IsNil)
	s.conn.ServiceInstances().Update(bson.M{"name": si.Name}, si)
	c.Assert(si.Teams, check.DeepEquals, []string{s.team.Name})
}

func (s *ConsumptionSuite) TestCreateInstanceHandlerReturnsErrorWhenUserCannotUseService(c *check.C) {
	service := service.Service{Name: "mysql", IsRestricted: true}
	service.Create()
	params := map[string]string{
		"name":         "brainSQL",
		"service_name": "mysql",
		"owner":        s.team.Name,
	}
	recorder, request := makeRequestToCreateInstanceHandler(params, c)
	err := createServiceInstance(recorder, request, s.token)
	c.Assert(err, check.ErrorMatches, "^This user does not have access to this service$")
}

func (s *ConsumptionSuite) TestCreateInstanceHandlerIgnoresTeamAuthIfServiceIsNotRestricted(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"DATABASE_HOST":"localhost"}`))
	}))
	defer ts.Close()
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}}
	err := srvc.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": "mysql"})
	params := map[string]string{
		"name":         "brainSQL",
		"service_name": "mysql",
		"owner":        s.team.Name,
	}
	recorder, request := makeRequestToCreateInstanceHandler(params, c)
	err = createServiceInstance(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	var si service.ServiceInstance
	err = s.conn.ServiceInstances().Find(bson.M{"name": "brainSQL"}).One(&si)
	c.Assert(err, check.IsNil)
	c.Assert(si.Name, check.Equals, "brainSQL")
	c.Assert(si.Teams, check.DeepEquals, []string{s.team.Name})
}

func (s *ConsumptionSuite) TestCreateInstanceHandlerReturnsErrorWhenServiceDoesntExists(c *check.C) {
	params := map[string]string{
		"name":         "brainSQL",
		"service_name": "mysql",
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
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}}
	err := srvc.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": "mysql"})
	params := map[string]string{
		"name":         "brainSQL",
		"service_name": "mysql",
		"owner":        s.team.Name,
	}
	recorder, request := makeRequestToCreateInstanceHandler(params, c)
	err = createServiceInstance(recorder, request, s.token)
	c.Assert(err, check.NotNil)
}

func makeRequestToRemoveInstanceHandler(name string, c *check.C) (*httptest.ResponseRecorder, *http.Request) {
	url := fmt.Sprintf("/services/c/instances/%s?:name=%s", name, name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	return recorder, request
}

func (s *ConsumptionSuite) TestRemoveServiceInstanceHandler(c *check.C) {
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
	recorder, request := makeRequestToRemoveInstanceHandler("foo-instance", c)
	err = removeServiceInstance(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	b, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, check.IsNil)
	c.Assert(string(b), check.Equals, "service instance successfuly removed")
	n, err := s.conn.ServiceInstances().Find(bson.M{"name": "foo-instance"}).Count()
	c.Assert(err, check.IsNil)
	c.Assert(n, check.Equals, 0)
	action := rectest.Action{
		Action: "remove-service-instance",
		User:   s.user.Email,
		Extra:  []interface{}{"foo-instance"},
	}
	c.Assert(action, rectest.IsRecorded)
}

func (s *ConsumptionSuite) TestRemoveServiceHandlerWithoutPermissionShouldReturn401(c *check.C) {
	se := service.Service{Name: "foo"}
	err := se.Create()
	defer s.conn.Services().Remove(bson.M{"_id": se.Name})
	c.Assert(err, check.IsNil)
	si := service.ServiceInstance{Name: "foo-instance", ServiceName: "foo"}
	err = si.Create()
	defer s.conn.ServiceInstances().Remove(bson.M{"name": si.Name})
	c.Assert(err, check.IsNil)
	recorder, request := makeRequestToRemoveInstanceHandler("foo-instance", c)
	err = removeServiceInstance(recorder, request, s.token)
	c.Assert(err.Error(), check.Equals, service.ErrAccessNotAllowed.Error())
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
	recorder, request := makeRequestToRemoveInstanceHandler("foo-instance", c)
	err = removeServiceInstance(recorder, request, s.token)
	c.Assert(err, check.ErrorMatches, "^This service instance is bound to at least one app. Unbind them before removing it$")
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
	recorder, request := makeRequestToRemoveInstanceHandler("purity-instance", c)
	err = removeServiceInstance(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	c.Assert(called, check.Equals, true)
}

func (s *ConsumptionSuite) TestServicesInstancesHandler(c *check.C) {
	srv := service.Service{Name: "redis", Teams: []string{s.team.Name}}
	err := srv.Create()
	c.Assert(err, check.IsNil)
	instance := service.ServiceInstance{
		Name:        "redis-globo",
		ServiceName: "redis",
		Apps:        []string{"globo"},
		Teams:       []string{s.team.Name},
	}
	err = instance.Create()
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("GET", "/services/instances", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = serviceInstances(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, check.IsNil)
	var instances []service.ServiceModel
	err = json.Unmarshal(body, &instances)
	c.Assert(err, check.IsNil)
	expected := []service.ServiceModel{
		{Service: "redis", Instances: []string{"redis-globo"}},
	}
	c.Assert(instances, check.DeepEquals, expected)
	action := rectest.Action{Action: "list-service-instances", User: s.user.Email}
	c.Assert(action, rectest.IsRecorded)
}

func (s *ConsumptionSuite) TestServicesInstancesHandlerAppFilter(c *check.C) {
	srv := service.Service{Name: "redis", Teams: []string{s.team.Name}}
	err := srv.Create()
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
	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, check.IsNil)
	var instances []service.ServiceModel
	err = json.Unmarshal(body, &instances)
	c.Assert(err, check.IsNil)
	expected := []service.ServiceModel{
		{Service: "redis", Instances: []string{}},
		{Service: "mongodb", Instances: []string{"mongodb-other"}},
	}
	c.Assert(instances, check.DeepEquals, expected)
	action := rectest.Action{Action: "list-service-instances", User: s.user.Email}
	c.Assert(action, rectest.IsRecorded)
}

func (s *ConsumptionSuite) TestServicesInstancesHandlerReturnsOnlyServicesThatTheUserHasAccess(c *check.C) {
	u := &auth.User{Email: "me@globo.com", Password: "123456"}
	_, err := nativeScheme.Create(u)
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
	recorder := httptest.NewRecorder()
	err = serviceInstances(recorder, request, token)
	c.Assert(err, check.IsNil)
	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, check.IsNil)
	var instances []service.ServiceModel
	err = json.Unmarshal(body, &instances)
	c.Assert(err, check.IsNil)
	c.Assert(instances, check.DeepEquals, []service.ServiceModel{})
}

func (s *ConsumptionSuite) TestServicesInstancesHandlerFilterInstancesPerServiceIncludingServicesThatDoesNotHaveInstances(c *check.C) {
	serviceNames := []string{"redis", "mysql", "pgsql", "memcached"}
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
	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, check.IsNil)
	var instances []service.ServiceModel
	err = json.Unmarshal(body, &instances)
	c.Assert(err, check.IsNil)
	expected := []service.ServiceModel{
		{Service: "redis", Instances: []string{"redis1", "redis2"}},
		{Service: "mysql", Instances: []string{"mysql1", "mysql2"}},
		{Service: "pgsql", Instances: []string{"pgsql1", "pgsql2"}},
		{Service: "memcached", Instances: []string{"memcached1", "memcached2"}},
		{Service: "oracle", Instances: []string{}},
	}
	c.Assert(instances, check.DeepEquals, expected)
}

func makeRequestToStatusHandler(name string, c *check.C) (*httptest.ResponseRecorder, *http.Request) {
	url := fmt.Sprintf("/services/instances/%s/status/?:instance=%s", name, name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	return recorder, request
}

func (s *ConsumptionSuite) TestServiceInstanceStatusHandler(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	defer service.DeleteInstance(&si)
	recorder, request := makeRequestToStatusHandler("my_nosql", c)
	err = serviceInstanceStatus(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	b, err := ioutil.ReadAll(recorder.Body)
	c.Assert(string(b), check.Equals, "Service instance \"my_nosql\" is up")
	action := rectest.Action{
		Action: "service-instance-status",
		User:   s.user.Email,
		Extra:  []interface{}{"my_nosql"},
	}
	c.Assert(action, rectest.IsRecorded)
}

func (s *ConsumptionSuite) TestServiceInstanceStatusHandlerShouldReturnErrorWhenServiceInstanceNotExists(c *check.C) {
	recorder, request := makeRequestToStatusHandler("inexistent-instance", c)
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
	defer service.DeleteInstance(&si)
	recorder, request := makeRequestToStatusHandler("my_nosql", c)
	err = serviceInstanceStatus(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusForbidden)
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
	defer service.DeleteInstance(&si1)
	si2 := service.ServiceInstance{
		Name:        "your_nosql",
		ServiceName: srv.Name,
		Apps:        []string{"wordpress"},
		Teams:       []string{s.team.Name},
	}
	err = si2.Create()
	c.Assert(err, check.IsNil)
	defer service.DeleteInstance(&si2)
	request, err := http.NewRequest("GET", "/services/mongodb?:name=mongodb", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = serviceInfo(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, check.IsNil)
	var instances []service.ServiceInstance
	err = json.Unmarshal(body, &instances)
	c.Assert(err, check.IsNil)
	expected := []service.ServiceInstance{si1, si2}
	c.Assert(instances, check.DeepEquals, expected)
	action := rectest.Action{
		Action: "service-info",
		User:   s.user.Email,
		Extra:  []interface{}{"mongodb"},
	}
	c.Assert(action, rectest.IsRecorded)
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
	defer service.DeleteInstance(&si1)
	si2 := service.ServiceInstance{
		Name:        "your_nosql",
		ServiceName: srv.Name,
		Apps:        []string{"wordpress"},
		Teams:       []string{},
	}
	err = si2.Create()
	c.Assert(err, check.IsNil)
	defer service.DeleteInstance(&si2)
	request, err := http.NewRequest("GET", fmt.Sprintf("/services/%s?:name=%s", "mongodb", "mongodb"), nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = serviceInfo(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, check.IsNil)
	var instances []service.ServiceInstance
	err = json.Unmarshal(body, &instances)
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

func (s *ConsumptionSuite) TestServiceInfoHandlerReturns403WhenTheUserDoesNotHaveAccessToTheService(c *check.C) {
	se := service.Service{Name: "Mysql", IsRestricted: true}
	se.Create()
	defer s.conn.Services().Remove(bson.M{"_id": se.Name})
	request, err := http.NewRequest("DELETE", fmt.Sprintf("/services/%s?:name=%s", se.Name, se.Name), nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = serviceInfo(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusForbidden)
	c.Assert(e, check.ErrorMatches, "^This user does not have access to this service$")
}

func (s *ConsumptionSuite) TestGetServiceInstance(c *check.C) {
	instance := service.ServiceInstance{
		Name:        "mongo-1",
		ServiceName: "mongodb",
		Teams:       []string{s.team.Name},
		Apps:        []string{"myapp"},
	}
	s.conn.ServiceInstances().Insert(instance)
	defer s.conn.ServiceInstances().Remove(instance)
	request, _ := http.NewRequest("GET", "/services/instances/mongo-1?:name=mongo-1", nil)
	recorder := httptest.NewRecorder()
	err := serviceInstance(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	var got service.ServiceInstance
	err = json.NewDecoder(recorder.Body).Decode(&got)
	c.Assert(err, check.IsNil)
	c.Assert(got, check.DeepEquals, instance)
}

func (s *ConsumptionSuite) TestGetServiceInstanceNotFound(c *check.C) {
	request, _ := http.NewRequest("GET", "/services/instances/mongo-1?:name=mongo-1", nil)
	recorder := httptest.NewRecorder()
	err := serviceInstance(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusNotFound)
	c.Assert(e.Message, check.Equals, service.ErrServiceInstanceNotFound.Error())
}

func (s *ConsumptionSuite) TestGetServiceInstanceForbidden(c *check.C) {
	instance := service.ServiceInstance{
		Name:        "mongo-1",
		ServiceName: "mongodb",
		Apps:        []string{"myapp"},
	}
	s.conn.ServiceInstances().Insert(instance)
	defer s.conn.ServiceInstances().Remove(instance)
	request, _ := http.NewRequest("GET", "/services/instances/mongo-1?:name=mongo-1", nil)
	recorder := httptest.NewRecorder()
	err := serviceInstance(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusForbidden)
	c.Assert(e.Message, check.Equals, service.ErrAccessNotAllowed.Error())
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
	b, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, check.IsNil)
	c.Assert(string(b), check.Equals, doc)
	action := rectest.Action{
		Action: "service-doc",
		User:   s.user.Email,
		Extra:  []interface{}{"coolnosql"},
	}
	c.Assert(action, rectest.IsRecorded)
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
	c.Assert(err, check.ErrorMatches, "^This user does not have access to this service$")
}

func (s *ConsumptionSuite) TestDocHandlerReturns404WhenServiceDoesNotExists(c *check.C) {
	recorder, request := s.makeRequestToGetDocHandler("inexistentsql", c)
	err := serviceDoc(recorder, request, s.token)
	c.Assert(err, check.ErrorMatches, "^Service not found$")
}

func (s *ConsumptionSuite) TestGetServiceOrError(c *check.C) {
	srv := service.Service{Name: "foo", Teams: []string{s.team.Name}, IsRestricted: true}
	err := srv.Create()
	c.Assert(err, check.IsNil)
	rSrv, err := getServiceOrError("foo", s.user)
	c.Assert(err, check.IsNil)
	c.Assert(rSrv.Name, check.Equals, srv.Name)
}

func (s *ConsumptionSuite) TestGetServiceOrErrorShouldReturnErrorWhenUserHaveNoAccessToService(c *check.C) {
	srv := service.Service{Name: "foo", IsRestricted: true}
	err := srv.Create()
	c.Assert(err, check.IsNil)
	_, err = getServiceOrError("foo", s.user)
	c.Assert(err, check.ErrorMatches, "^This user does not have access to this service$")
}

func (s *ConsumptionSuite) TestGetServiceOrErrorShoudNotReturnErrorWhenServiceIsNotRestricted(c *check.C) {
	srv := service.Service{Name: "foo"}
	err := srv.Create()
	c.Assert(err, check.IsNil)
	_, err = getServiceOrError("foo", s.user)
	c.Assert(err, check.IsNil)
}

func (s *ConsumptionSuite) TestGetServiceInstanceOrError(c *check.C) {
	si := service.ServiceInstance{Name: "foo", Teams: []string{s.team.Name}}
	err := si.Create()
	c.Assert(err, check.IsNil)
	rSi, err := getServiceInstanceOrError("foo", s.user)
	c.Assert(err, check.IsNil)
	c.Assert(rSi.Name, check.Equals, si.Name)
}

func (s *ConsumptionSuite) TestServicePlansHandler(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		content := `[{"name": "ignite", "description": "some value"}, {"name": "small", "description": "not space left for you"}]`
		w.Write([]byte(content))
	}))
	defer ts.Close()
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}}
	err := srvc.Create()
	c.Assert(err, check.IsNil)
	defer srvc.Delete()
	request, err := http.NewRequest("GET", "/services/mysql/plans?:name=mysql", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = servicePlans(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, check.IsNil)
	var plans []service.Plan
	err = json.Unmarshal(body, &plans)
	c.Assert(err, check.IsNil)
	expected := []service.Plan{
		{Name: "ignite", Description: "some value"},
		{Name: "small", Description: "not space left for you"},
	}
	c.Assert(plans, check.DeepEquals, expected)
	action := rectest.Action{
		Action: "service-plans",
		User:   s.user.Email,
		Extra:  []interface{}{"mysql"},
	}
	c.Assert(action, rectest.IsRecorded)
}

func (s *ConsumptionSuite) TestServiceProxy(c *check.C) {
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
	defer service.DeleteInstance(&si)
	url := fmt.Sprintf("/services/proxy/%s?callback=/mypath", si.Name)
	request, err := http.NewRequest("GET", url, nil)
	reqAuth := "bearer " + s.token.GetValue()
	request.Header.Set("Authorization", reqAuth)
	request.Header.Set("X-Custom", "my request header")
	m := RunServer(true)
	recorder := httptest.NewRecorder()
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
	c.Assert(recorder.Header().Get("X-Response-Custom"), check.Equals, "custom response header")
	c.Assert(recorder.Body.String(), check.Equals, "a message")
	c.Assert(proxyedRequest, check.NotNil)
	c.Assert(proxyedRequest.Header.Get("X-Custom"), check.Equals, "my request header")
	c.Assert(proxyedRequest.Header.Get("Authorization"), check.Not(check.Equals), reqAuth)
	c.Assert(proxyedRequest.URL.String(), check.Equals, "/mypath")
}

func (s *ConsumptionSuite) TestServiceProxyNoContent(c *check.C) {
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
	defer service.DeleteInstance(&si)
	url := fmt.Sprintf("/services/proxy/%s?:instance=%s&callback=/mypath", si.Name, si.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = serviceProxy(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Code, check.Equals, http.StatusNoContent)
}

func (s *ConsumptionSuite) TestServiceProxyError(c *check.C) {
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
	defer service.DeleteInstance(&si)
	url := fmt.Sprintf("/services/proxy/%s?:instance=%s&callback=/mypath", si.Name, si.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = serviceProxy(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Code, check.Equals, http.StatusBadGateway)
	c.Assert(recorder.Body.Bytes(), check.DeepEquals, []byte("some error"))
}
