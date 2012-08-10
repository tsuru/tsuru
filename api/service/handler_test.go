package service

import (
	"github.com/timeredbull/tsuru/db"
	"labix.org/v2/mgo/bson"
	. "launchpad.net/gocheck"
)

func (s *S) TestServiceAndServiceInstancesByTeams(c *C) {
	srv := Service{Name: "mongodb", Teams: []string{s.team.Name}}
	srv.Create()
	defer db.Session.Services().Remove(bson.M{"_id": srv.Name})
	si := ServiceInstance{Name: "my_nosql", ServiceName: srv.Name}
	si.Create()
	defer si.Delete()
	obtained := ServiceAndServiceInstancesByTeams("teams", s.user)
	expected := []ServiceModel{
		ServiceModel{Service: "mongodb", Instances: []string{"my_nosql"}},
	}
	c.Assert(obtained, DeepEquals, expected)
}

func (s *S) TestServiceAndServiceInstancesByOwnerTeams(c *C) {
	srv := Service{Name: "mongodb", OwnerTeams: []string{s.team.Name}}
	srv.Create()
	defer srv.Delete()
	si := ServiceInstance{Name: "my_nosql", ServiceName: srv.Name}
	si.Create()
	defer si.Delete()
	obtained := ServiceAndServiceInstancesByTeams("owner_teams", s.user)
	expected := []ServiceModel{
		ServiceModel{Service: "mongodb", Instances: []string{"my_nosql"}},
	}
	c.Assert(obtained, DeepEquals, expected)
}

func (s *S) TestServiceAndServiceInstancesByTeamsShouldAlsoReturnServicesWithIsRestrictedFalse(c *C) {
	srv := Service{Name: "mongodb", OwnerTeams: []string{s.team.Name}}
	srv.Create()
	defer srv.Delete()
    srv2 := Service{Name: "mysql"}
    srv2.Create()
    defer srv2.Delete()
	si := ServiceInstance{Name: "my_nosql", ServiceName: srv.Name}
	si.Create()
	defer si.Delete()
	obtained := ServiceAndServiceInstancesByTeams("owner_teams", s.user)
	expected := []ServiceModel{
		ServiceModel{Service: "mongodb", Instances: []string{"my_nosql"}},
		ServiceModel{Service: "mysql"},
	}
	c.Assert(obtained, DeepEquals, expected)
}
