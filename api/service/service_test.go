package service_test

import (
	. "github.com/timeredbull/tsuru/api/service"
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
	rows, err := s.db.Query("SELECT service_type_id, name FROM service WHERE name = 'my_service'")
	c.Check(err, IsNil)

	var serviceTypeId int
	var name string

	for rows.Next() {
		rows.Scan(&serviceTypeId, &name)
	}

	c.Assert(name, Equals, "my_service")
	c.Assert(serviceTypeId, Equals, 2)
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

// func (s *ServiceSuite) TestBindService(c *C) {
// 	s.createService()
// 	s.service.Bind()
// }
