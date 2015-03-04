// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/app/bind"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/rec/rectest"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

type InstanceSuite struct {
	conn            *db.Storage
	service         *Service
	serviceInstance *ServiceInstance
	team            *auth.Team
	user            *auth.User
}

var _ = check.Suite(&InstanceSuite{})

func (s *InstanceSuite) SetUpSuite(c *check.C) {
	var err error
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "tsuru_service_instance_test")
	config.Set("admin-team", "admin")
	s.conn, err = db.Conn()
	c.Assert(err, check.IsNil)
	s.user = &auth.User{Email: "cidade@raul.com", Password: "123"}
	s.team = &auth.Team{Name: "Raul", Users: []string{s.user.Email}}
	s.conn.Users().Insert(s.user)
	s.conn.Teams().Insert(s.team)
}

func (s *InstanceSuite) TearDownSuite(c *check.C) {
	dbtest.ClearAllCollections(s.conn.Apps().Database)
	s.conn.Close()
}

func (s *InstanceSuite) TestDeleteServiceInstance(c *check.C) {
	si := &ServiceInstance{Name: "MySQL"}
	s.conn.ServiceInstances().Insert(&si)
	DeleteInstance(si)
	query := bson.M{"name": si.Name}
	qtd, err := s.conn.ServiceInstances().Find(query).Count()
	c.Assert(err, check.IsNil)
	c.Assert(qtd, check.Equals, 0)
}

func (s *InstanceSuite) TestRetrieveAssociatedService(c *check.C) {
	service := Service{Name: "my_service"}
	s.conn.Services().Insert(&service)
	defer s.conn.Services().RemoveId(service.Name)
	serviceInstance := &ServiceInstance{
		Name:        service.Name,
		ServiceName: service.Name,
	}
	rService := serviceInstance.Service()
	c.Assert(service.Name, check.Equals, rService.Name)
}

func (s *InstanceSuite) TestAddApp(c *check.C) {
	instance := ServiceInstance{
		Name: "myinstance",
		Apps: []string{},
	}
	err := instance.AddApp("app1")
	c.Assert(err, check.IsNil)
	c.Assert(instance.Apps, check.DeepEquals, []string{"app1"})
}

func (s *InstanceSuite) TestAddAppReturnErrorIfTheAppIsAlreadyPresent(c *check.C) {
	instance := ServiceInstance{
		Name: "myinstance",
		Apps: []string{"app1"},
	}
	err := instance.AddApp("app1")
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, "^This instance already has this app.$")
}

func (s *InstanceSuite) TestFindApp(c *check.C) {
	instance := ServiceInstance{
		Name: "myinstance",
		Apps: []string{"app1", "app2"},
	}
	c.Assert(instance.FindApp("app1"), check.Equals, 0)
	c.Assert(instance.FindApp("app2"), check.Equals, 1)
	c.Assert(instance.FindApp("what"), check.Equals, -1)
}

func (s *InstanceSuite) TestRemoveApp(c *check.C) {
	instance := ServiceInstance{
		Name: "myinstance",
		Apps: []string{"app1", "app2", "app3"},
	}
	err := instance.RemoveApp("app2")
	c.Assert(err, check.IsNil)
	c.Assert(instance.Apps, check.DeepEquals, []string{"app1", "app3"})
	err = instance.RemoveApp("app3")
	c.Assert(err, check.IsNil)
	c.Assert(instance.Apps, check.DeepEquals, []string{"app1"})
}

func (s *InstanceSuite) TestRemoveAppReturnsErrorWhenTheAppIsNotBoundToTheInstance(c *check.C) {
	instance := ServiceInstance{
		Name: "myinstance",
		Apps: []string{"app1", "app2", "app3"},
	}
	err := instance.RemoveApp("app4")
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, "^This app is not bound to this service instance.$")
}

