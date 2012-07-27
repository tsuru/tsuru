package provision

import (
	. "github.com/timeredbull/tsuru/api/service"
	. "launchpad.net/gocheck"
)

func (s *S) TestGetServiceOrError(c *C) {
    srv := Service{Name: "foo", OwnerTeams: []string{s.team.Name}}
    err := srv.Create()
    c.Assert(err, IsNil)
    rSrv, err := GetServiceOrError("foo", s.user)
    c.Assert(err, IsNil)
    c.Assert(rSrv.Name, Equals, srv.Name)
}
