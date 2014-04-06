// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import (
	"encoding/json"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app/bind"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/testing"
	"labix.org/v2/mgo/bson"
	"launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
)

type InstanceSuite struct {
	conn            *db.Storage
	service         *Service
	serviceInstance *ServiceInstance
	team            *auth.Team
	user            *auth.User
}

var _ = gocheck.Suite(&InstanceSuite{})

func (s *InstanceSuite) SetUpSuite(c *gocheck.C) {
	var err error
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "tsuru_service_instance_test")
	s.conn, err = db.Conn()
	c.Assert(err, gocheck.IsNil)
	s.user = &auth.User{Email: "cidade@raul.com", Password: "123"}
	s.team = &auth.Team{Name: "Raul", Users: []string{s.user.Email}}
	s.conn.Users().Insert(s.user)
	s.conn.Teams().Insert(s.team)
}

func (s *InstanceSuite) TearDownSuite(c *gocheck.C) {
	s.conn.Apps().Database.DropDatabase()
	s.conn.Close()
}

func (s *InstanceSuite) TestDeleteServiceInstance(c *gocheck.C) {
	si := &ServiceInstance{Name: "MySQL"}
	s.conn.ServiceInstances().Insert(&si)
	DeleteInstance(si)
	query := bson.M{"name": si.Name}
	qtd, err := s.conn.ServiceInstances().Find(query).Count()
	c.Assert(err, gocheck.IsNil)
	c.Assert(qtd, gocheck.Equals, 0)
}

func (s *InstanceSuite) TestRetrieveAssociatedService(c *gocheck.C) {
	service := Service{Name: "my_service"}
	s.conn.Services().Insert(&service)
	defer s.conn.Services().RemoveId(service.Name)
	serviceInstance := &ServiceInstance{
		Name:        service.Name,
		ServiceName: service.Name,
	}
	rService := serviceInstance.Service()
	c.Assert(service.Name, gocheck.Equals, rService.Name)
}

func (s *InstanceSuite) TestAddApp(c *gocheck.C) {
	instance := ServiceInstance{
		Name: "myinstance",
		Apps: []string{},
	}
	err := instance.AddApp("app1")
	c.Assert(err, gocheck.IsNil)
	c.Assert(instance.Apps, gocheck.DeepEquals, []string{"app1"})
}

func (s *InstanceSuite) TestAddAppReturnErrorIfTheAppIsAlreadyPresent(c *gocheck.C) {
	instance := ServiceInstance{
		Name: "myinstance",
		Apps: []string{"app1"},
	}
	err := instance.AddApp("app1")
	c.Assert(err, gocheck.NotNil)
	c.Assert(err, gocheck.ErrorMatches, "^This instance already has this app.$")
}

func (s *InstanceSuite) TestFindApp(c *gocheck.C) {
	instance := ServiceInstance{
		Name: "myinstance",
		Apps: []string{"app1", "app2"},
	}
	c.Assert(instance.FindApp("app1"), gocheck.Equals, 0)
	c.Assert(instance.FindApp("app2"), gocheck.Equals, 1)
	c.Assert(instance.FindApp("what"), gocheck.Equals, -1)
}

func (s *InstanceSuite) TestRemoveApp(c *gocheck.C) {
	instance := ServiceInstance{
		Name: "myinstance",
		Apps: []string{"app1", "app2", "app3"},
	}
	err := instance.RemoveApp("app2")
	c.Assert(err, gocheck.IsNil)
	c.Assert(instance.Apps, gocheck.DeepEquals, []string{"app1", "app3"})
	err = instance.RemoveApp("app3")
	c.Assert(err, gocheck.IsNil)
	c.Assert(instance.Apps, gocheck.DeepEquals, []string{"app1"})
}

func (s *InstanceSuite) TestRemoveAppReturnsErrorWhenTheAppIsNotBoundToTheInstance(c *gocheck.C) {
	instance := ServiceInstance{
		Name: "myinstance",
		Apps: []string{"app1", "app2", "app3"},
	}
	err := instance.RemoveApp("app4")
	c.Assert(err, gocheck.NotNil)
	c.Assert(err, gocheck.ErrorMatches, "^This app is not bound to this service instance.$")
}

func (s *InstanceSuite) TestServiceInstanceIsABinder(c *gocheck.C) {
	var _ bind.Binder = &ServiceInstance{}
}

