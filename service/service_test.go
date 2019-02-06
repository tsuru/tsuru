// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"sync/atomic"

	"github.com/globalsign/mgo/bson"
	osb "github.com/pmorie/go-open-service-broker-client/v2"
	osbfake "github.com/pmorie/go-open-service-broker-client/v2/fake"
	authTypes "github.com/tsuru/tsuru/types/auth"
	serviceTypes "github.com/tsuru/tsuru/types/service"
	check "gopkg.in/check.v1"
)

func (s *S) createService(c *check.C) {
	s.service = &Service{
		Name:     "my-service",
		Password: "my-password",
		Endpoint: map[string]string{
			"production": "http://localhost:8080",
		},
		OwnerTeams: []string{"admin"},
	}
	err := Create(*s.service)
	c.Assert(err, check.IsNil)
}

func (s *S) TestGetService(c *check.C) {
	s.createService(c)
	anotherService, err := Get(s.service.Name)
	c.Assert(err, check.IsNil)
	c.Assert(anotherService.Name, check.Equals, s.service.Name)
}

func (s *S) TestGetServiceReturnsErrorIfTheServiceIsDeleted(c *check.C) {
	_, err := Get("anything")
	c.Assert(err, check.NotNil)
}

func (s *S) TestGetServiceBrokered(c *check.C) {
	var calls int32
	s.mockService.ServiceBroker.OnFind = func(brokerName string) (serviceTypes.Broker, error) {
		atomic.AddInt32(&calls, 1)
		c.Assert(brokerName, check.Equals, "aws")
		return serviceTypes.Broker{Name: brokerName}, nil
	}
	s.mockService.ServiceBrokerCatalogCache.OnLoad = func(brokerName string) (*serviceTypes.BrokerCatalog, error) {
		atomic.AddInt32(&calls, 1)
		c.Assert(brokerName, check.Equals, "aws")
		return nil, nil
	}
	s.mockService.ServiceBrokerCatalogCache.OnSave = func(brokerName string, catalog serviceTypes.BrokerCatalog) error {
		atomic.AddInt32(&calls, 1)
		c.Assert(brokerName, check.Equals, "aws")
		c.Assert(catalog.Services, check.HasLen, 2)
		c.Assert(catalog.Services[0].Name, check.Equals, "otherservice")
		c.Assert(catalog.Services[1].Name, check.Equals, "service")
		c.Assert(catalog.Services[1].Description, check.Equals, "This service is awesome!")
		return nil
	}
	config := osbfake.FakeClientConfiguration{
		CatalogReaction: &osbfake.CatalogReaction{Response: &osb.CatalogResponse{
			Services: []osb.Service{
				{Name: "otherservice"},
				{Name: "service", Description: "This service is awesome!"},
			},
		}},
	}
	ClientFactory = osbfake.NewFakeClientFunc(config)
	serv, err := Get("aws::service")
	c.Assert(err, check.IsNil)
	c.Assert(serv, check.DeepEquals, Service{
		Name: "aws::service",
		Doc:  "This service is awesome!",
	})
	c.Assert(atomic.LoadInt32(&calls), check.Equals, int32(3))
}

func (s *S) TestGetServiceBrokeredFromCache(c *check.C) {
	s.mockService.ServiceBroker.OnFind = func(brokerName string) (serviceTypes.Broker, error) {
		c.Assert(brokerName, check.Equals, "aws")
		return serviceTypes.Broker{Name: brokerName}, nil
	}
	s.mockService.ServiceBrokerCatalogCache.OnLoad = func(brokerName string) (*serviceTypes.BrokerCatalog, error) {
		c.Assert(brokerName, check.Equals, "aws")
		return &serviceTypes.BrokerCatalog{
			Services: []serviceTypes.BrokerService{
				{
					ID:          "123",
					Name:        "service",
					Description: "cached service",
				},
			},
		}, nil
	}
	serv, err := Get("aws::service")
	c.Assert(err, check.IsNil)
	c.Assert(serv, check.DeepEquals, Service{
		Name: "aws::service",
		Doc:  "cached service",
	})
}

func (s *S) TestGetServiceBrokeredServiceBrokerNotFound(c *check.C) {
	s.mockService.ServiceBroker.OnFind = func(brokerName string) (serviceTypes.Broker, error) {
		c.Assert(brokerName, check.Equals, "broker")
		return serviceTypes.Broker{}, serviceTypes.ErrServiceBrokerNotFound
	}
	serv, err := Get("broker::service")
	c.Assert(err, check.DeepEquals, serviceTypes.ErrServiceBrokerNotFound)
	c.Assert(serv, check.DeepEquals, Service{})
}

