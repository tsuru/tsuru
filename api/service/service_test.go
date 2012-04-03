package service_test

import (
	. "github.com/timeredbull/tsuru/api/app"
	. "github.com/timeredbull/tsuru/api/service"
	. "github.com/timeredbull/tsuru/database"
	. "launchpad.net/gocheck"
)

func (s *ServiceSuite) createService() {
	s.serviceType = &ServiceType{Name: "Mysql", Charm: "mysql"}
	s.serviceType.Create()

	s.service = &Service{ServiceTypeId: s.serviceType.Id, Name: "my_service"}
	s.service.Create()
}

func (s *ServiceSuite) TestGetService(c *C) {
	s.createService()

	anotherService := Service{Id: s.service.Id}
	anotherService.Get()

	c.Assert(anotherService.Id, Equals, s.service.Id)
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
	se := Service{Id: s.service.Id}
	se.Get()

	c.Assert(se.Id, Equals, s.service.Id)
	c.Assert(se.ServiceTypeId, Equals, s.serviceType.Id)
	c.Assert(se.Name, Equals, s.service.Name)
}

func (s *ServiceSuite) TestDeleteService(c *C) {
	s.createService()
	s.service.Delete()

	collection := Mdb.C("services")
	qtd, err := collection.Find(nil).Count()
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
	app := &App{Name: "my_app", Framework: "django"}
	app.Create()
	s.service.Bind(app)
	var result ServiceApp

	collection := Mdb.C("service_apps")
	query := map[string]interface{}{
		"service_id": s.service.Id,
		"app_id":     app.Id,
	}
	err := collection.Find(query).One(&result)
	if err != nil {
		panic(err)
	}

	c.Assert(s.service.Id, Equals, result.ServiceId)
	c.Assert(app.Id, Equals, result.AppId)
}

func (s *ServiceSuite) TestUnbindService(c *C) {
	s.createService()
	app := &App{Name: "my_app", Framework: "django"}
	app.Create()
	s.service.Bind(app)
	s.service.Unbind(app)

	query := make(map[string]interface{})
	query["service_id"] = s.service.Id
	query["app_id"] = app.Id

	collection := Mdb.C("service_apps")
	qtd, err := collection.Find(query).Count()
	c.Assert(err, IsNil)

	c.Assert(qtd, Equals, 0)
}