func (s *InstanceSuite) TestGetServiceInstancesByServices(c *gocheck.C) {
	srvc := Service{Name: "mysql"}
	err := s.conn.Services().Insert(&srvc)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Services().RemoveId(srvc.Name)
	sInstance := ServiceInstance{Name: "t3sql", ServiceName: "mysql", Teams: []string{s.team.Name}}
	err = s.conn.ServiceInstances().Insert(&sInstance)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": sInstance.Name})
	sInstance2 := ServiceInstance{Name: "s9sql", ServiceName: "mysql"}
	err = s.conn.ServiceInstances().Insert(&sInstance2)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": sInstance2.Name})
	sInstances, err := GetServiceInstancesByServices([]Service{srvc})
	c.Assert(err, gocheck.IsNil)
	expected := []ServiceInstance{{Name: "t3sql", ServiceName: "mysql"}, sInstance2}
	c.Assert(sInstances, gocheck.DeepEquals, expected)
}

func (s *InstanceSuite) TestGetServiceInstancesByServicesWithoutAnyExistingServiceInstances(c *gocheck.C) {
	srvc := Service{Name: "mysql"}
	err := s.conn.Services().Insert(&srvc)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Services().RemoveId(srvc.Name)
	sInstances, err := GetServiceInstancesByServices([]Service{srvc})
	c.Assert(err, gocheck.IsNil)
	c.Assert(sInstances, gocheck.DeepEquals, []ServiceInstance(nil))
}

func (s *InstanceSuite) TestGetServiceInstancesByServicesWithTwoServices(c *gocheck.C) {
	srvc := Service{Name: "mysql"}
	err := s.conn.Services().Insert(&srvc)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Services().RemoveId(srvc.Name)
	srvc2 := Service{Name: "mongodb"}
	err = s.conn.Services().Insert(&srvc2)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Services().RemoveId(srvc2.Name)
	sInstance := ServiceInstance{Name: "t3sql", ServiceName: "mysql", Teams: []string{s.team.Name}}
	err = s.conn.ServiceInstances().Insert(&sInstance)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": sInstance.Name})
	sInstance2 := ServiceInstance{Name: "s9nosql", ServiceName: "mongodb"}
	err = s.conn.ServiceInstances().Insert(&sInstance2)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": sInstance2.Name})
	sInstances, err := GetServiceInstancesByServices([]Service{srvc, srvc2})
	c.Assert(err, gocheck.IsNil)
	expected := []ServiceInstance{{Name: "t3sql", ServiceName: "mysql"}, sInstance2}
	c.Assert(sInstances, gocheck.DeepEquals, expected)
}

func (s *InstanceSuite) TestGenericServiceInstancesFilter(c *gocheck.C) {
	srvc := Service{Name: "mysql"}
	teams := []string{s.team.Name}
	q, f := genericServiceInstancesFilter(srvc, teams)
	c.Assert(q, gocheck.DeepEquals, bson.M{"service_name": srvc.Name, "teams": bson.M{"$in": teams}})
	c.Assert(f, gocheck.DeepEquals, bson.M{"name": 1, "service_name": 1, "apps": 1})
}

func (s *InstanceSuite) TestGenericServiceInstancesFilterWithServiceSlice(c *gocheck.C) {
	services := []Service{
		{Name: "mysql"},
		{Name: "mongodb"},
	}
	names := []string{"mysql", "mongodb"}
	teams := []string{s.team.Name}
	q, f := genericServiceInstancesFilter(services, teams)
	c.Assert(q, gocheck.DeepEquals, bson.M{"service_name": bson.M{"$in": names}, "teams": bson.M{"$in": teams}})
	c.Assert(f, gocheck.DeepEquals, bson.M{"name": 1, "service_name": 1, "apps": 1})
}

func (s *InstanceSuite) TestGenericServiceInstancesFilterWithoutSpecifingTeams(c *gocheck.C) {
	services := []Service{
		{Name: "mysql"},
		{Name: "mongodb"},
	}
	names := []string{"mysql", "mongodb"}
	teams := []string{}
	q, f := genericServiceInstancesFilter(services, teams)
	c.Assert(q, gocheck.DeepEquals, bson.M{"service_name": bson.M{"$in": names}})
	c.Assert(f, gocheck.DeepEquals, bson.M{"name": 1, "service_name": 1, "apps": 1})
}