func (s *S) TestGetServiceBrokeredServiceNotFound(c *check.C) {
	config := osbfake.FakeClientConfiguration{
		CatalogReaction: &osbfake.CatalogReaction{Response: &osb.CatalogResponse{}},
	}
	ClientFactory = osbfake.NewFakeClientFunc(config)
	sb, err := BrokerService()
	c.Assert(err, check.IsNil)
	err = sb.Create(serviceTypes.Broker{Name: "aws"})
	c.Assert(err, check.IsNil)
	serv, err := Get("aws::service")
	c.Assert(err, check.DeepEquals, ErrServiceNotFound)
	c.Assert(serv, check.DeepEquals, Service{})
}

func (s *S) TestCreateService(c *check.C) {
	endpt := map[string]string{
		"production": "somehost.com",
	}
	service := &Service{
		Name:       "my-service",
		Username:   "test",
		Endpoint:   endpt,
		OwnerTeams: []string{s.team.Name},
		Password:   "abcde",
	}
	err := Create(*service)
	c.Assert(err, check.IsNil)
	se, err := Get(service.Name)
	c.Assert(err, check.IsNil)
	c.Assert(se.Name, check.Equals, service.Name)
	c.Assert(se.Endpoint["production"], check.Equals, endpt["production"])
	c.Assert(se.OwnerTeams, check.DeepEquals, []string{s.team.Name})
	c.Assert(se.IsRestricted, check.Equals, false)
	c.Assert(se.Username, check.Equals, "test")
}

func (s *S) TestCreateServiceValidation(c *check.C) {
	endpt := map[string]string{
		"production": "somehost.com",
	}
	service := &Service{
		Username:   "test",
		Endpoint:   endpt,
		OwnerTeams: []string{s.team.Name},
		Password:   "abcde",
	}
	err := Create(*service)
	c.Assert(err, check.ErrorMatches, "Service id is required")
	service.Name = "INVALID NAME"
	err = Create(*service)
	c.Assert(err, check.ErrorMatches, "Invalid service id, should have at most 40 characters, containing only lower case letters, numbers or dashes, starting with a letter.")
	service.Name = "a-very-loooooooooooong-name-41-characters"
	err = Create(*service)
	c.Assert(err, check.ErrorMatches, "Invalid service id, should have at most 40 characters, containing only lower case letters, numbers or dashes, starting with a letter.")
	service.Name = "servicename"
	service.Password = ""
	err = Create(*service)
	c.Assert(err, check.ErrorMatches, "Service password is required")
	service.Password = "abcde"
	service.Endpoint = nil
	err = Create(*service)
	c.Assert(err, check.ErrorMatches, "Service production endpoint is required")
	service.Endpoint = endpt
	service.OwnerTeams = []string{}
	err = Create(*service)
	c.Assert(err, check.ErrorMatches, "At least one service team owner is required")
	service.OwnerTeams = []string{"unknown-team", s.team.Name}
	err = Create(*service)
	c.Assert(err, check.ErrorMatches, "Team owner doesn't exist")
	service.OwnerTeams = []string{s.team.Name, ""}
	err = Create(*service)
	c.Assert(err, check.ErrorMatches, "Team owner doesn't exist")
}

func (s *S) TestDeleteService(c *check.C) {
	s.createService(c)
	err := Delete(*s.service)
	c.Assert(err, check.IsNil)
	l, err := s.conn.Services().Find(bson.M{"_id": s.service.Name}).Count()
	c.Assert(err, check.IsNil)
	c.Assert(l, check.Equals, 0)
}

func (s *S) TestGetClient(c *check.C) {
	endpoints := map[string]string{
		"production": "http://mysql.api.com",
	}
	service := Service{Name: "redis", Password: "abcde", Endpoint: endpoints}
	cli, err := service.getClient("production")
	expected := &endpointClient{
		serviceName: "redis",
		endpoint:    endpoints["production"],
		username:    "redis",
		password:    "abcde",
	}
	c.Assert(err, check.IsNil)
	c.Assert(cli, check.DeepEquals, expected)
}

func (s *S) TestGetClientWithServiceUsername(c *check.C) {
	endpoints := map[string]string{
		"production": "http://mysql.api.com",
	}
	service := Service{Name: "redis", Username: "redis_test", Password: "abcde", Endpoint: endpoints}
	cli, err := service.getClient("production")
	expected := &endpointClient{
		serviceName: "redis",
		endpoint:    endpoints["production"],
		username:    "redis_test",
		password:    "abcde",
	}
	c.Assert(err, check.IsNil)
	c.Assert(cli, check.DeepEquals, expected)
}

