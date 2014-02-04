// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/auth"
	"github.com/globocom/tsuru/db"
	"github.com/globocom/tsuru/errors"
	"github.com/globocom/tsuru/service"
	"github.com/globocom/tsuru/testing"
	"io/ioutil"
	"labix.org/v2/mgo/bson"
	"launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
)

type ConsumptionSuite struct {
	conn  *db.Storage
	team  *auth.Team
	user  *auth.User
	token *auth.Token
}

var _ = gocheck.Suite(&ConsumptionSuite{})

func (s *ConsumptionSuite) SetUpSuite(c *gocheck.C) {
	var err error
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "tsuru_api_consumption_test")
	config.Set("auth:hash-cost", 4)
	s.conn, err = db.Conn()
	c.Assert(err, gocheck.IsNil)
	s.createUserAndTeam(c)
}

func (s *ConsumptionSuite) TearDownSuite(c *gocheck.C) {
	s.conn.Apps().Database.DropDatabase()
}

func (s *ConsumptionSuite) TearDownTest(c *gocheck.C) {
	_, err := s.conn.Services().RemoveAll(nil)
	c.Assert(err, gocheck.IsNil)
	_, err = s.conn.ServiceInstances().RemoveAll(nil)
	c.Assert(err, gocheck.IsNil)
}

func (s *ConsumptionSuite) createUserAndTeam(c *gocheck.C) {
	s.user = &auth.User{Email: "whydidifall@thewho.com", Password: "123456"}
	err := s.user.Create()
	c.Assert(err, gocheck.IsNil)
	s.team = &auth.Team{Name: "tsuruteam", Users: []string{s.user.Email}}
	err = s.conn.Teams().Insert(s.team)
	c.Assert(err, gocheck.IsNil)
	s.token, err = s.user.CreateToken("123456")
	c.Assert(err, gocheck.IsNil)
}

func makeRequestToCreateInstanceHandler(c *gocheck.C) (*httptest.ResponseRecorder, *http.Request) {
	b := bytes.NewBufferString(`{"name":"brainSQL","service_name":"mysql"}`)
	request, err := http.NewRequest("POST", "/services/instances", b)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	return recorder, request
}

