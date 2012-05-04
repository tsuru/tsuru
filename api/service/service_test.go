package service

import (
	"github.com/timeredbull/tsuru/api/app"
	"github.com/timeredbull/tsuru/db"
	. "launchpad.net/gocheck"
	"launchpad.net/mgo/bson"
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
	s.createService()
	se := Service{Name: s.service.Name}
	se.Get()
	c.Assert(se.ServiceTypeId, Equals, s.serviceType.Id)
	c.Assert(se.Name, Equals, s.service.Name)
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
