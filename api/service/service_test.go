// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import (
	"github.com/globocom/tsuru/db"
	"labix.org/v2/mgo/bson"
	. "launchpad.net/gocheck"
)

func (s *S) createService() {
	s.service = &Service{Name: "my_service"}
	s.service.Create()
}

func (s *S) TestGetService(c *C) {
	s.createService()
	anotherService := Service{Name: s.service.Name}
	anotherService.Get()
	c.Assert(anotherService.Name, Equals, s.service.Name)
}

func (s *S) TestGetServiceReturnsErrorIfTheServiceIsDeleted(c *C) {
	se := Service{Name: "anything", Status: "deleted"}
	err := db.Session.Services().Insert(se)
	c.Assert(err, IsNil)
	defer db.Session.Services().Remove(bson.M{"_id": se.Name})
	err = se.Get()
	c.Assert(err, NotNil)
}

func (s *S) TestCreateService(c *C) {
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
	c.Assert(err, IsNil)
	se := Service{Name: service.Name}
	err = se.Get()
	c.Assert(err, IsNil)
	c.Assert(se.Name, Equals, service.Name)
	c.Assert(se.Endpoint["production"], Equals, endpt["production"])
	c.Assert(se.Endpoint["test"], Equals, endpt["test"])
	c.Assert(se.OwnerTeams, DeepEquals, []string{s.team.Name})
	c.Assert(se.Status, Equals, "created")
	c.Assert(se.IsRestricted, Equals, false)
}

func (s *S) TestDeleteService(c *C) {
	s.createService()
	err := s.service.Delete()
	defer db.Session.Services().Remove(bson.M{"_id": s.service.Name})
	c.Assert(err, IsNil)
	var se Service
	err = db.Session.Services().Find(bson.M{"_id": s.service.Name}).One(&se)
	c.Assert(se.Status, Equals, "deleted")
}

func (s *S) TestProductionEndpoint(c *C) {
	endpoints := map[string]string{
		"production": "http://mysql.api.com",
		"test":       "http://localhost:9090",
	}
	service := Service{Name: "redis", Endpoint: endpoints}
	c.Assert(service.ProductionEndpoint(), DeepEquals, &Client{endpoint: endpoints["production"]})
}

func (s *S) TestGetClient(c *C) {
	endpoints := map[string]string{
		"production": "http://mysql.api.com",
		"test":       "http://localhost:9090",
	}
	service := Service{Name: "redis", Endpoint: endpoints}
	cli, err := service.getClient("production")
	c.Assert(err, IsNil)
	c.Assert(cli, DeepEquals, &Client{endpoint: endpoints["production"]})
}

func (s *S) TestGetClientWithouHttp(c *C) {
	endpoints := map[string]string{
		"production": "mysql.api.com",
		"test":       "localhost:9090",
	}
	service := Service{Name: "redis", Endpoint: endpoints}
	cli, err := service.getClient("production")
	c.Assert(err, IsNil)
	c.Assert(cli.endpoint, Equals, "http://mysql.api.com")
}

func (s *S) TestGetClientWithUnknownEndpoint(c *C) {
	endpoints := map[string]string{
		"production": "http://mysql.api.com",
		"test":       "http://localhost:9090",
	}
	service := Service{Name: "redis", Endpoint: endpoints}
	cli, err := service.getClient("staging")
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^Unknown endpoint: staging$")
	c.Assert(cli, IsNil)
}

func (s *S) TestGrantAccessShouldAddTeamToTheService(c *C) {
	s.createService()
	err := s.service.GrantAccess(s.team)
	c.Assert(err, IsNil)
	c.Assert(*s.team, HasAccessTo, *s.service)
}

func (s *S) TestGrantAccessShouldReturnErrorIfTheTeamAlreadyHasAcessToTheService(c *C) {
	s.createService()
	err := s.service.GrantAccess(s.team)
	c.Assert(err, IsNil)
	err = s.service.GrantAccess(s.team)
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^This team already has access to this service$")
}

func (s *S) TestRevokeAccessShouldRemoveTeamFromService(c *C) {
	s.createService()
	err := s.service.GrantAccess(s.team)
	c.Assert(err, IsNil)
	err = s.service.RevokeAccess(s.team)
	c.Assert(err, IsNil)
	c.Assert(*s.team, Not(HasAccessTo), *s.service)
}

func (s *S) TestRevokeAcessShouldReturnErrorIfTheTeamDoesNotHaveAccessToTheService(c *C) {
	s.createService()
	err := s.service.RevokeAccess(s.team)
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^This team does not have access to this service$")
}