func (s *S) TestGetClientWithUnknownEndpoint(c *check.C) {
	endpoints := map[string]string{
		"production": "http://mysql.api.com",
	}
	service := Service{Name: "redis", Endpoint: endpoints, Password: "abcde"}
	cli, err := service.getClient("staging")
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, "^Unknown endpoint: staging$")
	c.Assert(cli, check.IsNil)
}

func (s *S) TestGetUsername(c *check.C) {
	service := Service{Name: "test"}
	c.Assert(service.Name, check.Equals, service.getUsername())
	service.Username = "test_test"
	c.Assert(service.Username, check.Equals, service.getUsername())
}

func (s *S) TestGrantAccessShouldAddTeamToTheService(c *check.C) {
	s.createService(c)
	err := s.service.GrantAccess(s.team)
	c.Assert(err, check.IsNil)
	c.Assert(*s.team, HasAccessTo, *s.service)
}

func (s *S) TestGrantAccessShouldReturnErrorIfTheTeamAlreadyHasAcessToTheService(c *check.C) {
	s.createService(c)
	err := s.service.GrantAccess(s.team)
	c.Assert(err, check.IsNil)
	err = s.service.GrantAccess(s.team)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, "^This team already has access to this service$")
}

func (s *S) TestRevokeAccessShouldRemoveTeamFromService(c *check.C) {
	s.createService(c)
	err := s.service.GrantAccess(s.team)
	c.Assert(err, check.IsNil)
	err = s.service.RevokeAccess(s.team)
	c.Assert(err, check.IsNil)
	c.Assert(*s.team, check.Not(HasAccessTo), *s.service)
}

func (s *S) TestRevokeAcessShouldReturnErrorIfTheTeamDoesNotHaveAccessToTheService(c *check.C) {
	s.createService(c)
	err := s.service.RevokeAccess(s.team)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, "^This team does not have access to this service$")
}

func (s *S) TestGetServicesNames(c *check.C) {
	s1 := Service{Name: "Foo"}
	s2 := Service{Name: "Bar"}
	s3 := Service{Name: "FooBar"}
	sNames := getServicesNames([]Service{s1, s2, s3})
	c.Assert(sNames, check.DeepEquals, []string{"Foo", "Bar", "FooBar"})
}

func (s *S) TestUpdateService(c *check.C) {
	service := Service{
		Name:       "something",
		Password:   "abcde",
		Endpoint:   map[string]string{"production": "url"},
		OwnerTeams: []string{s.team.Name},
	}
	err := Create(service)
	c.Assert(err, check.IsNil)
	service.Doc = "doc"
	err = Update(service)
	c.Assert(err, check.IsNil)
	err = s.conn.Services().Find(bson.M{"_id": service.Name}).One(&service)
	c.Assert(err, check.IsNil)
	c.Assert(service.Doc, check.Equals, "doc")
}

func (s *S) TestUpdateServiceReturnErrorIfServiceDoesNotExist(c *check.C) {
	service := Service{Name: "something", Password: "abcde", Endpoint: map[string]string{"production": "url"}}
	err := Update(service)
	c.Assert(err, check.NotNil)
}

func (s *S) TestGetServicesNoCache(c *check.C) {
	var calls int32
	s.mockService.ServiceBrokerCatalogCache.OnLoad = func(brokerName string) (*serviceTypes.BrokerCatalog, error) {
		atomic.AddInt32(&calls, 1)
		c.Assert(brokerName, check.Equals, "aws")
		return nil, fmt.Errorf("not found")
	}
	s.mockService.ServiceBrokerCatalogCache.OnSave = func(brokerName string, catalog serviceTypes.BrokerCatalog) error {
		atomic.AddInt32(&calls, 1)
		c.Assert(brokerName, check.Equals, "aws")
		c.Assert(catalog.Services, check.HasLen, 2)
		return nil
	}
	s.createService(c)
	sb, err := BrokerService()
	c.Assert(err, check.IsNil)
	err = sb.Create(serviceTypes.Broker{Name: "aws"})
	c.Assert(err, check.IsNil)
	config := osbfake.FakeClientConfiguration{
		CatalogReaction: &osbfake.CatalogReaction{Response: &osb.CatalogResponse{
			Services: []osb.Service{
				{Name: "otherservice"},
				{Name: "service", Description: "This service is awesome!"},
			},
		}},
	}
	ClientFactory = osbfake.NewFakeClientFunc(config)
	services, err := GetServices()
	c.Assert(err, check.IsNil)
	c.Assert(services, check.DeepEquals, []Service{
		{
			Name:     "my-service",
			Password: "my-password",
			Endpoint: map[string]string{
				"production": "http://localhost:8080",
			},
			OwnerTeams: []string{"admin"},
			Teams:      []string{},
		},
		{
			Name:       "aws::otherservice",
			Teams:      []string(nil),
			OwnerTeams: []string(nil),
		},
		{
			Name:       "aws::service",
			Doc:        "This service is awesome!",
			Teams:      []string(nil),
			OwnerTeams: []string(nil),
		},
	})
	c.Assert(atomic.LoadInt32(&calls), check.Equals, int32(2))
}

