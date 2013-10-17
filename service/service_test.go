// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import (
	"encoding/json"
	"labix.org/v2/mgo/bson"
	"launchpad.net/gocheck"
)

func (s *S) createService() {
	s.service = &Service{Name: "my_service"}
	s.service.Create()
}

func (s *S) TestGetService(c *gocheck.C) {
	s.createService()
	anotherService := Service{Name: s.service.Name}
	anotherService.Get()
	c.Assert(anotherService.Name, gocheck.Equals, s.service.Name)
}

func (s *S) TestGetServiceReturnsErrorIfTheServiceIsDeleted(c *gocheck.C) {
	se := Service{Name: "anything", Status: "deleted"}
	err := s.conn.Services().Insert(se)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": se.Name})
	err = se.Get()
	c.Assert(err, gocheck.NotNil)
}

func (s *S) TestCreateService(c *gocheck.C) {
	endpt := map[string]string{
		"production": "somehost.com",
		"test":       "test.somehost.com",
	}
	service := &Service{
		Name:       "my_service",
		Endpoint:   endpt,
		OwnerTeams: []string{s.team.Name},
	}
	err := service.Create()
	c.Assert(err, gocheck.IsNil)
	se := Service{Name: service.Name}
	err = se.Get()
	c.Assert(err, gocheck.IsNil)
	c.Assert(se.Name, gocheck.Equals, service.Name)
	c.Assert(se.Endpoint["production"], gocheck.Equals, endpt["production"])
	c.Assert(se.Endpoint["test"], gocheck.Equals, endpt["test"])
	c.Assert(se.OwnerTeams, gocheck.DeepEquals, []string{s.team.Name})
	c.Assert(se.Status, gocheck.Equals, "created")
	c.Assert(se.IsRestricted, gocheck.Equals, false)
}

func (s *S) TestDeleteService(c *gocheck.C) {
	s.createService()
	err := s.service.Delete()
	defer s.conn.Services().Remove(bson.M{"_id": s.service.Name})
	c.Assert(err, gocheck.IsNil)
	l, err := s.conn.Services().Find(bson.M{"_id": s.service.Name}).Count()
	c.Assert(err, gocheck.IsNil)
	c.Assert(l, gocheck.Equals, 0)
}

func (s *S) TestGetClient(c *gocheck.C) {
	endpoints := map[string]string{
		"production": "http://mysql.api.com",
		"test":       "http://localhost:9090",
	}
	service := Service{Name: "redis", Endpoint: endpoints}
	cli, err := service.getClient("production")
	c.Assert(err, gocheck.IsNil)
	c.Assert(cli, gocheck.DeepEquals, &Client{endpoint: endpoints["production"]})
}

func (s *S) TestGetClientWithouHTTP(c *gocheck.C) {
	endpoints := map[string]string{
		"production": "mysql.api.com",
		"test":       "localhost:9090",
	}
	service := Service{Name: "redis", Endpoint: endpoints}
	cli, err := service.getClient("production")
	c.Assert(err, gocheck.IsNil)
	c.Assert(cli.endpoint, gocheck.Equals, "http://mysql.api.com")
}

func (s *S) TestGetClientWithUnknownEndpoint(c *gocheck.C) {
	endpoints := map[string]string{
		"production": "http://mysql.api.com",
		"test":       "http://localhost:9090",
	}
	service := Service{Name: "redis", Endpoint: endpoints}
	cli, err := service.getClient("staging")
	c.Assert(err, gocheck.NotNil)
	c.Assert(err, gocheck.ErrorMatches, "^Unknown endpoint: staging$")
	c.Assert(cli, gocheck.IsNil)
}

func (s *S) TestGrantAccessShouldAddTeamToTheService(c *gocheck.C) {
	s.createService()
	err := s.service.GrantAccess(s.team)
	c.Assert(err, gocheck.IsNil)
	c.Assert(*s.team, HasAccessTo, *s.service)
}

func (s *S) TestGrantAccessShouldReturnErrorIfTheTeamAlreadyHasAcessToTheService(c *gocheck.C) {
	s.createService()
	err := s.service.GrantAccess(s.team)
	c.Assert(err, gocheck.IsNil)
	err = s.service.GrantAccess(s.team)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err, gocheck.ErrorMatches, "^This team already has access to this service$")
}

func (s *S) TestRevokeAccessShouldRemoveTeamFromService(c *gocheck.C) {
	s.createService()
	err := s.service.GrantAccess(s.team)
	c.Assert(err, gocheck.IsNil)
	err = s.service.RevokeAccess(s.team)
	c.Assert(err, gocheck.IsNil)
	c.Assert(*s.team, gocheck.Not(HasAccessTo), *s.service)
}

func (s *S) TestRevokeAcessShouldReturnErrorIfTheTeamDoesNotHaveAccessToTheService(c *gocheck.C) {
	s.createService()
	err := s.service.RevokeAccess(s.team)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err, gocheck.ErrorMatches, "^This team does not have access to this service$")
}

func (s *S) TestGetServicesNames(c *gocheck.C) {
	s1 := Service{Name: "Foo"}
	s2 := Service{Name: "Bar"}
	s3 := Service{Name: "FooBar"}
	sNames := GetServicesNames([]Service{s1, s2, s3})
	c.Assert(sNames, gocheck.DeepEquals, []string{"Foo", "Bar", "FooBar"})
}

