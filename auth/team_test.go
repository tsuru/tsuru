// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"sort"

	"github.com/tsuru/tsuru/types"
	"github.com/tsuru/tsuru/types/service"

	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

func (s *S) TestGetTeamsNames(c *check.C) {
	team := types.Team{Name: "cheese"}
	team2 := types.Team{Name: "eggs"}
	teamNames := GetTeamsNames([]types.Team{team, team2})
	c.Assert(teamNames, check.DeepEquals, []string{"cheese", "eggs"})
}

func (s *S) TestCreateTeam(c *check.C) {
	one := User{Email: "king@pos.com"}
	err := CreateTeam("pos", &one)
	c.Assert(err, check.IsNil)
	team, err := GetTeam("pos")
	c.Assert(err, check.IsNil)
	c.Assert(team.CreatingUser, check.Equals, one.Email)
}

func (s *S) TestCreateTeamDuplicate(c *check.C) {
	u := User{Email: "king@pos.com"}
	err := CreateTeam("pos", &u)
	c.Assert(err, check.IsNil)
	err = CreateTeam("pos", &u)
	c.Assert(err, check.Equals, types.ErrTeamAlreadyExists)
}

func (s *S) TestCreateTeamTrimsName(c *check.C) {
	u := User{Email: "king@pos.com"}
	err := CreateTeam("pos    ", &u)
	c.Assert(err, check.IsNil)
	_, err = GetTeam("pos")
	c.Assert(err, check.IsNil)
}

func (s *S) TestCreateTeamValidation(c *check.C) {
	u := User{Email: "king@pos.com"}
	var tests = []struct {
		input string
		err   error
	}{
		{"", types.ErrInvalidTeamName},
		{"    ", types.ErrInvalidTeamName},
		{"1abc", types.ErrInvalidTeamName},
		{"@abc", types.ErrInvalidTeamName},
		{"my team", types.ErrInvalidTeamName},
		{"Abacaxi", types.ErrInvalidTeamName},
		{"TEAM", types.ErrInvalidTeamName},
		{"TeaM", types.ErrInvalidTeamName},
		{"team_1", nil},
		{"tsuru@corp.globo.com", nil},
		{"team-1", nil},
		{"a", types.ErrInvalidTeamName},
		{"ab", nil},
		{"team1", nil},
	}
	for _, t := range tests {
		err := CreateTeam(t.input, &u)
		if err != t.err {
			c.Errorf("Is %q valid? Want %v. Got %v.", t.input, t.err, err)
		}
	}
}

func (s *S) TestGetTeam(c *check.C) {
	team := types.Team{Name: "symfonia"}
	err := service.Team().Insert(team)
	c.Assert(err, check.IsNil)
	t, err := GetTeam(team.Name)
	c.Assert(err, check.IsNil)
	c.Assert(t.Name, check.Equals, team.Name)
	t, err = GetTeam("wat")
	c.Assert(err, check.Equals, types.ErrTeamNotFound)
	c.Assert(t, check.IsNil)
}

func (s *S) TestRemoveTeam(c *check.C) {
	team := types.Team{Name: "atreides"}
	err := service.Team().Insert(team)
	c.Assert(err, check.IsNil)
	err = RemoveTeam(team.Name)
	c.Assert(err, check.IsNil)
	t, err := GetTeam("atreides")
	c.Assert(err, check.Equals, types.ErrTeamNotFound)
	c.Assert(t, check.IsNil)
}

func (s *S) TestRemoveTeamWithApps(c *check.C) {
	team := types.Team{Name: "atreides"}
	err := service.Team().Insert(team)
	c.Assert(err, check.IsNil)
	err = s.conn.Apps().Insert(bson.M{"name": "leto", "teams": []string{"atreides"}})
	c.Assert(err, check.IsNil)
	err = RemoveTeam(team.Name)
	c.Assert(err, check.ErrorMatches, "Apps: leto")
}

func (s *S) TestRemoveTeamWithServiceInstances(c *check.C) {
	team := types.Team{Name: "harkonnen"}
	err := service.Team().Insert(team)
	c.Assert(err, check.IsNil)
	err = s.conn.ServiceInstances().Insert(bson.M{"name": "vladimir", "teams": []string{"harkonnen"}})
	c.Assert(err, check.IsNil)
	err = RemoveTeam(team.Name)
	c.Assert(err, check.ErrorMatches, "Service instances: vladimir")
}

func (s *S) TestListTeams(c *check.C) {
	err := service.Team().Insert(types.Team{Name: "corrino"})
	c.Assert(err, check.IsNil)
	err = service.Team().Insert(types.Team{Name: "fenring"})
	c.Assert(err, check.IsNil)
	teams, err := ListTeams()
	c.Assert(err, check.IsNil)
	c.Assert(teams, check.HasLen, 3)
	names := []string{teams[0].Name, teams[1].Name, teams[2].Name}
	sort.Strings(names)
	c.Assert(names, check.DeepEquals, []string{"cobrateam", "corrino", "fenring"})
}