func (s *S) TestGetServicesFromCache(c *check.C) {
	var calls int32
	s.mockService.ServiceBrokerCatalogCache.OnLoad = func(brokerName string) (*serviceTypes.BrokerCatalog, error) {
		atomic.AddInt32(&calls, 1)
		c.Assert(brokerName, check.Equals, "aws")
		return &serviceTypes.BrokerCatalog{
			Services: []serviceTypes.BrokerService{
				{Name: "otherservice"},
				{Name: "service", Description: "service loaded from cache"},
			},
		}, nil
	}
	s.createService(c)
	sb, err := BrokerService()
	c.Assert(err, check.IsNil)
	err = sb.Create(serviceTypes.Broker{Name: "aws"})
	c.Assert(err, check.IsNil)
	services, err := GetServices()
	c.Assert(err, check.IsNil)
	c.Assert(services, check.DeepEquals, []Service{
		{
			Name:     "my-service",
			Password: "my-password",
			Endpoint: map[string]string{
				"production": "http://localhost:8080",
			},
			OwnerTeams: []string{"admin"},
			Teams:      []string{},
		},
		{
			Name:       "aws::otherservice",
			Teams:      []string(nil),
			OwnerTeams: []string(nil),
		},
		{
			Name:       "aws::service",
			Doc:        "service loaded from cache",
			Teams:      []string(nil),
			OwnerTeams: []string(nil),
		},
	})
	c.Assert(atomic.LoadInt32(&calls), check.Equals, int32(1))
}

func (s *S) TestGetServicesByOwnerTeamsAndServices(c *check.C) {
	srvc := Service{
		Name:       "mongodb",
		OwnerTeams: []string{s.team.Name},
		Endpoint:   map[string]string{"production": "url"},
		Teams:      []string{},
		Password:   "abcde",
	}
	err := Create(srvc)
	c.Assert(err, check.IsNil)
	otherTeam := authTypes.Team{Name: "other-team"}
	srvc2 := Service{
		Name:       "mysql",
		OwnerTeams: []string{otherTeam.Name},
		Endpoint:   map[string]string{"production": "url"},
		Teams:      []string{s.team.Name},
		Password:   "abcde",
	}
	err = Create(srvc2)
	c.Assert(err, check.IsNil)
	services, err := GetServicesByOwnerTeamsAndServices([]string{s.team.Name}, nil)
	c.Assert(err, check.IsNil)
	expected := []Service{srvc}
	c.Assert(services, check.DeepEquals, expected)
}

func (s *S) TestGetServicesByOwnerTeamsAndServicesWithServices(c *check.C) {
	srvc := Service{
		Name:       "mongodb",
		OwnerTeams: []string{s.team.Name},
		Endpoint:   map[string]string{"production": "url"},
		Teams:      []string{},
		Password:   "abcde",
	}
	err := Create(srvc)
	c.Assert(err, check.IsNil)
	srvc2 := Service{
		Name:       "mysql",
		Teams:      []string{s.team.Name},
		OwnerTeams: []string{s.team.Name},
		Password:   "abcde",
		Endpoint:   map[string]string{"production": "url"},
	}
	err = Create(srvc2)
	c.Assert(err, check.IsNil)
	services, err := GetServicesByOwnerTeamsAndServices([]string{s.team.Name}, []string{srvc2.Name})
	c.Assert(err, check.IsNil)
	c.Assert(services, check.HasLen, 2)
	var names []string
	for _, s := range services {
		names = append(names, s.Name)
	}
	sort.Strings(names)
	c.Assert(names, check.DeepEquals, []string{"mongodb", "mysql"})
}

