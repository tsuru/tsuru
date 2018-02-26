// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"sort"

	authTypes "github.com/tsuru/tsuru/types/auth"

	"github.com/globalsign/mgo/bson"
	"gopkg.in/check.v1"
)

func (s *S) TestTeamServiceCreate(c *check.C) {
	one := authTypes.User{Email: "king@pos.com"}
	err := TeamService().Create("pos", &one)
	c.Assert(err, check.IsNil)
	team, err := TeamService().FindByName("pos")
	c.Assert(err, check.IsNil)
	c.Assert(team.CreatingUser, check.Equals, one.Email)
}

func (s *S) TestTeamServiceCreateDuplicate(c *check.C) {
	u := authTypes.User{Email: "king@pos.com"}
	err := TeamService().Create("pos", &u)
	c.Assert(err, check.IsNil)
	err = TeamService().Create("pos", &u)
	c.Assert(err, check.Equals, authTypes.ErrTeamAlreadyExists)
}

func (s *S) TestTeamServiceCreateTrimsName(c *check.C) {
	u := authTypes.User{Email: "king@pos.com"}
	err := TeamService().Create("pos    ", &u)
	c.Assert(err, check.IsNil)
	_, err = TeamService().FindByName("pos")
	c.Assert(err, check.IsNil)
}

func (s *S) TestTeamServiceCreateValidation(c *check.C) {
	u := authTypes.User{Email: "king@pos.com"}
	var tests = []struct {
		input string
		err   error
	}{
		{"", authTypes.ErrInvalidTeamName},
		{"    ", authTypes.ErrInvalidTeamName},
		{"1abc", authTypes.ErrInvalidTeamName},
		{"@abc", authTypes.ErrInvalidTeamName},
		{"my team", authTypes.ErrInvalidTeamName},
		{"Abacaxi", authTypes.ErrInvalidTeamName},
		{"TEAM", authTypes.ErrInvalidTeamName},
		{"TeaM", authTypes.ErrInvalidTeamName},
		{"team_1", nil},
		{"tsuru@corp.globo.com", nil},
		{"team-1", nil},
		{"a", authTypes.ErrInvalidTeamName},
		{"ab", nil},
		{"team1", nil},
	}
	for _, t := range tests {
		err := TeamService().Create(t.input, &u)
		if err != t.err {
			c.Errorf("Is %q valid? Want %v. Got %v.", t.input, t.err, err)
		}
	}
}

func (s *S) TestTeamServiceRemove(c *check.C) {
	teamName := "atreides"
	u := authTypes.User(*s.user)
	err := TeamService().Create(teamName, &u)
	c.Assert(err, check.IsNil)
	err = TeamService().Remove(teamName)
	c.Assert(err, check.IsNil)
	t, err := TeamService().FindByName(teamName)
	c.Assert(err, check.Equals, authTypes.ErrTeamNotFound)
	c.Assert(t, check.IsNil)
}

func (s *S) TestTeamServiceRemoveWithApps(c *check.C) {
	teamName := "atreides"
	u := authTypes.User(*s.user)
	err := TeamService().Create(teamName, &u)
	c.Assert(err, check.IsNil)
	err = s.conn.Apps().Insert(bson.M{"name": "leto", "teams": []string{teamName}})
	c.Assert(err, check.IsNil)
	err = TeamService().Remove(teamName)
	c.Assert(err, check.ErrorMatches, "Apps: leto")
}

func (s *S) TestTeamServiceRemoveWithServiceInstances(c *check.C) {
	teamName := "harkonnen"
	u := authTypes.User(*s.user)
	err := TeamService().Create(teamName, &u)
	c.Assert(err, check.IsNil)
	err = s.conn.ServiceInstances().Insert(bson.M{"name": "vladimir", "teams": []string{teamName}})
	c.Assert(err, check.IsNil)
	err = TeamService().Remove(teamName)
	c.Assert(err, check.ErrorMatches, "Service instances: vladimir")
}

func (s *S) TestTeamServiceList(c *check.C) {
	u := authTypes.User(*s.user)
	err := TeamService().Create("corrino", &u)
	c.Assert(err, check.IsNil)
	err = TeamService().Create("fenring", &u)
	c.Assert(err, check.IsNil)
	teams, err := TeamService().List()
	c.Assert(err, check.IsNil)
	c.Assert(teams, check.HasLen, 3)
	names := []string{teams[0].Name, teams[1].Name, teams[2].Name}
	sort.Strings(names)
	c.Assert(names, check.DeepEquals, []string{"cobrateam", "corrino", "fenring"})
}
