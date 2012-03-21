package service_test

import (
	. "github.com/timeredbull/tsuru/api/service"
	. "launchpad.net/gocheck"
)

func (s *ServiceSuite) TestShouldCallCreateAndSaveInDB(c *C) {
	serviceBinding := ServiceBinding{
		ServiceConfigId: 1,
		AppId:           2,
		UserId:          3,
		BindingTokenId:  4,
		Name:            "my_service",
	}
	err := serviceBinding.Create()

	rows, err := s.db.Query("SELECT service_config_id, app_id, user_id, binding_token_id, name FROM service_bindings WHERE name = 'my_service'")
	c.Check(err, IsNil)

	var serviceConfigId int
	var appId int
	var userId int
	var bindingTokenId int
	var name string

	for rows.Next() {
		rows.Scan(&serviceConfigId, &appId, &userId, &bindingTokenId, &name)
	}

	c.Assert(name, Equals, "my_service")
	c.Assert(serviceConfigId, Equals, 1)
	c.Assert(appId, Equals, 2)
	c.Assert(userId, Equals, 3)
	c.Assert(bindingTokenId, Equals, 4)
}
