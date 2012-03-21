package service_test

import (
	. "github.com/timeredbull/tsuru/api/service"
	. "launchpad.net/gocheck"
)

func (s *ServiceSuite) createService() (service *Service) {
	service = &Service{
		AppId: 2,
		Name:  "my_service",
	}
	service.Create()

	return
}

func (s *ServiceSuite) TestCreate(c *C) {
	s.createService()
	rows, err := s.db.Query("SELECT app_id, name FROM service WHERE name = 'my_service'")
	c.Check(err, IsNil)

	var appId int
	var name string

	for rows.Next() {
		rows.Scan(&appId, &name)
	}

	c.Assert(name, Equals, "my_service")
	c.Assert(appId, Equals, 2)
}

func (s *ServiceSuite) TestDelete(c *C) {
	service := s.createService()
	service.Delete()

	rows, err := s.db.Query("SELECT count(*) FROM service WHERE name = 'my_service'")
	c.Assert(err, IsNil)

	var qtd int
	for rows.Next() {
		rows.Scan(&qtd)
	}

	c.Assert(qtd, Equals, 0)
}
