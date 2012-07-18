package service

import (
	"github.com/timeredbull/tsuru/db"
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

func (s *S) TestAllServices(c *C) {
	se := Service{Name: "myService"}
	se2 := Service{Name: "myOtherService"}
	err := se.Create()
	c.Assert(err, IsNil)
	err = se2.Create()
	c.Assert(err, IsNil)
	defer db.Session.Services().Remove(bson.M{"_id": bson.M{"$in": []string{se.Name, se2.Name}}})

	s_ := Service{}
	results := s_.All()
	c.Assert(len(results), Equals, 2)
}

func (s *S) TestCreateService(c *C) {
	endpt := map[string]string{
		"production": "somehost.com",
		"test":       "test.somehost.com",
	}
	bootstrap := map[string]string{
		"ami":  "ami-0000007",
		"when": "on-new-instance",
	}
	service := &Service{
		Name:       "my_service",
		Endpoint:   endpt,
		Bootstrap:  bootstrap,
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

func (s *S) TestGetClient(c *C) {
	endpoints := map[string]string{
		"production": "http://mysql.api.com",
		"test":       "http://localhost:9090",
	}
	service := Service{Name: "redis", Endpoint: endpoints}
	cli, err := service.GetClient("production")
	c.Assert(err, IsNil)
	c.Assert(cli, DeepEquals, &Client{endpoint: endpoints["production"]})
}

func (s *S) TestGetClientWithouHttp(c *C) {
	endpoints := map[string]string{
		"production": "mysql.api.com",
		"test":       "localhost:9090",
	}
	service := Service{Name: "redis", Endpoint: endpoints}
	cli, err := service.GetClient("production")
	c.Assert(err, IsNil)
	c.Assert(cli.endpoint, Equals, "http://mysql.api.com")
}

func (s *S) TestGetClientWithUnknownEndpoint(c *C) {
	endpoints := map[string]string{
		"production": "http://mysql.api.com",
		"test":       "http://localhost:9090",
	}
	service := Service{Name: "redis", Endpoint: endpoints}
	cli, err := service.GetClient("staging")
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
