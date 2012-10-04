package consumption

import (
	. "github.com/globocom/tsuru/api/service"
	. "launchpad.net/gocheck"
)

func (s *S) TestGetServiceOrError(c *C) {
	srv := Service{Name: "foo", Teams: []string{s.team.Name}, IsRestricted: true}
	err := srv.Create()
	c.Assert(err, IsNil)
	rSrv, err := GetServiceOrError("foo", s.user)
	c.Assert(err, IsNil)
	c.Assert(rSrv.Name, Equals, srv.Name)
}

func (s *S) TestGetServiceOrErrorShouldReturnErrorWhenUserHaveNoAccessToService(c *C) {
	srv := Service{Name: "foo", IsRestricted: true}
	err := srv.Create()
	c.Assert(err, IsNil)
	_, err = GetServiceOrError("foo", s.user)
	c.Assert(err, ErrorMatches, "^This user does not have access to this service$")
}

func (s *S) TestGetServiceOrErrorShoudNotReturnErrorWhenServiceIsNotRestricted(c *C) {
	srv := Service{Name: "foo"}
	err := srv.Create()
	c.Assert(err, IsNil)
	_, err = GetServiceOrError("foo", s.user)
	c.Assert(err, IsNil)
}

func (s *S) TestGetServiceOr404(c *C) {
	_, err := GetServiceOr404("foo")
	c.Assert(err, ErrorMatches, "^Service not found$")
}

func (s *S) TestGetServiceInstanceOr404(c *C) {
	si := ServiceInstance{Name: "foo"}
	err := si.Create()
	c.Assert(err, IsNil)
	rSi, err := GetServiceInstanceOr404("foo")
	c.Assert(err, IsNil)
	c.Assert(rSi.Name, Equals, si.Name)
}

func (s *S) TestGetServiceInstanceOr404ReturnsErrorWhenInstanceDoesntExists(c *C) {
	_, err := GetServiceInstanceOr404("foo")
	c.Assert(err, ErrorMatches, "^Service instance not found$")
}

func (s *S) TestGetServiceInstanceOrError(c *C) {
	si := ServiceInstance{Name: "foo", Teams: []string{s.team.Name}}
	err := si.Create()
	c.Assert(err, IsNil)
	rSi, err := GetServiceInstanceOrError("foo", s.user)
	c.Assert(err, IsNil)
	c.Assert(rSi.Name, Equals, si.Name)
}

func (s *S) TestServiceAndServiceInstancesByTeamsShouldReturnServiceInstancesByTeam(c *C) {
	srv := Service{Name: "mongodb"}
	srv.Create()
	defer srv.Delete()
	si := ServiceInstance{Name: "my_nosql", ServiceName: srv.Name, Teams: []string{s.team.Name}}
	si.Create()
	defer si.Delete()
	si2 := ServiceInstance{Name: "some_nosql", ServiceName: srv.Name}
	si2.Create()
	defer si2.Delete()
	obtained := ServiceAndServiceInstancesByTeams(s.user)
	expected := []ServiceModel{
		ServiceModel{Service: "mongodb", Instances: []string{"my_nosql"}},
	}
	c.Assert(obtained, DeepEquals, expected)
}