func (s *InstanceSuite) TestGetServiceInstancesByServicesAndTeams(c *gocheck.C) {
	srvc := Service{Name: "mysql", Teams: []string{s.team.Name}, IsRestricted: true}
	err := s.conn.Services().Insert(&srvc)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Services().RemoveId(srvc.Name)
	srvc2 := Service{Name: "mongodb", Teams: []string{s.team.Name}, IsRestricted: false}
	err = s.conn.Services().Insert(&srvc2)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Services().RemoveId(srvc2.Name)
	sInstance := ServiceInstance{
		Name:        "j4sql",
		ServiceName: srvc.Name,
		Teams:       []string{s.team.Name},
	}
	err = s.conn.ServiceInstances().Insert(&sInstance)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": sInstance.Name})
	sInstance2 := ServiceInstance{
		Name:        "j4nosql",
		ServiceName: srvc2.Name,
		Teams:       []string{s.team.Name},
	}
	err = s.conn.ServiceInstances().Insert(&sInstance2)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": sInstance2.Name})
	sInstance3 := ServiceInstance{
		Name:        "f9nosql",
		ServiceName: srvc2.Name,
	}
	err = s.conn.ServiceInstances().Insert(&sInstance3)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": sInstance3.Name})
	expected := []ServiceInstance{
		{
			Name:        sInstance.Name,
			ServiceName: sInstance.ServiceName,
			Teams:       []string(nil),
			Apps:        []string{},
		},
		{
			Name:        sInstance2.Name,
			ServiceName: sInstance2.ServiceName,
			Teams:       []string(nil),
			Apps:        []string{},
		},
	}
	sInstances, err := GetServiceInstancesByServicesAndTeams([]Service{srvc, srvc2}, s.user)
	c.Assert(err, gocheck.IsNil)
	c.Assert(sInstances, gocheck.DeepEquals, expected)
}

func (s *InstanceSuite) TestGetServiceInstancesByServicesAndTeamsForUsersThatAreNotMembersOfAnyTeam(c *gocheck.C) {
	u := auth.User{Email: "noteamforme@globo.com", Password: "123"}
	err := u.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	srvc := Service{Name: "mysql", Teams: []string{s.team.Name}, IsRestricted: true}
	err = s.conn.Services().Insert(&srvc)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Services().RemoveId(srvc.Name)
	instance := ServiceInstance{
		Name:        "j4sql",
		ServiceName: srvc.Name,
	}
	err = s.conn.ServiceInstances().Insert(&instance)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": instance.Name})
	instances, err := GetServiceInstancesByServicesAndTeams([]Service{srvc}, &u)
	c.Assert(err, gocheck.IsNil)
	c.Assert(instances, gocheck.IsNil)
}

func (s *InstanceSuite) TestAdditionalInfo(c *gocheck.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"label": "key", "value": "value"}, {"label": "key2", "value": "value2"}]`))
	}))
	defer ts.Close()
	srvc := Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}}
	err := s.conn.Services().Insert(&srvc)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Services().RemoveId(srvc.Name)
	si := ServiceInstance{Name: "ql", ServiceName: srvc.Name}
	info, err := si.Info()
	c.Assert(err, gocheck.IsNil)
	expected := map[string]string{
		"key":  "value",
		"key2": "value2",
	}
	c.Assert(info, gocheck.DeepEquals, expected)
}

func (s *InstanceSuite) TestMarshalJSON(c *gocheck.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"label": "key", "value": "value"}]`))
	}))
	defer ts.Close()
	srvc := Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}}
	err := s.conn.Services().Insert(&srvc)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Services().RemoveId(srvc.Name)
	si := ServiceInstance{Name: "ql", ServiceName: srvc.Name}
	data, err := json.Marshal(&si)
	c.Assert(err, gocheck.IsNil)
	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	c.Assert(err, gocheck.IsNil)
	expected := map[string]interface{}{
		"Name":        "ql",
		"Teams":       nil,
		"Apps":        nil,
		"ServiceName": "mysql",
		"Info":        map[string]interface{}{"key": "value"},
	}
	c.Assert(result, gocheck.DeepEquals, expected)
}

