package service_test

import (
	. "github.com/timeredbull/tsuru/api/service"
	. "github.com/timeredbull/tsuru/api/app"
	. "launchpad.net/gocheck"
)

func (s *ServiceSuite) createService() {
	s.service = &Service{
		ServiceTypeId: 2,
		Name:  "my_service",
	}
	s.service.Create()
}

func (s *ServiceSuite) TestCreateService(c *C) {
	s.createService()
	rows, err := s.db.Query("SELECT id, service_type_id, name FROM service WHERE name = 'my_service'")
	c.Check(err, IsNil)

	var id int
	var serviceTypeId int
	var name string

	for rows.Next() {
		rows.Scan(&id, &serviceTypeId, &name)
	}

	c.Assert(id, Equals, 2)
	c.Assert(serviceTypeId, Equals, 2)
	c.Assert(name, Equals, "my_service")
}

func (s *ServiceSuite) TestDeleteService(c *C) {
	s.createService()
	s.service.Delete()

	rows, err := s.db.Query("SELECT count(*) FROM service WHERE name = 'my_service'")
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
		Name:  "my_service",
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

	rows, err := s.db.Query("SELECT service_id, app_id FROM service_app WHERE service_id = ? AND app_id = ?", s.service.Id, app.Id)
	c.Assert(err, IsNil)

	var serviceId int64
	var appId     int64
	for rows.Next() {
		rows.Scan(&serviceId, &appId)
	}

	c.Assert(s.service.Id, Equals, serviceId)
	c.Assert(app.Id, Equals, appId)
}
