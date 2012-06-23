package service

import (
	"bytes"
	"github.com/timeredbull/commandmocker"
	"github.com/timeredbull/tsuru/api/app"
	"github.com/timeredbull/tsuru/db"
	"github.com/timeredbull/tsuru/log"
	. "launchpad.net/gocheck"
	"labix.org/v2/mgo/bson"
	stdlog "log"
	"strings"
)

func (s *ServiceSuite) createService() {
	s.serviceType = &ServiceType{Name: "Mysql", Charm: "mysql"}
	s.serviceType.Create()
	s.service = &Service{ServiceTypeId: s.serviceType.Id, Name: "my_service"}
	s.service.Create()
}

func (s *ServiceSuite) TestGetService(c *C) {
	s.createService()
	anotherService := Service{Name: s.service.Name}
	anotherService.Get()
	c.Assert(anotherService.Name, Equals, s.service.Name)
	c.Assert(anotherService.ServiceTypeId, Equals, s.service.ServiceTypeId)
}

func (s *ServiceSuite) TestAllServices(c *C) {
	st := ServiceType{Name: "mysql", Charm: "mysql"}
	st.Create()
	se := Service{ServiceTypeId: st.Id, Name: "myService"}
	se2 := Service{ServiceTypeId: st.Id, Name: "myOtherService"}
	se.Create()
	se2.Create()

	s_ := Service{}
	results := s_.All()
	c.Assert(len(results), Equals, 2)
}

func (s *ServiceSuite) TestCreateService(c *C) {
	w := bytes.NewBuffer([]byte{})
	l := stdlog.New(w, "", stdlog.LstdFlags)
	log.Target = l
	dir, err := commandmocker.Add("juju", "$*")
	c.Assert(err, IsNil)
	defer commandmocker.Remove(dir)
	s.createService()
	se := Service{Name: s.service.Name}
	se.Get()
	c.Assert(se.ServiceTypeId, Equals, s.serviceType.Id)
	c.Assert(se.Name, Equals, s.service.Name)
	strOut := strings.Replace(w.String(), "\n", "", -1)
	c.Assert(strOut, Matches, ".*deploy --repository=/home/charms mysql my_service")
}

func (s *ServiceSuite) TestDeleteService(c *C) {
	s.createService()
	s.service.Delete()
	qtd, err := db.Session.Services().Find(nil).Count()
	c.Assert(err, IsNil)
	c.Assert(qtd, Equals, 0)
}

func (s *ServiceSuite) TestRetrieveAssociateServiceType(c *C) {
	serviceType := ServiceType{Name: "Mysql", Charm: "mysql"}
	serviceType.Create()

	service := &Service{
		ServiceTypeId: serviceType.Id,
		Name:          "my_service",
	}
	service.Create()
	retrievedServiceType := service.ServiceType()

	c.Assert(serviceType.Id, Equals, retrievedServiceType.Id)
	c.Assert(serviceType.Name, Equals, retrievedServiceType.Name)
	c.Assert(serviceType.Charm, Equals, retrievedServiceType.Charm)
}

func (s *ServiceSuite) TestBindService(c *C) {
	s.createService()
	app := &app.App{Name: "my_app", Framework: "django"}
	app.Create()
	s.service.Bind(app)
	var result ServiceApp
	query := bson.M{
		"service_name": s.service.Name,
		"app_name":     app.Name,
	}
	err := db.Session.ServiceApps().Find(query).One(&result)
	c.Assert(err, IsNil)
	c.Assert(s.service.Name, Equals, result.ServiceName)
	c.Assert(app.Name, Equals, result.AppName)
}

func (s *ServiceSuite) TestUnbindService(c *C) {
	s.createService()
	app := &app.App{Name: "my_app", Framework: "django"}
	app.Create()
	s.service.Bind(app)
	s.service.Unbind(app)
	query := bson.M{
		"service_name": s.service.Name,
		"app_name":     app.Name,
	}
	qtd, err := db.Session.ServiceApps().Find(query).Count()
	c.Assert(err, IsNil)
	c.Assert(qtd, Equals, 0)
}

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