func (s *S) TestGetServicesByOwnerTeamsAndServicesShouldNotReturnsDeletedServices(c *check.C) {
	service := Service{
		Name:       "mysql",
		OwnerTeams: []string{s.team.Name},
		Endpoint:   map[string]string{"production": "url"},
		Teams:      []string{},
		Password:   "abcde",
	}
	err := Create(service)
	c.Assert(err, check.IsNil)
	deletedService := Service{
		Name:       "mongodb",
		OwnerTeams: []string{s.team.Name},
		Password:   "abcde",
		Endpoint:   map[string]string{"production": "url"},
	}
	err = Create(deletedService)
	c.Assert(err, check.IsNil)
	err = Delete(deletedService)
	c.Assert(err, check.IsNil)
	services, err := GetServicesByOwnerTeamsAndServices([]string{s.team.Name}, nil)
	c.Assert(err, check.IsNil)
	c.Assert(err, check.IsNil)
	expected := []Service{service}
	c.Assert(services, check.DeepEquals, expected)
}

func (s *S) TestServiceModelMarshalJSON(c *check.C) {
	sm := []ServiceModel{
		{Service: "mysql"},
		{Service: "mongo", ServiceInstances: []ServiceInstance{
			{
				Name:        "my instance",
				Tags:        []string{"my tag"},
				TeamOwner:   "t1",
				ServiceName: "mysql",
				Teams:       []string{"t1", "t2"},
				Apps:        []string{"app1", "app2"},
				BoundUnits: []Unit{
					{AppName: "app1", ID: "unitid1", IP: "unitip1"},
				},
				Parameters: map[string]interface{}{"parameter": "val"},
			},
		}},
	}
	data, err := json.Marshal(&sm)
	c.Assert(err, check.IsNil)
	expected := make([]map[string]interface{}, 2)
	expected[0] = map[string]interface{}{
		"service":           "mysql",
		"instances":         nil,
		"plans":             nil,
		"service_instances": nil,
	}
	expected[1] = map[string]interface{}{
		"service":   "mongo",
		"instances": nil,
		"plans":     nil,
		"service_instances": []interface{}{map[string]interface{}{
			"name":         "my instance",
			"tags":         []interface{}{"my tag"},
			"team_owner":   "t1",
			"id":           float64(0),
			"teams":        []interface{}{"t1", "t2"},
			"apps":         []interface{}{"app1", "app2"},
			"plan_name":    "",
			"service_name": "mysql",
			"description":  "",
			"bound_units": []interface{}{map[string]interface{}{
				"id":       "unitid1",
				"ip":       "unitip1",
				"app_name": "app1",
			}},
			"parameters": map[string]interface{}{
				"parameter": "val",
			},
		}},
	}
	result := make([]map[string]interface{}, 2)
	err = json.Unmarshal(data, &result)
	c.Assert(err, check.IsNil)
	c.Assert(result, check.DeepEquals, expected)
}

func (s *S) TestProxy(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()
	service := Service{
		Name:     "mongodb",
		Endpoint: map[string]string{"production": ts.URL},
		Password: "abcde",
	}
	err := s.conn.Services().Insert(service)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("DELETE", "/something", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	evt := createEvt(c)
	err = Proxy(&service, "/aaa", evt, "", recorder, request)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Code, check.Equals, http.StatusNoContent)
}

func (s *S) TestRenameServiceTeam(c *check.C) {
	services := []Service{
		{Name: "s1", Teams: []string{"team1", "team2", "team3"}, OwnerTeams: []string{"team1", "teamx"}},
		{Name: "s2", Teams: []string{"team1", "team3"}, OwnerTeams: []string{"teamx", "team2"}},
		{Name: "s3", Teams: []string{"team2", "team3"}, OwnerTeams: []string{"team3"}},
	}
	for _, si := range services {
		err := s.conn.Services().Insert(&si)
		c.Assert(err, check.IsNil)
	}
	err := RenameServiceTeam("team2", "team9000")
	c.Assert(err, check.IsNil)
	var dbServices []Service
	err = s.conn.Services().Find(nil).Sort("_id").All(&dbServices)
	c.Assert(err, check.IsNil)
	c.Assert(dbServices, check.DeepEquals, []Service{
		{Name: "s1", Teams: []string{"team1", "team3", "team9000"}, OwnerTeams: []string{"team1", "teamx"}, Endpoint: map[string]string{}},
		{Name: "s2", Teams: []string{"team1", "team3"}, OwnerTeams: []string{"teamx", "team9000"}, Endpoint: map[string]string{}},
		{Name: "s3", Teams: []string{"team3", "team9000"}, OwnerTeams: []string{"team3"}, Endpoint: map[string]string{}},
	})
}