func (s *ConsumptionSuite) TestCreateInstanceWithPlan(c *gocheck.C) {
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
	data := `{"name":"brainSQL","service_name":"mysql","plan":"small"}`
	b := bytes.NewBufferString(data)
	request, err := http.NewRequest("POST", "/services/instances", b)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	err = createServiceInstance(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	var si service.ServiceInstance
	err = s.conn.ServiceInstances().Find(bson.M{
		"name":         "brainSQL",
		"service_name": "mysql",
		"plan_name":    "small",
	}).One(&si)
	c.Assert(err, gocheck.IsNil)
	s.conn.ServiceInstances().Update(bson.M{"name": si.Name}, si)
	c.Assert(si.Name, gocheck.Equals, "brainSQL")
	c.Assert(si.ServiceName, gocheck.Equals, "mysql")
	action := testing.Action{
		Action: "create-service-instance",
		User:   s.user.Email,
		Extra:  []interface{}{data},
	}
	c.Assert(action, testing.IsRecorded)
}

func (s *ConsumptionSuite) TestCreateInstanceHandlerSavesServiceInstanceInDb(c *gocheck.C) {
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
	recorder, request := makeRequestToCreateInstanceHandler(c)
	err := createServiceInstance(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	var si service.ServiceInstance
	err = s.conn.ServiceInstances().Find(bson.M{"name": "brainSQL", "service_name": "mysql"}).One(&si)
	c.Assert(err, gocheck.IsNil)
	s.conn.ServiceInstances().Update(bson.M{"name": si.Name}, si)
	c.Assert(si.Name, gocheck.Equals, "brainSQL")
	c.Assert(si.ServiceName, gocheck.Equals, "mysql")
	action := testing.Action{
		Action: "create-service-instance",
		User:   s.user.Email,
		Extra:  []interface{}{`{"name":"brainSQL","service_name":"mysql"}`},
	}
	c.Assert(action, testing.IsRecorded)
}

func (s *ConsumptionSuite) TestCreateInstanceHandlerSavesAllTeamsThatTheGivenUserIsMemberAndHasAccessToTheServiceInTheInstance(c *gocheck.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"DATABASE_HOST":"localhost"}`))
	}))
	defer ts.Close()
	t := auth.Team{Name: "judaspriest", Users: []string{s.user.Email}}
	err := s.conn.Teams().Insert(t)
	defer s.conn.Teams().Remove(bson.M{"name": t.Name})
	srv := service.Service{Name: "mysql", Teams: []string{s.team.Name}, IsRestricted: true, Endpoint: map[string]string{"production": ts.URL}}
	err = srv.Create()
	c.Assert(err, gocheck.IsNil)
	recorder, request := makeRequestToCreateInstanceHandler(c)
	err = createServiceInstance(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	var si service.ServiceInstance
	err = s.conn.ServiceInstances().Find(bson.M{"name": "brainSQL"}).One(&si)
	c.Assert(err, gocheck.IsNil)
	s.conn.ServiceInstances().Update(bson.M{"name": si.Name}, si)
	c.Assert(si.Teams, gocheck.DeepEquals, []string{s.team.Name})
}

func (s *ConsumptionSuite) TestCreateInstanceHandlerReturnsErrorWhenUserCannotUseService(c *gocheck.C) {
	service := service.Service{Name: "mysql", IsRestricted: true}
	service.Create()
	recorder, request := makeRequestToCreateInstanceHandler(c)
	err := createServiceInstance(recorder, request, s.token)
	c.Assert(err, gocheck.ErrorMatches, "^This user does not have access to this service$")
}

func (s *ConsumptionSuite) TestCreateInstanceHandlerIgnoresTeamAuthIfServiceIsNotRestricted(c *gocheck.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"DATABASE_HOST":"localhost"}`))
	}))
	defer ts.Close()
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}}
	err := srvc.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": "mysql"})
	recorder, request := makeRequestToCreateInstanceHandler(c)
	err = createServiceInstance(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	var si service.ServiceInstance
	err = s.conn.ServiceInstances().Find(bson.M{"name": "brainSQL"}).One(&si)
	c.Assert(err, gocheck.IsNil)
	c.Assert(si.Name, gocheck.Equals, "brainSQL")
	c.Assert(si.Teams, gocheck.DeepEquals, []string{s.team.Name})
}

func (s *ConsumptionSuite) TestCreateInstanceHandlerReturnsErrorWhenServiceDoesntExists(c *gocheck.C) {
	recorder, request := makeRequestToCreateInstanceHandler(c)
	err := createServiceInstance(recorder, request, s.token)
	c.Assert(err.Error(), gocheck.Equals, "Service not found")
}

func (s *ConsumptionSuite) TestCreateInstanceHandlerReturnErrorIfTheServiceAPICallFailAndDoesNotSaveTheInstanceInTheDatabase(c *gocheck.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}}
	err := srvc.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": "mysql"})
	recorder, request := makeRequestToCreateInstanceHandler(c)
	err = createServiceInstance(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
}