func (s *S) TestUpdateService(c *gocheck.C) {
	service := Service{Name: "something", Status: "created"}
	err := service.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": service.Name})
	service.Status = "destroyed"
	err = service.Update()
	c.Assert(err, gocheck.IsNil)
	err = s.conn.Services().Find(bson.M{"_id": service.Name}).One(&service)
	c.Assert(service.Status, gocheck.Equals, "destroyed")
}

func (s *S) TestUpdateServiceReturnErrorIfServiceDoesNotExist(c *gocheck.C) {
	service := Service{Name: "something"}
	err := service.Update()
	c.Assert(err, gocheck.NotNil)
}

func (s *S) TestServiceByTeamKindFilteringByOwnerTeamsAndRetrievingNotRestrictedServices(c *gocheck.C) {
	srvc := Service{Name: "mysql", OwnerTeams: []string{s.team.Name}, Status: "created"}
	err := srvc.Create()
	c.Assert(err, gocheck.IsNil)
	srvc2 := Service{Name: "mongodb", IsRestricted: false}
	err = srvc2.Create()
	c.Assert(err, gocheck.IsNil)
	rSrvc, err := GetServicesByTeamKindAndNoRestriction("owner_teams", s.user)
	c.Assert(err, gocheck.IsNil)
	expected := []Service{{Name: srvc.Name}, {Name: srvc2.Name}}
	c.Assert(expected, gocheck.DeepEquals, rSrvc)
}

func (s *S) TestServiceByTeamKindFilteringByTeamsAndNotRetrieveRestrictedServices(c *gocheck.C) {
	srvc := Service{Name: "mysql", Teams: []string{s.team.Name}, Status: "created"}
	err := srvc.Create()
	c.Assert(err, gocheck.IsNil)
	srvc2 := Service{Name: "mongodb", IsRestricted: true}
	err = srvc2.Create()
	c.Assert(err, gocheck.IsNil)
	rSrvc, err := GetServicesByTeamKindAndNoRestriction("teams", s.user)
	c.Assert(err, gocheck.IsNil)
	expected := []Service{{Name: srvc.Name}}
	c.Assert(expected, gocheck.DeepEquals, rSrvc)
}

func (s *S) TestServiceByTeamKindShouldNotReturnsDeletedServices(c *gocheck.C) {
	service := Service{Name: "mysql", Teams: []string{s.team.Name}}
	err := service.Create()
	c.Assert(err, gocheck.IsNil)
	deletedService := Service{Name: "firebird", Teams: []string{s.team.Name}}
	err = deletedService.Create()
	c.Assert(err, gocheck.IsNil)
	err = deletedService.Delete()
	c.Assert(err, gocheck.IsNil)
	result, err := GetServicesByTeamKindAndNoRestriction("teams", s.user)
	c.Assert(err, gocheck.IsNil)
	expected := []Service{{Name: service.Name}}
	c.Assert(expected, gocheck.DeepEquals, result)
}

func (s *S) TestGetServicesByOwnerTeams(c *gocheck.C) {
	srvc := Service{Name: "mongodb", OwnerTeams: []string{s.team.Name}, Endpoint: map[string]string{}, Teams: []string{}}
	err := srvc.Create()
	c.Assert(err, gocheck.IsNil)
	defer srvc.Delete()
	srvc2 := Service{Name: "mysql", Teams: []string{s.team.Name}}
	err = srvc2.Create()
	c.Assert(err, gocheck.IsNil)
	defer srvc2.Delete()
	services, err := GetServicesByOwnerTeams("owner_teams", s.user)
	expected := []Service{srvc}
	c.Assert(services, gocheck.DeepEquals, expected)
}

func (s *S) TestGetServicesByOwnerTeamsShouldNotReturnsDeletedServices(c *gocheck.C) {
	service := Service{Name: "mysql", OwnerTeams: []string{s.team.Name}, Endpoint: map[string]string{}, Teams: []string{}}
	err := service.Create()
	c.Assert(err, gocheck.IsNil)
	deletedService := Service{Name: "mongodb", OwnerTeams: []string{s.team.Name}}
	err = deletedService.Create()
	c.Assert(err, gocheck.IsNil)
	err = deletedService.Delete()
	services, err := GetServicesByOwnerTeams("owner_teams", s.user)
	expected := []Service{service}
	c.Assert(services, gocheck.DeepEquals, expected)
}

func (s *S) TestServiceModelMarshalJSON(c *gocheck.C) {
	sm := []ServiceModel{
		{Service: "mysql"},
		{Service: "mongo"},
	}
	data, err := json.Marshal(&sm)
	c.Assert(err, gocheck.IsNil)
	expected := make([]map[string]interface{}, 2)
	expected[0] = map[string]interface{}{
		"service":   "mysql",
		"instances": nil,
	}
	expected[1] = map[string]interface{}{
		"service":   "mongo",
		"instances": nil,
	}
	result := make([]map[string]interface{}, 2)
	err = json.Unmarshal(data, &result)
	c.Assert(err, gocheck.IsNil)
	c.Assert(result, gocheck.DeepEquals, expected)
}