func (s *S) TestGetServicesNames(c *C) {
	s1 := Service{Name: "Foo"}
	s2 := Service{Name: "Bar"}
	s3 := Service{Name: "FooBar"}
	sNames := GetServicesNames([]Service{s1, s2, s3})
	c.Assert(sNames, DeepEquals, []string{"Foo", "Bar", "FooBar"})
}

func (s *S) TestUpdateService(c *C) {
	service := Service{Name: "something", Status: "created"}
	err := service.Create()
	c.Assert(err, IsNil)
	defer db.Session.Services().Remove(bson.M{"_id": service.Name})
	service.Status = "destroyed"
	err = service.Update()
	c.Assert(err, IsNil)
	err = db.Session.Services().Find(bson.M{"_id": service.Name}).One(&service)
	c.Assert(service.Status, Equals, "destroyed")
}

func (s *S) TestUpdateServiceReturnErrorIfServiceDoesNotExist(c *C) {
	service := Service{Name: "something"}
	err := service.Update()
	c.Assert(err, NotNil)
}

func (s *S) TestServiceByTeamKindFilteringByOwnerTeamsAndRetrievingNotRestrictedServices(c *C) {
	srvc := Service{Name: "mysql", OwnerTeams: []string{s.team.Name}, Status: "created"}
	err := srvc.Create()
	c.Assert(err, IsNil)
	srvc2 := Service{Name: "mongodb", IsRestricted: false}
	err = srvc2.Create()
	c.Assert(err, IsNil)
	rSrvc, err := GetServicesByTeamKindAndNoRestriction("owner_teams", s.user)
	c.Assert(err, IsNil)
	expected := []Service{Service{Name: srvc.Name}, Service{Name: srvc2.Name}}
	c.Assert(expected, DeepEquals, rSrvc)
}

func (s *S) TestServiceByTeamKindFilteringByTeamsAndNotRetrieveRestrictedServices(c *C) {
	srvc := Service{Name: "mysql", Teams: []string{s.team.Name}, Status: "created"}
	err := srvc.Create()
	c.Assert(err, IsNil)
	srvc2 := Service{Name: "mongodb", IsRestricted: true}
	err = srvc2.Create()
	c.Assert(err, IsNil)
	rSrvc, err := GetServicesByTeamKindAndNoRestriction("teams", s.user)
	c.Assert(err, IsNil)
	expected := []Service{Service{Name: srvc.Name}}
	c.Assert(expected, DeepEquals, rSrvc)
}

func (s *S) TestServiceByTeamKindShouldNotReturnsDeletedServices(c *C) {
	service := Service{Name: "mysql", Teams: []string{s.team.Name}}
	err := service.Create()
	c.Assert(err, IsNil)
	deleted_service := Service{Name: "firebird", Teams: []string{s.team.Name}}
	err = deleted_service.Create()
	c.Assert(err, IsNil)
	err = deleted_service.Delete()
	c.Assert(err, IsNil)
	result, err := GetServicesByTeamKindAndNoRestriction("teams", s.user)
	c.Assert(err, IsNil)
	expected := []Service{Service{Name: service.Name}}
	c.Assert(expected, DeepEquals, result)
}

func (s *S) TestGetServicesByOwnerTeams(c *C) {
	srvc := Service{Name: "mongodb", OwnerTeams: []string{s.team.Name}, Endpoint: map[string]string{}, Teams: []string{}}
	err := srvc.Create()
	c.Assert(err, IsNil)
	defer srvc.Delete()
	srvc2 := Service{Name: "mysql", Teams: []string{s.team.Name}}
	err = srvc2.Create()
	c.Assert(err, IsNil)
	defer srvc2.Delete()
	services, err := GetServicesByOwnerTeams("owner_teams", s.user)
	expected := []Service{srvc}
	c.Assert(services, DeepEquals, expected)
}

func (s *S) TestGetServicesByOwnerTeamsShouldNotReturnsDeletedServices(c *C) {
	service := Service{Name: "mysql", OwnerTeams: []string{s.team.Name}, Endpoint: map[string]string{}, Teams: []string{}}
	err := service.Create()
	c.Assert(err, IsNil)
	deleted_service := Service{Name: "mongodb", OwnerTeams: []string{s.team.Name}}
	err = deleted_service.Create()
	c.Assert(err, IsNil)
	err = deleted_service.Delete()
	services, err := GetServicesByOwnerTeams("owner_teams", s.user)
	expected := []Service{service}
	c.Assert(services, DeepEquals, expected)
}
