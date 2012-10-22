// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

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
		{Service: "mongodb", Instances: []string{"my_nosql"}},
	}
	c.Assert(obtained, DeepEquals, expected)
}
