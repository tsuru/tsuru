package service_test

import (
	. "github.com/timeredbull/tsuru/api/service"
	. "launchpad.net/gocheck"
)

func (s *ServiceSuite) createService() {
	s.service = &Service{
		Id: 2,
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
	s.createService()
	serviceType := ServiceType{Id: 2, Name: "Mysql", Charm: "mysql"}
	serviceType.Create()
	retrievedServiceType := s.service.ServiceType()

	c.Assert(retrievedServiceType.Id, Equals, serviceType.Id)
	c.Assert(retrievedServiceType.Name, Equals, serviceType.Name)
	c.Assert(retrievedServiceType.Charm, Equals, serviceType.Charm)
}

// func (s *ServiceSuite) TestBindService(c *C) {
// 	s.createService()
// 	s.service.Bind()
// }
