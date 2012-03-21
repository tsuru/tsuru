package service_test

import (
	. "github.com/timeredbull/tsuru/api/service"
	. "launchpad.net/gocheck"
)

func (s *ServiceSuite) TestCreate(c *C) {
	serviceBinding := Service{
		AppId:           2,
		Name:            "my_service",
	}
	err := serviceBinding.Create()

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
