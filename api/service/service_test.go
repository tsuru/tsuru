package service

import (
	"github.com/timeredbull/tsuru/db"
	. "launchpad.net/gocheck"
)

func (s *ServiceSuite) createService() {
	s.service = &Service{Name: "my_service"}
	s.service.Create()
}

func (s *ServiceSuite) TestGetService(c *C) {
	s.createService()
	anotherService := Service{Name: s.service.Name}
	anotherService.Get()
	c.Assert(anotherService.Name, Equals, s.service.Name)
}

func (s *ServiceSuite) TestAllServices(c *C) {
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

func (s *ServiceSuite) TestCreateService(c *C) {
	s.createService()
	se := Service{Name: s.service.Name}
	se.Get()
	c.Assert(se.Name, Equals, s.service.Name)
}

func (s *ServiceSuite) TestDeleteService(c *C) {
	s.createService()
	s.service.Delete()
	qtd, err := db.Session.Services().Find(nil).Count()
	c.Assert(err, IsNil)
	c.Assert(qtd, Equals, 0)
}

// func (s *ServiceSuite) TestBindService(c *C) {
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
// func (s *ServiceSuite) TestUnbindService(c *C) {
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

func (s *ServiceSuite) TestGrantAccessShouldAddTeamToTheService(c *C) {
	s.createService()
	err := s.service.GrantAccess(s.team)
	c.Assert(err, IsNil)
	c.Assert(*s.team, HasAccessTo, *s.service)
}

func (s *ServiceSuite) TestGrantAccessShouldReturnErrorIfTheTeamAlreadyHasAcessToTheService(c *C) {
	s.createService()
	err := s.service.GrantAccess(s.team)
	c.Assert(err, IsNil)
	err = s.service.GrantAccess(s.team)
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^This team already has access to this service$")
}

func (s *ServiceSuite) TestRevokeAccessShouldRemoveTeamFromService(c *C) {
	s.createService()
	err := s.service.GrantAccess(s.team)
	c.Assert(err, IsNil)
	err = s.service.RevokeAccess(s.team)
	c.Assert(err, IsNil)
	c.Assert(*s.team, Not(HasAccessTo), *s.service)
}

func (s *ServiceSuite) TestRevokeAcessShouldReturnErrorIfTheTeamDoesNotHaveAccessToTheService(c *C) {
	s.createService()
	err := s.service.RevokeAccess(s.team)
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^This team does not have access to this service$")
}

func (s *ServiceSuite) TestCheckUserPermissionShouldReturnTrueIfTheGivenUserIsMemberOfOneOfTheServicesTeam(c *C) {
	s.createService()
	s.service.GrantAccess(s.team)
	c.Assert(s.service.CheckUserAccess(s.user), Equals, true)
}

func (s *ServiceSuite) TestCheckUserPermissionShouldReturnFalseIfTheGivenUserIsNotMemberOfAnyOfTheServicesTeam(c *C) {
	s.createService()
	c.Assert(s.service.CheckUserAccess(s.user), Equals, false)
}