func (s *InstanceSuite) TestMarshalJSONWithoutInfo(c *gocheck.C) {
	srvc := Service{Name: "mysql", Endpoint: map[string]string{"production": ""}}
	err := s.conn.Services().Insert(&srvc)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Services().RemoveId(srvc.Name)
	si := ServiceInstance{Name: "ql", ServiceName: srvc.Name}
	data, err := json.Marshal(&si)
	c.Assert(err, gocheck.IsNil)
	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	c.Assert(err, gocheck.IsNil)
	expected := map[string]interface{}{
		"Name":        "ql",
		"Teams":       nil,
		"Apps":        nil,
		"ServiceName": "mysql",
		"Info":        nil,
	}
	c.Assert(result, gocheck.DeepEquals, expected)
}

func (s *InstanceSuite) TestMarshalJSONWithoutEndpoint(c *gocheck.C) {
	srvc := Service{Name: "mysql"}
	err := s.conn.Services().Insert(&srvc)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Services().RemoveId(srvc.Name)
	si := ServiceInstance{Name: "ql", ServiceName: srvc.Name}
	data, err := json.Marshal(&si)
	c.Assert(err, gocheck.IsNil)
	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	c.Assert(err, gocheck.IsNil)
	expected := map[string]interface{}{
		"Name":        "ql",
		"Teams":       nil,
		"Apps":        nil,
		"ServiceName": "mysql",
		"Info":        nil,
	}
	c.Assert(result, gocheck.DeepEquals, expected)
}

func (s *InstanceSuite) TestDeleteInstance(c *gocheck.C) {
	h := TestHandler{}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	srv := Service{Name: "mongodb", Endpoint: map[string]string{"production": ts.URL}}
	err := s.conn.Services().Insert(&srv)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Services().RemoveId(srv.Name)
	si := ServiceInstance{Name: "instance", ServiceName: srv.Name}
	err = s.conn.ServiceInstances().Insert(&si)
	c.Assert(err, gocheck.IsNil)
	err = DeleteInstance(&si)
	h.Lock()
	defer h.Unlock()
	c.Assert(err, gocheck.IsNil)
	l, err := s.conn.ServiceInstances().Find(bson.M{"name": si.Name}).Count()
	c.Assert(err, gocheck.IsNil)
	c.Assert(l, gocheck.Equals, 0)
	c.Assert(h.url, gocheck.Equals, "/resources/"+si.Name)
	c.Assert(h.method, gocheck.Equals, "DELETE")
}

func (s *InstanceSuite) TestDeleteInstanceWithApps(c *gocheck.C) {
	si := ServiceInstance{Name: "instance", Apps: []string{"foo"}}
	err := s.conn.ServiceInstances().Insert(&si)
	c.Assert(err, gocheck.IsNil)
	s.conn.ServiceInstances().Remove(bson.M{"name": si.Name})
	err = DeleteInstance(&si)
	c.Assert(err, gocheck.ErrorMatches, "^This service instance is bound to at least one app. Unbind them before removing it$")
}

func (s *InstanceSuite) TestCreateServiceInstance(c *gocheck.C) {
	var requests int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
		atomic.AddInt32(&requests, 1)
	}))
	defer ts.Close()
	srv := Service{Name: "mongodb", Endpoint: map[string]string{"production": ts.URL}}
	err := s.conn.Services().Insert(&srv)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Services().RemoveId(srv.Name)
	err = CreateServiceInstance("instance", &srv, "small", s.user)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": "instance"})
	si, err := GetServiceInstance("instance", s.user)
	c.Assert(err, gocheck.IsNil)
	c.Assert(atomic.LoadInt32(&requests), gocheck.Equals, int32(1))
	c.Assert(si.PlanName, gocheck.Equals, "small")
}

func (s *InstanceSuite) TestCreateServiceInstanceNameShouldBeUnique(c *gocheck.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()
	srv := Service{Name: "mongodb", Endpoint: map[string]string{"production": ts.URL}}
	err := s.conn.Services().Insert(&srv)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Services().RemoveId(srv.Name)
	err = CreateServiceInstance("instance", &srv, "", s.user)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": "instance"})
	err = CreateServiceInstance("instance", &srv, "", s.user)
	c.Assert(err, gocheck.Equals, ErrInstanceNameAlreadyExists)
}

