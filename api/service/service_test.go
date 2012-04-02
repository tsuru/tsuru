package service_test

import (
	. "github.com/timeredbull/tsuru/api/app"
	. "github.com/timeredbull/tsuru/api/service"
	. "github.com/timeredbull/tsuru/database"
	. "launchpad.net/gocheck"
)

func (s *ServiceSuite) createService() {
	s.service = &Service{
		ServiceTypeId: 2,
		Name:          "my_service",
	}
	s.service.Create()
}

func (s *ServiceSuite) TestGetService(c *C) {
	s.createService()
	id := s.service.Id
	sTypeId := s.service.ServiceTypeId
	s.service.Id = 0
	s.service.ServiceTypeId = 0
	s.service.Get()

	c.Assert(s.service.Id, Equals, id)
	c.Assert(s.service.ServiceTypeId, Equals, sTypeId)
}

func (s *ServiceSuite) TestAllServices(c *C) {
	st := ServiceType{Name: "Mysql", Charm: "mysql"}
	se := Service{ServiceTypeId: st.Id, Name: "myService"}
	se2 := Service{ServiceTypeId: st.Id, Name: "myOtherService"}
	st.Create()
	se.Create()
	se2.Create()

	s_ := Service{}
	results := s_.All()
	c.Assert(len(results), Equals, 2)
}

func (s *ServiceSuite) TestCreateService(c *C) {
	s.createService()
	rows, err := Db.Query("SELECT id, service_type_id, name FROM services WHERE name = 'my_service'")
	c.Check(err, IsNil)

	var id int64
	var serviceTypeId int64
	var name string

	for rows.Next() {
		rows.Scan(&id, &serviceTypeId, &name)
	}

	c.Assert(id, Equals, s.service.Id)
	c.Assert(serviceTypeId, Equals, int64(2))
	c.Assert(name, Equals, "my_service")
}

func (s *ServiceSuite) TestDeleteService(c *C) {
	s.createService()
	s.service.Delete()

	rows, err := Db.Query("SELECT count(*) FROM services WHERE name = 'my_service'")
	c.Assert(err, IsNil)

	var qtd int
	for rows.Next() {
		rows.Scan(&qtd)
	}

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

	rows, err := Db.Query("SELECT service_id, app_id FROM service_apps WHERE service_id = ? AND app_id = ?", s.service.Id, app.Id)
	c.Assert(err, IsNil)

	var serviceId int64
	var appId int64
	for rows.Next() {
		rows.Scan(&serviceId, &appId)
	}

	c.Assert(s.service.Id, Equals, serviceId)
	c.Assert(app.Id, Equals, appId)
}

func (s *ServiceSuite) TestUnbindService(c *C) {
	s.createService()
	app := &App{Name: "my_app", Framework: "django"}
	app.Create()
	s.service.Bind(app)
	s.service.Unbind(app)

	rows, err := Db.Query("SELECT count(*) FROM service_apps WHERE service_id = ? AND app_id = ?", s.service.Id, app.Id)
	c.Assert(err, IsNil)

	var qtd int
	for rows.Next() {
		rows.Scan(&qtd)
	}

	c.Assert(qtd, Equals, 0)
}
