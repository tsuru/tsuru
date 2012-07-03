package service

import (
	"github.com/timeredbull/tsuru/db"
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

func (s *S) TestAllServices(c *C) {
	se := Service{Name: "myService"}
	se2 := Service{Name: "myOtherService"}
	err := se.Create()
	c.Assert(err, IsNil)
	err = se2.Create()
	c.Assert(err, IsNil)
	defer se.Delete()
	defer se2.Delete()

	s_ := Service{}
	results := s_.All()
	c.Assert(len(results), Equals, 2)
}

func (s *S) TestCreateService(c *C) {
	endpt := map[string]string{
		"production": "somehost.com",
		"test":       "test.somehost.com",
	}
	service := &Service{Name: "my_service", Endpoint: endpt}
	service.Create()
	se := Service{Name: service.Name}
	se.Get()
	c.Assert(se.Name, Equals, s.service.Name)
	c.Assert(endpt["production"], Equals, se.Endpoint["production"])
	c.Assert(endpt["test"], Equals, se.Endpoint["test"])
}

func (s *S) TestDeleteService(c *C) {
	s.createService()
	s.service.Delete()
	qtd, err := db.Session.Services().Find(nil).Count()
	c.Assert(err, IsNil)
	c.Assert(qtd, Equals, 0)
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

// func (s *S) TestBindService(c *C) {
// 	s.createService()
// 	app := &app.App{Name: "my_app", Framework: "django"}
// 	app.Create()
// 	s.service.Bind(app)
// 	var result ServiceInstance
// 	query := bson.M{
// 		"_id": s.service.Name,
// 		"apps":     []app.App{app.Name},
// 	}
// 	err := db.Session.ServiceInstances().Find(query).One(&result)
// 	c.Assert(err, IsNil)
// 	c.Assert(s.service.Name, Equals, result.Name)
// 	c.Assert(app.Name, Equals, result.Apps[0].Name)
// }
// 
// func (s *S) TestUnbindService(c *C) {
// 	s.createService()
// 	app := &app.App{Name: "my_app", Framework: "django"}
// 	app.Create()
// 	s.service.Bind(app)
// 	s.service.Unbind(app)
// 	query := bson.M{
// 		"_id": s.service.Name,
// 		"apps":     []app.App{app.Name},
// 	}
// 	qtd, err := db.Session.ServiceInstances().Find(query).Count()
// 	c.Assert(err, IsNil)
// 	c.Assert(qtd, Equals, 0)
// }

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

func (s *S) TestCheckUserPermissionShouldReturnTrueIfTheGivenUserIsMemberOfOneOfTheServicesTeam(c *C) {
	s.createService()
	s.service.GrantAccess(s.team)
	c.Assert(s.service.CheckUserAccess(s.user), Equals, true)
}

func (s *S) TestCheckUserPermissionShouldReturnFalseIfTheGivenUserIsNotMemberOfAnyOfTheServicesTeam(c *C) {
	s.createService()
	c.Assert(s.service.CheckUserAccess(s.user), Equals, false)
}