func (s *InstanceSuite) TestBindApp(c *check.C) {
	oldAddAppToServiceInstance := addAppToServiceInstance
	oldSetBindAppAction := setBindAppAction
	oldSetTsuruServices := setTsuruServices
	oldBindUnitsToServiceInstance := bindUnitsToServiceInstance
	defer func() {
		addAppToServiceInstance = oldAddAppToServiceInstance
		setBindAppAction = oldSetBindAppAction
		setTsuruServices = oldSetTsuruServices
		bindUnitsToServiceInstance = oldBindUnitsToServiceInstance
	}()
	var calls []string
	var params []interface{}
	addAppToServiceInstance = action.Action{
		Forward: func(ctx action.FWContext) (action.Result, error) {
			calls = append(calls, "addAppToServiceInstance")
			params = ctx.Params
			return nil, nil
		},
	}
	setBindAppAction = action.Action{
		Forward: func(ctx action.FWContext) (action.Result, error) {
			calls = append(calls, "setBindAppAction")
			return nil, nil
		},
	}
	setTsuruServices = action.Action{
		Forward: func(ctx action.FWContext) (action.Result, error) {
			calls = append(calls, "setTsuruServices")
			return nil, nil
		},
	}
	bindUnitsToServiceInstance = action.Action{
		Forward: func(ctx action.FWContext) (action.Result, error) {
			calls = append(calls, "bindUnitsToServiceInstance")
			return nil, nil
		},
	}
	var si ServiceInstance
	a := provisiontest.NewFakeApp("myapp", "python", 1)
	var buf bytes.Buffer
	err := si.BindApp(a, &buf)
	c.Assert(err, check.IsNil)
	expectedCalls := []string{
		"addAppToServiceInstance", "setBindAppAction",
		"setTsuruServices", "bindUnitsToServiceInstance",
	}
	expectedParams := []interface{}{a, si, &buf}
	c.Assert(calls, check.DeepEquals, expectedCalls)
	c.Assert(params, check.DeepEquals, expectedParams)
	c.Assert(buf.String(), check.Equals, "")
}

