package provision

import (
	. "github.com/timeredbull/tsuru/api/service"
	. "launchpad.net/gocheck"
)

func (s *S) TestGetServiceOrError(c *C) {
	srv := Service{Name: "foo", OwnerTeams: []string{s.team.Name}}
	err := srv.Create()
	c.Assert(err, IsNil)
	defer srv.Delete()
	rSrv, err := GetServiceOrError("foo", s.user)
	c.Assert(err, IsNil)
	c.Assert(rSrv.Name, Equals, srv.Name)
}

func (s *S) TestServicesAndInstancesByOwnerTeams(c *C) {
	srvc := Service{Name: "mysql", OwnerTeams: []string{s.team.Name}}
	err := srvc.Create()
	c.Assert(err, IsNil)
	defer srvc.Delete()
	srvc2 := Service{Name: "mongodb"}
	err = srvc2.Create()
	c.Assert(err, IsNil)
	defer srvc2.Delete()
	sInstance := ServiceInstance{Name: "foo", ServiceName: "mysql"}
	err = sInstance.Create()
	c.Assert(err, IsNil)
	defer sInstance.Delete()
	sInstance2 := ServiceInstance{Name: "bar", ServiceName: "mongodb"}
	err = sInstance2.Create()
	defer sInstance2.Delete()
	results := ServicesAndInstancesByOwner(s.user)
	expected := []ServiceModel{
		ServiceModel{Service: "mysql", Instances: []string{"foo"}},
	}
	c.Assert(results, DeepEquals, expected)
}
