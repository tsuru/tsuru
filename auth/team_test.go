// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"sort"

	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

func (s *S) TestGetTeamsNames(c *check.C) {
	team := Team{Name: "cheese"}
	team2 := Team{Name: "eggs"}
	teamNames := GetTeamsNames([]Team{team, team2})
	c.Assert(teamNames, check.DeepEquals, []string{"cheese", "eggs"})
}

func (s *S) TestTeamAllowedApps(c *check.C) {
	team := Team{Name: "teamname"}
	err := s.conn.Teams().Insert(&team)
	c.Assert(err, check.IsNil)
	a := testApp{Name: "myapp", Teams: []string{s.team.Name}}
	err = s.conn.Apps().Insert(&a)
	c.Assert(err, check.IsNil)
	a2 := testApp{Name: "otherapp", Teams: []string{team.Name}}
	err = s.conn.Apps().Insert(&a2)
	c.Assert(err, check.IsNil)
	defer func() {
		s.conn.Apps().Remove(bson.M{"name": a.Name})
		s.conn.Apps().Remove(bson.M{"name": a2.Name})
		s.conn.Teams().RemoveId(team.Name)
	}()
	alwdApps, err := team.AllowedApps()
	c.Assert(alwdApps, check.DeepEquals, []string{a2.Name})
}

func (s *S) TestCreateTeam(c *check.C) {
	one := User{Email: "king@pos.com"}
	err := CreateTeam("pos", &one)
	c.Assert(err, check.IsNil)
	defer s.conn.Teams().Remove(bson.M{"_id": "pos"})
	team, err := GetTeam("pos")
	c.Assert(err, check.IsNil)
	c.Assert(team.CreatingUser, check.Equals, one.Email)
}

func (s *S) TestCreateTeamDuplicate(c *check.C) {
	u := User{Email: "king@pos.com"}
	err := CreateTeam("pos", &u)
	c.Assert(err, check.IsNil)
	defer s.conn.Teams().Remove(bson.M{"_id": "pos"})
	err = CreateTeam("pos", &u)
	c.Assert(err, check.Equals, ErrTeamAlreadyExists)
}

func (s *S) TestCreateTeamTrimsName(c *check.C) {
	u := User{Email: "king@pos.com"}
	err := CreateTeam("pos    ", &u)
	c.Assert(err, check.IsNil)
	defer s.conn.Teams().Remove(bson.M{"_id": "pos"})
	_, err = GetTeam("pos")
	c.Assert(err, check.IsNil)
}

func (s *S) TestCreateTeamValidation(c *check.C) {
	u := User{Email: "king@pos.com"}
	var tests = []struct {
		input string
		err   error
	}{
		{"", ErrInvalidTeamName},
		{"    ", ErrInvalidTeamName},
		{"1abc", ErrInvalidTeamName},
		{"a", ErrInvalidTeamName},
		{"@abc", ErrInvalidTeamName},
		{"my team", ErrInvalidTeamName},
		{"team-1", nil},
		{"team_1", nil},
		{"ab", nil},
		{"Abacaxi", nil},
		{"tsuru@corp.globo.com", nil},
	}
	for _, t := range tests {
		err := CreateTeam(t.input, &u)
		if err != t.err {
			c.Errorf("Is %q valid? Want %v. Got %v.", t.input, t.err, err)
		}
		defer s.conn.Teams().Remove(bson.M{"_id": t.input})
	}
}

func (s *S) TestGetTeam(c *check.C) {
	team := Team{Name: "symfonia"}
	s.conn.Teams().Insert(team)
	defer s.conn.Teams().RemoveId(team.Name)
	t, err := GetTeam("symfonia")
	c.Assert(err, check.IsNil)
	c.Assert(t.Name, check.Equals, team.Name)
	t, err = GetTeam("wat")
	c.Assert(err, check.Equals, ErrTeamNotFound)
	c.Assert(t, check.IsNil)
}

func (s *S) TestRemoveTeam(c *check.C) {
	team := Team{Name: "atreides"}
	err := s.conn.Teams().Insert(team)
	c.Assert(err, check.IsNil)
	err = RemoveTeam(team.Name)
	c.Assert(err, check.IsNil)
	t, err := GetTeam("atreides")
	c.Assert(err, check.Equals, ErrTeamNotFound)
	c.Assert(t, check.IsNil)
}

func (s *S) TestRemoveTeamWithApps(c *check.C) {
	team := Team{Name: "atreides"}
	err := s.conn.Teams().Insert(team)
	c.Assert(err, check.IsNil)
	err = s.conn.Apps().Insert(bson.M{"name": "leto", "teams": []string{"atreides"}})
	c.Assert(err, check.IsNil)
	err = RemoveTeam(team.Name)
	c.Assert(err, check.ErrorMatches, "Apps: leto")
}

func (s *S) TestRemoveTeamWithServiceInstances(c *check.C) {
	team := Team{Name: "harkonnen"}
	err := s.conn.Teams().Insert(team)
	c.Assert(err, check.IsNil)
	err = s.conn.ServiceInstances().Insert(bson.M{"name": "vladimir", "teams": []string{"harkonnen"}})
	c.Assert(err, check.IsNil)
	err = RemoveTeam(team.Name)
	c.Assert(err, check.ErrorMatches, "Service instances: vladimir")
}

func (s *S) TestListTeams(c *check.C) {
	err := s.conn.Teams().Insert(Team{Name: "corrino"})
	c.Assert(err, check.IsNil)
	err = s.conn.Teams().Insert(Team{Name: "fenring"})
	c.Assert(err, check.IsNil)
	teams, err := ListTeams()
	c.Assert(err, check.IsNil)
	c.Assert(teams, check.HasLen, 3)
	names := []string{teams[0].Name, teams[1].Name, teams[2].Name}
	sort.Strings(names)
	c.Assert(names, check.DeepEquals, []string{"cobrateam", "corrino", "fenring"})
}