func (s *InstanceSuite) TestUnbindApp(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"DATABASE_USER":"root","DATABASE_PASSWORD":"s3cr3t"}`))
	}))
	defer ts.Close()
	service := Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}}
	err := service.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.Services().RemoveId(service.Name)
	a := provisiontest.NewFakeApp("myapp", "static", 1)
	si := ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
		Apps:        []string{a.GetName()},
	}
	err = si.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.ServiceInstances().RemoveId(si.Name)
	instance := bind.ServiceInstance{Name: si.Name}
	err = a.AddInstance(si.ServiceName, instance, nil)
	c.Assert(err, check.IsNil)
	var buf bytes.Buffer
	err = si.UnbindApp(a, &buf)
	c.Assert(err, check.IsNil)
	c.Assert(a.GetInstances("mysql"), check.HasLen, 0)
	c.Assert(buf.String(), check.Equals, "remove instance")
}

func (s *InstanceSuite) TestGetServiceInstancesByServices(c *check.C) {
	srvc := Service{Name: "mysql"}
	err := s.conn.Services().Insert(&srvc)
	c.Assert(err, check.IsNil)
	defer s.conn.Services().RemoveId(srvc.Name)
	sInstance := ServiceInstance{Name: "t3sql", ServiceName: "mysql", Teams: []string{s.team.Name}}
	err = s.conn.ServiceInstances().Insert(&sInstance)
	c.Assert(err, check.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": sInstance.Name})
	sInstance2 := ServiceInstance{Name: "s9sql", ServiceName: "mysql"}
	err = s.conn.ServiceInstances().Insert(&sInstance2)
	c.Assert(err, check.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": sInstance2.Name})
	sInstances, err := GetServiceInstancesByServices([]Service{srvc})
	c.Assert(err, check.IsNil)
	expected := []ServiceInstance{{Name: "t3sql", ServiceName: "mysql"}, sInstance2}
	c.Assert(sInstances, check.DeepEquals, expected)
}

func (s *InstanceSuite) TestGetServiceInstancesByServicesWithoutAnyExistingServiceInstances(c *check.C) {
	srvc := Service{Name: "mysql"}
	err := s.conn.Services().Insert(&srvc)
	c.Assert(err, check.IsNil)
	defer s.conn.Services().RemoveId(srvc.Name)
	sInstances, err := GetServiceInstancesByServices([]Service{srvc})
	c.Assert(err, check.IsNil)
	c.Assert(sInstances, check.DeepEquals, []ServiceInstance(nil))
}

func (s *InstanceSuite) TestGetServiceInstancesByServicesWithTwoServices(c *check.C) {
	srvc := Service{Name: "mysql"}
	err := s.conn.Services().Insert(&srvc)
	c.Assert(err, check.IsNil)
	defer s.conn.Services().RemoveId(srvc.Name)
	srvc2 := Service{Name: "mongodb"}
	err = s.conn.Services().Insert(&srvc2)
	c.Assert(err, check.IsNil)
	defer s.conn.Services().RemoveId(srvc2.Name)
	sInstance := ServiceInstance{Name: "t3sql", ServiceName: "mysql", Teams: []string{s.team.Name}}
	err = s.conn.ServiceInstances().Insert(&sInstance)
	c.Assert(err, check.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": sInstance.Name})
	sInstance2 := ServiceInstance{Name: "s9nosql", ServiceName: "mongodb"}
	err = s.conn.ServiceInstances().Insert(&sInstance2)
	c.Assert(err, check.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": sInstance2.Name})
	sInstances, err := GetServiceInstancesByServices([]Service{srvc, srvc2})
	c.Assert(err, check.IsNil)
	expected := []ServiceInstance{{Name: "t3sql", ServiceName: "mysql"}, sInstance2}
	c.Assert(sInstances, check.DeepEquals, expected)
}

func (s *InstanceSuite) TestGenericServiceInstancesFilter(c *check.C) {
	srvc := Service{Name: "mysql"}
	teams := []string{s.team.Name}
	query := genericServiceInstancesFilter(srvc, teams)
	c.Assert(query, check.DeepEquals, bson.M{"service_name": srvc.Name, "teams": bson.M{"$in": teams}})
}

func (s *InstanceSuite) TestGenericServiceInstancesFilterWithServiceSlice(c *check.C) {
	services := []Service{
		{Name: "mysql"},
		{Name: "mongodb"},
	}
	names := []string{"mysql", "mongodb"}
	teams := []string{s.team.Name}
	query := genericServiceInstancesFilter(services, teams)
	c.Assert(query, check.DeepEquals, bson.M{"service_name": bson.M{"$in": names}, "teams": bson.M{"$in": teams}})
}

func (s *InstanceSuite) TestGenericServiceInstancesFilterWithoutSpecifingTeams(c *check.C) {
	services := []Service{
		{Name: "mysql"},
		{Name: "mongodb"},
	}
	names := []string{"mysql", "mongodb"}
	teams := []string{}
	query := genericServiceInstancesFilter(services, teams)
	c.Assert(query, check.DeepEquals, bson.M{"service_name": bson.M{"$in": names}})
}

func (s *InstanceSuite) TestGetServiceInstancesByServicesAndTeams(c *check.C) {
	srvc := Service{Name: "mysql", Teams: []string{s.team.Name}, IsRestricted: true}
	err := s.conn.Services().Insert(&srvc)
	c.Assert(err, check.IsNil)
	defer s.conn.Services().RemoveId(srvc.Name)
	srvc2 := Service{Name: "mongodb", Teams: []string{s.team.Name}, IsRestricted: false}
	err = s.conn.Services().Insert(&srvc2)
	c.Assert(err, check.IsNil)
	defer s.conn.Services().RemoveId(srvc2.Name)
	sInstance := ServiceInstance{
		Name:        "j4sql",
		ServiceName: srvc.Name,
		Teams:       []string{s.team.Name},
		Apps:        []string{},
	}
	err = s.conn.ServiceInstances().Insert(&sInstance)
	c.Assert(err, check.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": sInstance.Name})
	sInstance2 := ServiceInstance{
		Name:        "j4nosql",
		ServiceName: srvc2.Name,
		Teams:       []string{s.team.Name},
		Apps:        []string{},
	}
	err = s.conn.ServiceInstances().Insert(&sInstance2)
	c.Assert(err, check.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": sInstance2.Name})
	sInstance3 := ServiceInstance{
		Name:        "f9nosql",
		ServiceName: srvc2.Name,
	}
	err = s.conn.ServiceInstances().Insert(&sInstance3)
	c.Assert(err, check.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": sInstance3.Name})
	expected := []ServiceInstance{sInstance, sInstance2}
	sInstances, err := GetServiceInstancesByServicesAndTeams([]Service{srvc, srvc2}, s.user, "")
	c.Assert(err, check.IsNil)
	c.Assert(sInstances, check.DeepEquals, expected)
}

func (s *InstanceSuite) TestGetServiceInstancesByServicesAndTeamsAppFilter(c *check.C) {
	srvc := Service{Name: "mysql", Teams: []string{s.team.Name}, IsRestricted: true}
	err := s.conn.Services().Insert(&srvc)
	c.Assert(err, check.IsNil)
	defer s.conn.Services().RemoveId(srvc.Name)
	srvc2 := Service{Name: "mongodb", Teams: []string{s.team.Name}, IsRestricted: false}
	err = s.conn.Services().Insert(&srvc2)
	c.Assert(err, check.IsNil)
	defer s.conn.Services().RemoveId(srvc2.Name)
	sInstance := ServiceInstance{
		Name:        "j4sql",
		ServiceName: srvc.Name,
		Teams:       []string{s.team.Name},
		Apps:        []string{"app1"},
	}
	err = s.conn.ServiceInstances().Insert(&sInstance)
	c.Assert(err, check.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": sInstance.Name})
	sInstance2 := ServiceInstance{
		Name:        "j4nosql",
		ServiceName: srvc2.Name,
		Teams:       []string{s.team.Name},
		Apps:        []string{},
	}
	err = s.conn.ServiceInstances().Insert(&sInstance2)
	c.Assert(err, check.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": sInstance2.Name})
	expected := []ServiceInstance{sInstance}
	sInstances, err := GetServiceInstancesByServicesAndTeams([]Service{srvc, srvc2}, s.user, "app1")
	c.Assert(err, check.IsNil)
	c.Assert(sInstances, check.DeepEquals, expected)
}

func (s *InstanceSuite) TestGetServiceInstancesByServicesAndTeamsForUsersThatAreNotMembersOfAnyTeam(c *check.C) {
	u := auth.User{Email: "noteamforme@globo.com"}
	err := u.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	srvc := Service{Name: "mysql", Teams: []string{s.team.Name}, IsRestricted: true}
	err = s.conn.Services().Insert(&srvc)
	c.Assert(err, check.IsNil)
	defer s.conn.Services().RemoveId(srvc.Name)
	instance := ServiceInstance{
		Name:        "j4sql",
		ServiceName: srvc.Name,
		Teams:       []string{s.team.Name},
	}
	err = s.conn.ServiceInstances().Insert(&instance)
	c.Assert(err, check.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": instance.Name})
	instances, err := GetServiceInstancesByServicesAndTeams([]Service{srvc}, &u, "")
	c.Assert(err, check.IsNil)
	c.Assert(instances, check.IsNil)
}

func (s *InstanceSuite) TestGetServiceinstancesByServicesAndTeamsUserAdmin(c *check.C) {
	u := auth.User{Email: "adminuser@globo.com"}
	err := u.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	team := auth.Team{Name: "admin", Users: []string{u.Email}}
	err = s.conn.Teams().Insert(team)
	c.Assert(err, check.IsNil)
	defer s.conn.Teams().RemoveId(team.Name)
	srvc := Service{Name: "mysql", Teams: []string{s.team.Name}, IsRestricted: true}
	err = s.conn.Services().Insert(&srvc)
	c.Assert(err, check.IsNil)
	defer s.conn.Services().RemoveId(srvc.Name)
	instance := ServiceInstance{
		Name:        "j4sql",
		ServiceName: srvc.Name,
		Teams:       []string{s.team.Name},
		Apps:        []string{},
	}
	err = s.conn.ServiceInstances().Insert(&instance)
	c.Assert(err, check.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": instance.Name})
	instances, err := GetServiceInstancesByServicesAndTeams([]Service{srvc}, &u, "")
	c.Assert(err, check.IsNil)
	c.Assert(instances, check.DeepEquals, []ServiceInstance{instance})
}

func (s *InstanceSuite) TestAdditionalInfo(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"label": "key", "value": "value"}, {"label": "key2", "value": "value2"}]`))
	}))
	defer ts.Close()
	srvc := Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}}
	err := s.conn.Services().Insert(&srvc)
	c.Assert(err, check.IsNil)
	defer s.conn.Services().RemoveId(srvc.Name)
	si := ServiceInstance{Name: "ql", ServiceName: srvc.Name}
	info, err := si.Info()
	c.Assert(err, check.IsNil)
	expected := map[string]string{
		"key":  "value",
		"key2": "value2",
	}
	c.Assert(info, check.DeepEquals, expected)
}