func makeRequestToRemoveInstanceHandler(name string, c *gocheck.C) (*httptest.ResponseRecorder, *http.Request) {
	url := fmt.Sprintf("/services/c/instances/%s?:name=%s", name, name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	return recorder, request
}

func (s *ConsumptionSuite) TestRemoveServiceInstanceHandler(c *gocheck.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	se := service.Service{Name: "foo", Endpoint: map[string]string{"production": ts.URL}}
	err := se.Create()
	defer s.conn.Services().Remove(bson.M{"_id": se.Name})
	c.Assert(err, gocheck.IsNil)
	si := service.ServiceInstance{Name: "foo-instance", ServiceName: "foo", Teams: []string{s.team.Name}}
	err = si.Create()
	c.Assert(err, gocheck.IsNil)
	recorder, request := makeRequestToRemoveInstanceHandler("foo-instance", c)
	err = removeServiceInstance(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	b, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, gocheck.IsNil)
	c.Assert(string(b), gocheck.Equals, "service instance successfuly removed")
	n, err := s.conn.ServiceInstances().Find(bson.M{"name": "foo-instance"}).Count()
	c.Assert(err, gocheck.IsNil)
	c.Assert(n, gocheck.Equals, 0)
	action := testing.Action{
		Action: "remove-service-instance",
		User:   s.user.Email,
		Extra:  []interface{}{"foo-instance"},
	}
	c.Assert(action, testing.IsRecorded)
}

func (s *ConsumptionSuite) TestRemoveServiceHandlerWithoutPermissionShouldReturn401(c *gocheck.C) {
	se := service.Service{Name: "foo"}
	err := se.Create()
	defer s.conn.Services().Remove(bson.M{"_id": se.Name})
	c.Assert(err, gocheck.IsNil)
	si := service.ServiceInstance{Name: "foo-instance", ServiceName: "foo"}
	err = si.Create()
	defer s.conn.ServiceInstances().Remove(bson.M{"name": si.Name})
	c.Assert(err, gocheck.IsNil)
	recorder, request := makeRequestToRemoveInstanceHandler("foo-instance", c)
	err = removeServiceInstance(recorder, request, s.token)
	c.Assert(err.Error(), gocheck.Equals, service.ErrAccessNotAllowed.Error())
}

func (s *ConsumptionSuite) TestRemoveServiceHandlerWIthAssociatedAppsShouldFailAndReturnError(c *gocheck.C) {
	se := service.Service{Name: "foo"}
	err := se.Create()
	defer s.conn.Services().Remove(bson.M{"_id": se.Name})
	c.Assert(err, gocheck.IsNil)
	si := service.ServiceInstance{Name: "foo-instance", ServiceName: "foo", Apps: []string{"foo-bar"}, Teams: []string{s.team.Name}}
	err = si.Create()
	defer s.conn.ServiceInstances().Remove(bson.M{"name": si.Name})
	c.Assert(err, gocheck.IsNil)
	recorder, request := makeRequestToRemoveInstanceHandler("foo-instance", c)
	err = removeServiceInstance(recorder, request, s.token)
	c.Assert(err, gocheck.ErrorMatches, "^This service instance is bound to at least one app. Unbind them before removing it$")
}

func (s *ConsumptionSuite) TestRemoveServiceShouldCallTheServiceAPI(c *gocheck.C) {
	var called bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = r.Method == "DELETE" && r.URL.Path == "/resources/purity-instance"
	}))
	defer ts.Close()
	se := service.Service{Name: "purity", Endpoint: map[string]string{"production": ts.URL}}
	err := se.Create()
	defer s.conn.Services().Remove(bson.M{"_id": se.Name})
	c.Assert(err, gocheck.IsNil)
	si := service.ServiceInstance{Name: "purity-instance", ServiceName: "purity", Teams: []string{s.team.Name}}
	err = si.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": si.Name})
	recorder, request := makeRequestToRemoveInstanceHandler("purity-instance", c)
	err = removeServiceInstance(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	c.Assert(called, gocheck.Equals, true)
}

