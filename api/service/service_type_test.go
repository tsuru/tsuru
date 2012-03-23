package service_test

import (
	. "github.com/timeredbull/tsuru/api/service"
	. "launchpad.net/gocheck"
)

func (s *ServiceSuite) createServiceType() {
	s.serviceType = &ServiceType{Name: "Mysql", Charm: "mysql"}
	s.serviceType.Create()
}

func (s *ServiceSuite) TestAllServiceTypes(c *C) {
	st := ServiceType{Name: "Mysql", Charm: "mysql"}
	st2 := ServiceType{Name: "MongoDB", Charm: "mongodb"}
	st.Create()
	st2.Create()

	results := st.All()
	c.Assert(len(results), Equals, 2)
}

func (s *ServiceSuite) TestGetServiceType(c *C) {
	s.createServiceType()
	name := s.serviceType.Name
	charm := s.serviceType.Charm

	s.serviceType.Charm = ""
	s.serviceType.Name = ""

	s.serviceType.Get()

	c.Assert(s.serviceType.Name, Equals, name)
	c.Assert(s.serviceType.Charm, Equals, charm)
}

func (s *ServiceSuite) TestCreateServiceType(c *C) {
	s.createServiceType()
	rows, err := s.db.Query("SELECT name, charm FROM service_types WHERE name = 'Mysql' AND charm='mysql'")
	c.Check(err, IsNil)

	var name string
	var charm string
	for rows.Next() {
		rows.Scan(&name, &charm)
	}

	c.Assert(name, Equals, "Mysql")
	c.Assert(charm, Equals, "mysql")
}

func (s *ServiceSuite) TestDeleteServiceType(c *C) {
	s.createServiceType()
	s.serviceType.Delete()

	rows, err := s.db.Query("SELECT count(*) FROM service_types WHERE name = 'Mysql' AND charm = 'mysql'")
	c.Assert(err, IsNil)

	var qtd int
	for rows.Next() {
		rows.Scan(&qtd)
	}

	c.Assert(qtd, Equals, 0)
}