func (s *InstanceSuite) TestMarshalJSON(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"label": "key", "value": "value"}]`))
	}))
	defer ts.Close()
	srvc := Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}}
	err := s.conn.Services().Insert(&srvc)
	c.Assert(err, check.IsNil)
	defer s.conn.Services().RemoveId(srvc.Name)
	si := ServiceInstance{Name: "ql", ServiceName: srvc.Name}
	data, err := json.Marshal(&si)
	c.Assert(err, check.IsNil)
	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	c.Assert(err, check.IsNil)
	expected := map[string]interface{}{
		"Id":          float64(0),
		"Name":        "ql",
		"Teams":       nil,
		"Apps":        nil,
		"ServiceName": "mysql",
		"Info":        map[string]interface{}{"key": "value"},
	}
	c.Assert(result, check.DeepEquals, expected)
}

func (s *InstanceSuite) TestMarshalJSONWithoutInfo(c *check.C) {
	srvc := Service{Name: "mysql", Endpoint: map[string]string{"production": ""}}
	err := s.conn.Services().Insert(&srvc)
	c.Assert(err, check.IsNil)
	defer s.conn.Services().RemoveId(srvc.Name)
	si := ServiceInstance{Name: "ql", ServiceName: srvc.Name}
	data, err := json.Marshal(&si)
	c.Assert(err, check.IsNil)
	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	c.Assert(err, check.IsNil)
	expected := map[string]interface{}{
		"Id":          float64(0),
		"Name":        "ql",
		"Teams":       nil,
		"Apps":        nil,
		"ServiceName": "mysql",
		"Info":        nil,
	}
	c.Assert(result, check.DeepEquals, expected)
}

func (s *InstanceSuite) TestMarshalJSONWithoutEndpoint(c *check.C) {
	srvc := Service{Name: "mysql"}
	err := s.conn.Services().Insert(&srvc)
	c.Assert(err, check.IsNil)
	defer s.conn.Services().RemoveId(srvc.Name)
	si := ServiceInstance{Name: "ql", ServiceName: srvc.Name}
	data, err := json.Marshal(&si)
	c.Assert(err, check.IsNil)
	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	c.Assert(err, check.IsNil)
	expected := map[string]interface{}{
		"Id":          float64(0),
		"Name":        "ql",
		"Teams":       nil,
		"Apps":        nil,
		"ServiceName": "mysql",
		"Info":        nil,
	}
	c.Assert(result, check.DeepEquals, expected)
}

func (s *InstanceSuite) TestDeleteInstance(c *check.C) {
	h := TestHandler{}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	srv := Service{Name: "mongodb", Endpoint: map[string]string{"production": ts.URL}}
	err := s.conn.Services().Insert(&srv)
	c.Assert(err, check.IsNil)
	defer s.conn.Services().RemoveId(srv.Name)
	si := ServiceInstance{Name: "instance", ServiceName: srv.Name}
	err = s.conn.ServiceInstances().Insert(&si)
	c.Assert(err, check.IsNil)
	err = DeleteInstance(&si)
	h.Lock()
	defer h.Unlock()
	c.Assert(err, check.IsNil)
	l, err := s.conn.ServiceInstances().Find(bson.M{"name": si.Name}).Count()
	c.Assert(err, check.IsNil)
	c.Assert(l, check.Equals, 0)
	c.Assert(h.url, check.Equals, "/resources/"+si.Name)
	c.Assert(h.method, check.Equals, "DELETE")
}

func (s *InstanceSuite) TestDeleteInstanceWithApps(c *check.C) {
	si := ServiceInstance{Name: "instance", Apps: []string{"foo"}}
	err := s.conn.ServiceInstances().Insert(&si)
	c.Assert(err, check.IsNil)
	s.conn.ServiceInstances().Remove(bson.M{"name": si.Name})
	err = DeleteInstance(&si)
	c.Assert(err, check.ErrorMatches, "^This service instance is bound to at least one app. Unbind them before removing it$")
}

func (s *InstanceSuite) TestCreateServiceInstance(c *check.C) {
	var requests int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
		atomic.AddInt32(&requests, 1)
	}))
	defer ts.Close()
	srv := Service{Name: "mongodb", Endpoint: map[string]string{"production": ts.URL}}
	err := s.conn.Services().Insert(&srv)
	c.Assert(err, check.IsNil)
	defer s.conn.Services().RemoveId(srv.Name)
	instance := ServiceInstance{Name: "instance", PlanName: "small"}
	err = CreateServiceInstance(instance, &srv, s.user)
	c.Assert(err, check.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": "instance"})
	si, err := GetServiceInstance("instance", s.user)
	c.Assert(err, check.IsNil)
	c.Assert(atomic.LoadInt32(&requests), check.Equals, int32(1))
	c.Assert(si.PlanName, check.Equals, "small")
	c.Assert(si.TeamOwner, check.Equals, s.team.Name)
	c.Assert(si.Teams, check.DeepEquals, []string{s.team.Name})
}

func (s *InstanceSuite) TestCreateSpecifyOwner(c *check.C) {
	var requests int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
		atomic.AddInt32(&requests, 1)
	}))
	defer ts.Close()
	team := auth.Team{Name: "owner", Users: []string{s.user.Email}}
	err := s.conn.Teams().Insert(team)
	defer s.conn.Teams().Remove(bson.M{"_id": team.Name})
	c.Assert(err, check.IsNil)
	srv := Service{Name: "mongodb", Endpoint: map[string]string{"production": ts.URL}}
	err = s.conn.Services().Insert(&srv)
	c.Assert(err, check.IsNil)
	defer s.conn.Services().RemoveId(srv.Name)
	instance := ServiceInstance{Name: "instance", PlanName: "small", TeamOwner: team.Name}
	err = CreateServiceInstance(instance, &srv, s.user)
	c.Assert(err, check.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": "instance"})
	si, err := GetServiceInstance("instance", s.user)
	c.Assert(err, check.IsNil)
	c.Assert(atomic.LoadInt32(&requests), check.Equals, int32(1))
	c.Assert(si.TeamOwner, check.Equals, team.Name)
}

func (s *InstanceSuite) TestCreateSpecifyOwnerUserNotInTeam(c *check.C) {
	team := auth.Team{Name: "owner"}
	err := s.conn.Teams().Insert(team)
	defer s.conn.Teams().Remove(bson.M{"_id": team.Name})
	c.Assert(err, check.IsNil)
	srv := Service{Name: "mongodb"}
	err = s.conn.Services().Insert(&srv)
	c.Assert(err, check.IsNil)
	defer s.conn.Services().RemoveId(srv.Name)
	instance := ServiceInstance{Name: "instance", PlanName: "small", TeamOwner: team.Name}
	err = CreateServiceInstance(instance, &srv, s.user)
	c.Assert(err, check.Equals, auth.ErrTeamNotFound)
}

func (s *InstanceSuite) TestCreateServiceInstanceMoreThanOneTeam(c *check.C) {
	var requests int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
		atomic.AddInt32(&requests, 1)
	}))
	defer ts.Close()
	team := auth.Team{Name: "owner", Users: []string{s.user.Email}}
	err := s.conn.Teams().Insert(team)
	defer s.conn.Teams().Remove(bson.M{"_id": team.Name})
	c.Assert(err, check.IsNil)
	srv := Service{Name: "mongodb", Endpoint: map[string]string{"production": ts.URL}}
	err = s.conn.Services().Insert(&srv)
	c.Assert(err, check.IsNil)
	defer s.conn.Services().RemoveId(srv.Name)
	instance := ServiceInstance{Name: "instance", PlanName: "small"}
	err = CreateServiceInstance(instance, &srv, s.user)
	c.Assert(err, check.Equals, ErrMultipleTeams)
}

func (s *InstanceSuite) TestCreateServiceInstanceNameShouldBeUnique(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()
	srv := Service{Name: "mongodb", Endpoint: map[string]string{"production": ts.URL}}
	err := s.conn.Services().Insert(&srv)
	c.Assert(err, check.IsNil)
	defer s.conn.Services().RemoveId(srv.Name)
	instance := ServiceInstance{Name: "instance"}
	err = CreateServiceInstance(instance, &srv, s.user)
	c.Assert(err, check.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": "instance"})
	err = CreateServiceInstance(instance, &srv, s.user)
	c.Assert(err, check.Equals, ErrInstanceNameAlreadyExists)
}

func (s *InstanceSuite) TestCreateServiceInstanceRestrictedService(c *check.C) {
	var requests int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
		atomic.AddInt32(&requests, 1)
	}))
	defer ts.Close()
	err := auth.CreateTeam("painkiller", s.user)
	c.Assert(err, check.IsNil)
	defer s.conn.Teams().RemoveId("painkiller")
	srv := Service{
		Name:         "mongodb",
		Endpoint:     map[string]string{"production": ts.URL},
		IsRestricted: true,
		Teams:        []string{"painkiller"},
	}
	err = s.conn.Services().Insert(&srv)
	c.Assert(err, check.IsNil)
	defer s.conn.Services().RemoveId(srv.Name)
	instance := &ServiceInstance{Name: "instance"}
	err = CreateServiceInstance(*instance, &srv, s.user)
	c.Assert(err, check.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": "instance"})
	instance, err = GetServiceInstance("instance", s.user)
	c.Assert(err, check.IsNil)
	c.Assert(instance.Teams, check.DeepEquals, []string{"painkiller"})
}

func (s *InstanceSuite) TestCreateServiceInstanceEndpointFailure(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()
	srv := Service{Name: "mongodb", Endpoint: map[string]string{"production": ts.URL}}
	err := s.conn.Services().Insert(&srv)
	c.Assert(err, check.IsNil)
	defer s.conn.Services().RemoveId(srv.Name)
	instance := ServiceInstance{Name: "instance"}
	err = CreateServiceInstance(instance, &srv, s.user)
	c.Assert(err, check.NotNil)
	count, err := s.conn.ServiceInstances().Find(bson.M{"name": "instance"}).Count()
	c.Assert(err, check.IsNil)
	c.Assert(count, check.Equals, 0)
}

func (s *InstanceSuite) TestCreateServiceInstanceValidatesTheName(c *check.C) {
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
	c.Assert(err, check.IsNil)
	defer s.conn.Services().RemoveId(srv.Name)
	for _, t := range tests {
		instance := ServiceInstance{Name: t.input}
		err := CreateServiceInstance(instance, &srv, s.user)
		c.Check(err, check.Equals, t.err)
		defer s.conn.ServiceInstances().Remove(bson.M{"name": t.input})
	}
}

func (s *InstanceSuite) TestStatus(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()
	srv := Service{Name: "mongodb", Endpoint: map[string]string{"production": ts.URL}}
	err := s.conn.Services().Insert(&srv)
	c.Assert(err, check.IsNil)
	defer s.conn.Services().RemoveId(srv.Name)
	si := ServiceInstance{Name: "instance", ServiceName: srv.Name}
	status, err := si.Status()
	c.Assert(err, check.IsNil)
	c.Assert(status, check.Equals, "up")
}

func (s *InstanceSuite) TestGetServiceInstance(c *check.C) {
	s.conn.ServiceInstances().Insert(
		ServiceInstance{Name: "mongo-1", ServiceName: "mongodb", Teams: []string{s.team.Name}},
		ServiceInstance{Name: "mongo-2", ServiceName: "mongodb", Teams: []string{s.team.Name}},
		ServiceInstance{Name: "mongo-3", ServiceName: "mongodb", Teams: []string{s.team.Name}},
		ServiceInstance{Name: "mongo-4", ServiceName: "mongodb", Teams: []string{s.team.Name}},
		ServiceInstance{Name: "mongo-5", ServiceName: "mongodb"},
	)
	defer s.conn.ServiceInstances().RemoveAll(bson.M{"service_name": "mongodb"})
	instance, err := GetServiceInstance("mongo-1", s.user)
	c.Assert(err, check.IsNil)
	c.Assert(instance.Name, check.Equals, "mongo-1")
	c.Assert(instance.ServiceName, check.Equals, "mongodb")
	c.Assert(instance.Teams, check.DeepEquals, []string{s.team.Name})
	action := rectest.Action{
		User:   s.user.Email,
		Action: "get-service-instance",
		Extra:  []interface{}{"mongo-1"},
	}
	c.Assert(action, rectest.IsRecorded)
	instance, err = GetServiceInstance("mongo-6", s.user)
	c.Assert(instance, check.IsNil)
	c.Assert(err, check.Equals, ErrServiceInstanceNotFound)
	instance, err = GetServiceInstance("mongo-5", s.user)
	c.Assert(instance, check.IsNil)
	c.Assert(err, check.Equals, ErrAccessNotAllowed)
}

func (s *InstanceSuite) TestProxy(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()
	srv := Service{Name: "mongodb", Endpoint: map[string]string{"production": ts.URL}}
	err := s.conn.Services().Insert(&srv)
	c.Assert(err, check.IsNil)
	defer s.conn.Services().RemoveId(srv.Name)
	si := ServiceInstance{Name: "instance", ServiceName: srv.Name}
	request, err := http.NewRequest("DELETE", "/something", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = Proxy(&si, "/aaa", recorder, request)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Code, check.Equals, http.StatusNoContent)
}

func (s *InstanceSuite) TestGetIdentfier(c *check.C) {
	srv := ServiceInstance{Name: "mongodb"}
	identifier := srv.GetIdentifier()
	c.Assert(identifier, check.Equals, srv.Name)
	srv.Id = 10
	identifier = srv.GetIdentifier()
	c.Assert(identifier, check.Equals, strconv.Itoa(srv.Id))
}