func (s *InstanceSuite) TestCreateServiceInstanceRestrictedService(c *gocheck.C) {
	var requests int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
		atomic.AddInt32(&requests, 1)
	}))
	defer ts.Close()
	err := auth.CreateTeam("painkiller", s.user)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Teams().RemoveId("painkiller")
	srv := Service{
		Name:         "mongodb",
		Endpoint:     map[string]string{"production": ts.URL},
		IsRestricted: true,
		Teams:        []string{"painkiller"},
	}
	err = s.conn.Services().Insert(&srv)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Services().RemoveId(srv.Name)
	err = CreateServiceInstance("instance", &srv, "", s.user)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": "instance"})
	instance, err := GetServiceInstance("instance", s.user)
	c.Assert(err, gocheck.IsNil)
	c.Assert(instance.Teams, gocheck.DeepEquals, []string{"painkiller"})
}

func (s *InstanceSuite) TestCreateServiceInstanceEndpointFailure(c *gocheck.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()
	srv := Service{Name: "mongodb", Endpoint: map[string]string{"production": ts.URL}}
	err := s.conn.Services().Insert(&srv)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Services().RemoveId(srv.Name)
	err = CreateServiceInstance("instance", &srv, "", s.user)
	c.Assert(err, gocheck.NotNil)
	count, err := s.conn.ServiceInstances().Find(bson.M{"name": "instance"}).Count()
	c.Assert(err, gocheck.IsNil)
	c.Assert(count, gocheck.Equals, 0)
}

func (s *InstanceSuite) TestCreateServiceInstanceValidatesTheName(c *gocheck.C) {
	var tests = []struct {
		input string
		err   error
	}{
		{"my-service", nil},
		{"my_service", nil},
		{"my_service_123", nil},
		{"My_service_123", nil},
		{"a1", nil},
		{"--app", ErrInvalidInstanceName},
		{"123servico", ErrInvalidInstanceName},
		{"a", ErrInvalidInstanceName},
		{"a@123", ErrInvalidInstanceName},
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()
	srv := Service{Name: "mongodb", Endpoint: map[string]string{"production": ts.URL}}
	err := s.conn.Services().Insert(&srv)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Services().RemoveId(srv.Name)
	for _, t := range tests {
		err := CreateServiceInstance(t.input, &srv, "", s.user)
		if err != t.err {
			c.Errorf("Is %q valid? Want %#v. Got %#v", t.input, t.err, err)
		}
		defer s.conn.ServiceInstances().Remove(bson.M{"name": t.input})
	}
}

func (s *InstanceSuite) TestStatus(c *gocheck.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()
	srv := Service{Name: "mongodb", Endpoint: map[string]string{"production": ts.URL}}
	err := s.conn.Services().Insert(&srv)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Services().RemoveId(srv.Name)
	si := ServiceInstance{Name: "instance", ServiceName: srv.Name}
	status, err := si.Status()
	c.Assert(err, gocheck.IsNil)
	c.Assert(status, gocheck.Equals, "up")
}

func (s *InstanceSuite) TestGetServiceInstance(c *gocheck.C) {
	s.conn.ServiceInstances().Insert(
		ServiceInstance{Name: "mongo-1", ServiceName: "mongodb", Teams: []string{s.team.Name}},
		ServiceInstance{Name: "mongo-2", ServiceName: "mongodb", Teams: []string{s.team.Name}},
		ServiceInstance{Name: "mongo-3", ServiceName: "mongodb", Teams: []string{s.team.Name}},
		ServiceInstance{Name: "mongo-4", ServiceName: "mongodb", Teams: []string{s.team.Name}},
		ServiceInstance{Name: "mongo-5", ServiceName: "mongodb"},
	)
	defer s.conn.ServiceInstances().RemoveAll(bson.M{"service_name": "mongodb"})
	instance, err := GetServiceInstance("mongo-1", s.user)
	c.Assert(err, gocheck.IsNil)
	c.Assert(instance.Name, gocheck.Equals, "mongo-1")
	c.Assert(instance.ServiceName, gocheck.Equals, "mongodb")
	c.Assert(instance.Teams, gocheck.DeepEquals, []string{s.team.Name})
	action := testing.Action{
		User:   s.user.Email,
		Action: "get-service-instance",
		Extra:  []interface{}{"mongo-1"},
	}
	c.Assert(action, testing.IsRecorded)
	instance, err = GetServiceInstance("mongo-6", s.user)
	c.Assert(instance, gocheck.IsNil)
	c.Assert(err, gocheck.Equals, ErrServiceInstanceNotFound)
	instance, err = GetServiceInstance("mongo-5", s.user)
	c.Assert(instance, gocheck.IsNil)
	c.Assert(err, gocheck.Equals, ErrAccessNotAllowed)
}