func (s *ConsumptionSuite) TestServicesInstancesHandler(c *gocheck.C) {
	srv := service.Service{Name: "redis", Teams: []string{s.team.Name}}
	err := srv.Create()
	c.Assert(err, gocheck.IsNil)
	instance := service.ServiceInstance{
		Name:        "redis-globo",
		ServiceName: "redis",
		Apps:        []string{"globo"},
		Teams:       []string{s.team.Name},
	}
	err = instance.Create()
	c.Assert(err, gocheck.IsNil)
	request, err := http.NewRequest("GET", "/services/instances", nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = serviceInstances(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, gocheck.IsNil)
	var instances []service.ServiceModel
	err = json.Unmarshal(body, &instances)
	c.Assert(err, gocheck.IsNil)
	expected := []service.ServiceModel{
		{Service: "redis", Instances: []string{"redis-globo"}},
	}
	c.Assert(instances, gocheck.DeepEquals, expected)
	action := testing.Action{Action: "list-service-instances", User: s.user.Email}
	c.Assert(action, testing.IsRecorded)
}

func (s *ConsumptionSuite) TestServicesInstancesHandlerReturnsOnlyServicesThatTheUserHasAccess(c *gocheck.C) {
	u := &auth.User{Email: "me@globo.com", Password: "123456"}
	err := u.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	token, err := u.CreateToken("123456")
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Tokens().Remove(bson.M{"token": token.Token})
	srv := service.Service{Name: "redis", IsRestricted: true}
	err = s.conn.Services().Insert(srv)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": "redis"})
	instance := service.ServiceInstance{
		Name:        "redis-globo",
		ServiceName: "redis",
		Apps:        []string{"globo"},
	}
	err = instance.Create()
	c.Assert(err, gocheck.IsNil)
	request, err := http.NewRequest("GET", "/services/instances", nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = serviceInstances(recorder, request, token)
	c.Assert(err, gocheck.IsNil)
	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, gocheck.IsNil)
	var instances []service.ServiceModel
	err = json.Unmarshal(body, &instances)
	c.Assert(err, gocheck.IsNil)
	c.Assert(instances, gocheck.DeepEquals, []service.ServiceModel{})
}

func (s *ConsumptionSuite) TestServicesInstancesHandlerFilterInstancesPerServiceIncludingServicesThatDoesNotHaveInstances(c *gocheck.C) {
	u := &auth.User{Email: "me@globo.com", Password: "123"}
	err := u.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	serviceNames := []string{"redis", "mysql", "pgsql", "memcached"}
	defer s.conn.Services().RemoveAll(bson.M{"name": bson.M{"$in": serviceNames}})
	defer s.conn.ServiceInstances().RemoveAll(bson.M{"service_name": bson.M{"$in": serviceNames}})
	for _, name := range serviceNames {
		srv := service.Service{Name: name, Teams: []string{s.team.Name}}
		err = srv.Create()
		c.Assert(err, gocheck.IsNil)
		instance := service.ServiceInstance{
			Name:        srv.Name + "1",
			ServiceName: srv.Name,
			Teams:       []string{s.team.Name},
		}
		err = instance.Create()
		c.Assert(err, gocheck.IsNil)
		instance = service.ServiceInstance{
			Name:        srv.Name + "2",
			ServiceName: srv.Name,
			Teams:       []string{s.team.Name},
		}
		err = instance.Create()
	}
	srv := service.Service{Name: "oracle", Teams: []string{s.team.Name}}
	err = srv.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Services().Remove(bson.M{"name": "oracle"})
	request, err := http.NewRequest("GET", "/services/instances", nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = serviceInstances(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, gocheck.IsNil)
	var instances []service.ServiceModel
	err = json.Unmarshal(body, &instances)
	c.Assert(err, gocheck.IsNil)
	expected := []service.ServiceModel{
		{Service: "redis", Instances: []string{"redis1", "redis2"}},
		{Service: "mysql", Instances: []string{"mysql1", "mysql2"}},
		{Service: "pgsql", Instances: []string{"pgsql1", "pgsql2"}},
		{Service: "memcached", Instances: []string{"memcached1", "memcached2"}},
		{Service: "oracle", Instances: []string(nil)},
	}
	c.Assert(instances, gocheck.DeepEquals, expected)
}

func makeRequestToStatusHandler(name string, c *gocheck.C) (*httptest.ResponseRecorder, *http.Request) {
	url := fmt.Sprintf("/services/instances/%s/status/?:instance=%s", name, name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	return recorder, request
}

func (s *ConsumptionSuite) TestServiceInstanceStatusHandler(c *gocheck.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
		w.Write([]byte(`Service instance "my_nosql" is up`))
	}))
	defer ts.Close()
	srv := service.Service{Name: "mongodb", OwnerTeams: []string{s.team.Name}, Endpoint: map[string]string{"production": ts.URL}}
	err := srv.Create()
	c.Assert(err, gocheck.IsNil)
	defer srv.Delete()
	si := service.ServiceInstance{Name: "my_nosql", ServiceName: srv.Name, Teams: []string{s.team.Name}}
	err = si.Create()
	c.Assert(err, gocheck.IsNil)
	defer service.DeleteInstance(&si)
	recorder, request := makeRequestToStatusHandler("my_nosql", c)
	err = serviceInstanceStatus(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	b, err := ioutil.ReadAll(recorder.Body)
	c.Assert(string(b), gocheck.Equals, "Service instance \"my_nosql\" is up")
	action := testing.Action{
		Action: "service-instance-status",
		User:   s.user.Email,
		Extra:  []interface{}{"my_nosql"},
	}
	c.Assert(action, testing.IsRecorded)
}

func (s *ConsumptionSuite) TestServiceInstanceStatusHandlerShouldReturnErrorWhenServiceInstanceNotExists(c *gocheck.C) {
	recorder, request := makeRequestToStatusHandler("inexistent-instance", c)
	err := serviceInstanceStatus(recorder, request, s.token)
	c.Assert(err, gocheck.ErrorMatches, "^Service instance not found$")
}

func (s *ConsumptionSuite) TestServiceInstanceStatusHandlerShouldReturnForbiddenWhenUserDontHaveAccess(c *gocheck.C) {
	srv := service.Service{Name: "mongodb", OwnerTeams: []string{s.team.Name}}
	err := srv.Create()
	c.Assert(err, gocheck.IsNil)
	defer srv.Delete()
	si := service.ServiceInstance{Name: "my_nosql", ServiceName: srv.Name}
	err = si.Create()
	c.Assert(err, gocheck.IsNil)
	defer service.DeleteInstance(&si)
	recorder, request := makeRequestToStatusHandler("my_nosql", c)
	err = serviceInstanceStatus(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusForbidden)
}

func (s *ConsumptionSuite) TestServiceInfoHandler(c *gocheck.C) {
	srv := service.Service{Name: "mongodb", Teams: []string{s.team.Name}}
	err := srv.Create()
	c.Assert(err, gocheck.IsNil)
	defer srv.Delete()
	si1 := service.ServiceInstance{
		Name:        "my_nosql",
		ServiceName: srv.Name,
		Apps:        []string{},
		Teams:       []string{s.team.Name},
	}
	err = si1.Create()
	c.Assert(err, gocheck.IsNil)
	defer service.DeleteInstance(&si1)
	si2 := service.ServiceInstance{
		Name:        "your_nosql",
		ServiceName: srv.Name,
		Apps:        []string{"wordpress"},
		Teams:       []string{s.team.Name},
	}
	err = si2.Create()
	c.Assert(err, gocheck.IsNil)
	defer service.DeleteInstance(&si2)
	request, err := http.NewRequest("GET", "/services/mongodb?:name=mongodb", nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = serviceInfo(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, gocheck.IsNil)
	var instances []service.ServiceInstance
	err = json.Unmarshal(body, &instances)
	c.Assert(err, gocheck.IsNil)
	expected := []service.ServiceInstance{si1, si2}
	c.Assert(instances, gocheck.DeepEquals, expected)
	action := testing.Action{
		Action: "service-info",
		User:   s.user.Email,
		Extra:  []interface{}{"mongodb"},
	}
	c.Assert(action, testing.IsRecorded)
}

func (s *ConsumptionSuite) TestServiceInfoHandlerShouldReturnOnlyInstancesOfTheSameTeamOfTheUser(c *gocheck.C) {
	srv := service.Service{Name: "mongodb", Teams: []string{s.team.Name}}
	err := srv.Create()
	c.Assert(err, gocheck.IsNil)
	defer srv.Delete()
	si1 := service.ServiceInstance{
		Name:        "my_nosql",
		ServiceName: srv.Name,
		Apps:        []string{},
		Teams:       []string{s.team.Name},
	}
	err = si1.Create()
	c.Assert(err, gocheck.IsNil)
	defer service.DeleteInstance(&si1)
	si2 := service.ServiceInstance{
		Name:        "your_nosql",
		ServiceName: srv.Name,
		Apps:        []string{"wordpress"},
		Teams:       []string{},
	}
	err = si2.Create()
	c.Assert(err, gocheck.IsNil)
	defer service.DeleteInstance(&si2)
	request, err := http.NewRequest("GET", fmt.Sprintf("/services/%s?:name=%s", "mongodb", "mongodb"), nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = serviceInfo(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, gocheck.IsNil)
	var instances []service.ServiceInstance
	err = json.Unmarshal(body, &instances)
	c.Assert(err, gocheck.IsNil)
	expected := []service.ServiceInstance{si1}
	c.Assert(instances, gocheck.DeepEquals, expected)
}

func (s *ConsumptionSuite) TestServiceInfoHandlerReturns404WhenTheServiceDoesNotExist(c *gocheck.C) {
	request, err := http.NewRequest("GET", fmt.Sprintf("/services/%s?:name=%s", "mongodb", "mongodb"), nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = serviceInfo(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusNotFound)
	c.Assert(e, gocheck.ErrorMatches, "^Service not found$")
}

func (s *ConsumptionSuite) TestServiceInfoHandlerReturns403WhenTheUserDoesNotHaveAccessToTheService(c *gocheck.C) {
	se := service.Service{Name: "Mysql", IsRestricted: true}
	se.Create()
	defer s.conn.Services().Remove(bson.M{"_id": se.Name})
	request, err := http.NewRequest("DELETE", fmt.Sprintf("/services/%s?:name=%s", se.Name, se.Name), nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = serviceInfo(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusForbidden)
	c.Assert(e, gocheck.ErrorMatches, "^This user does not have access to this service$")
}

func (s *ConsumptionSuite) TestGetServiceInstance(c *gocheck.C) {
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
	c.Assert(err, gocheck.IsNil)
	var got service.ServiceInstance
	err = json.NewDecoder(recorder.Body).Decode(&got)
	c.Assert(err, gocheck.IsNil)
	c.Assert(got, gocheck.DeepEquals, instance)
}

func (s *ConsumptionSuite) TestGetServiceInstanceNotFound(c *gocheck.C) {
	request, _ := http.NewRequest("GET", "/services/instances/mongo-1?:name=mongo-1", nil)
	recorder := httptest.NewRecorder()
	err := serviceInstance(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusNotFound)
	c.Assert(e.Message, gocheck.Equals, service.ErrServiceInstanceNotFound.Error())
}

func (s *ConsumptionSuite) TestGetServiceInstanceForbidden(c *gocheck.C) {
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
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusForbidden)
	c.Assert(e.Message, gocheck.Equals, service.ErrAccessNotAllowed.Error())
}

func (s *ConsumptionSuite) makeRequestToGetDocHandler(name string, c *gocheck.C) (*httptest.ResponseRecorder, *http.Request) {
	url := fmt.Sprintf("/services/%s/doc/?:name=%s", name, name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	return recorder, request
}

func (s *ConsumptionSuite) TestDocHandler(c *gocheck.C) {
	doc := `Doc for coolnosql
Collnosql is a really really cool nosql`
	srv := service.Service{
		Name:  "coolnosql",
		Doc:   doc,
		Teams: []string{s.team.Name},
	}
	err := srv.Create()
	c.Assert(err, gocheck.IsNil)
	recorder, request := s.makeRequestToGetDocHandler("coolnosql", c)
	err = serviceDoc(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	b, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, gocheck.IsNil)
	c.Assert(string(b), gocheck.Equals, doc)
	action := testing.Action{
		Action: "service-doc",
		User:   s.user.Email,
		Extra:  []interface{}{"coolnosql"},
	}
	c.Assert(action, testing.IsRecorded)
}

func (s *ConsumptionSuite) TestDocHandlerReturns401WhenUserHasNoAccessToService(c *gocheck.C) {
	srv := service.Service{
		Name:         "coolnosql",
		Doc:          "some doc...",
		IsRestricted: true,
	}
	err := srv.Create()
	c.Assert(err, gocheck.IsNil)
	recorder, request := s.makeRequestToGetDocHandler("coolnosql", c)
	err = serviceDoc(recorder, request, s.token)
	c.Assert(err, gocheck.ErrorMatches, "^This user does not have access to this service$")
}

func (s *ConsumptionSuite) TestDocHandlerReturns404WhenServiceDoesNotExists(c *gocheck.C) {
	recorder, request := s.makeRequestToGetDocHandler("inexistentsql", c)
	err := serviceDoc(recorder, request, s.token)
	c.Assert(err, gocheck.ErrorMatches, "^Service not found$")
}

func (s *ConsumptionSuite) TestGetServiceOrError(c *gocheck.C) {
	srv := service.Service{Name: "foo", Teams: []string{s.team.Name}, IsRestricted: true}
	err := srv.Create()
	c.Assert(err, gocheck.IsNil)
	rSrv, err := getServiceOrError("foo", s.user)
	c.Assert(err, gocheck.IsNil)
	c.Assert(rSrv.Name, gocheck.Equals, srv.Name)
}

func (s *ConsumptionSuite) TestGetServiceOrErrorShouldReturnErrorWhenUserHaveNoAccessToService(c *gocheck.C) {
	srv := service.Service{Name: "foo", IsRestricted: true}
	err := srv.Create()
	c.Assert(err, gocheck.IsNil)
	_, err = getServiceOrError("foo", s.user)
	c.Assert(err, gocheck.ErrorMatches, "^This user does not have access to this service$")
}

func (s *ConsumptionSuite) TestGetServiceOrErrorShoudNotReturnErrorWhenServiceIsNotRestricted(c *gocheck.C) {
	srv := service.Service{Name: "foo"}
	err := srv.Create()
	c.Assert(err, gocheck.IsNil)
	_, err = getServiceOrError("foo", s.user)
	c.Assert(err, gocheck.IsNil)
}

func (s *ConsumptionSuite) TestGetServiceInstanceOrError(c *gocheck.C) {
	si := service.ServiceInstance{Name: "foo", Teams: []string{s.team.Name}}
	err := si.Create()
	c.Assert(err, gocheck.IsNil)
	rSi, err := getServiceInstanceOrError("foo", s.user)
	c.Assert(err, gocheck.IsNil)
	c.Assert(rSi.Name, gocheck.Equals, si.Name)
}

func (s *ConsumptionSuite) TestServicePlansHandler(c *gocheck.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		content := `[{"name": "ignite", "description": "some value"}, {"name": "small", "description": "not space left for you"}]`
		w.Write([]byte(content))
	}))
	defer ts.Close()
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}}
	err := srvc.Create()
	c.Assert(err, gocheck.IsNil)
	defer srvc.Delete()
	request, err := http.NewRequest("GET", "/services/mysql/plans?:name=mysql", nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = servicePlans(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, gocheck.IsNil)
	var plans []service.Plan
	err = json.Unmarshal(body, &plans)
	c.Assert(err, gocheck.IsNil)
	expected := []service.Plan{
		{Name: "ignite", Description: "some value"},
		{Name: "small", Description: "not space left for you"},
	}
	c.Assert(plans, gocheck.DeepEquals, expected)
	action := testing.Action{
		Action: "service-plans",
		User:   s.user.Email,
		Extra:  []interface{}{"mysql"},
	}
	c.Assert(action, testing.IsRecorded)
}
