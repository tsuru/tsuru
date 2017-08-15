// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	"sort"

	"github.com/tsuru/tsuru/types/auth"
	"gopkg.in/check.v1"
)

func (s *S) TestInsertTeam(c *check.C) {
	t := auth.Team{Name: "teamname", CreatingUser: "me@example.com"}
	err := s.TeamService.Insert(t)
	c.Assert(err, check.IsNil)
	team, err := s.TeamService.FindByName(t.Name)
	c.Assert(err, check.IsNil)
	c.Assert(team.Name, check.Equals, t.Name)
	c.Assert(team.CreatingUser, check.Equals, t.CreatingUser)
}

func (s *S) TestInsertDuplicateTeam(c *check.C) {
	t := auth.Team{Name: "teamname", CreatingUser: "me@example.com"}
	err := s.TeamService.Insert(t)
	c.Assert(err, check.IsNil)
	err = s.TeamService.Insert(t)
	c.Assert(err, check.Equals, auth.ErrTeamAlreadyExists)
}

func (s *S) TestFindAllTeams(c *check.C) {
	err := s.TeamService.Insert(auth.Team{Name: "corrino"})
	c.Assert(err, check.IsNil)
	err = s.TeamService.Insert(auth.Team{Name: "fenring"})
	c.Assert(err, check.IsNil)
	teams, err := s.TeamService.FindAll()
	c.Assert(err, check.IsNil)
	c.Assert(teams, check.HasLen, 2)
	names := []string{teams[0].Name, teams[1].Name}
	sort.Strings(names)
	c.Assert(names, check.DeepEquals, []string{"corrino", "fenring"})
}

func (s *S) TestFindTeamByName(c *check.C) {
	t := auth.Team{Name: "myteam"}
	err := s.TeamService.Insert(t)
	c.Assert(err, check.IsNil)
	team, err := s.TeamService.FindByName(t.Name)
	c.Assert(err, check.IsNil)
	c.Assert(team.Name, check.Equals, t.Name)
}

func (s *S) TestFindTeamByNameNotFound(c *check.C) {
	team, err := s.TeamService.FindByName("wat")
	c.Assert(err, check.Equals, auth.ErrTeamNotFound)
	c.Assert(team, check.IsNil)
}

func (s *S) TestFindTeamByNames(c *check.C) {
	t1 := auth.Team{Name: "team1"}
	err := s.TeamService.Insert(t1)
	c.Assert(err, check.IsNil)
	t2 := auth.Team{Name: "team2"}
	err = s.TeamService.Insert(t2)
	c.Assert(err, check.IsNil)
	t3 := auth.Team{Name: "team3"}
	err = s.TeamService.Insert(t3)
	c.Assert(err, check.IsNil)
	teams, err := s.TeamService.FindByNames([]string{t1.Name, t2.Name, "unknown"})
	c.Assert(err, check.IsNil)
	c.Assert(teams, check.DeepEquals, []auth.Team{t1, t2})
}

func (s *S) TestFindTeamByNamesNotFound(c *check.C) {
	t1 := auth.Team{Name: "team1"}
	err := s.TeamService.Insert(t1)
	c.Assert(err, check.IsNil)
	teams, err := s.TeamService.FindByNames([]string{"unknown", "otherteam"})
	c.Assert(err, check.IsNil)
	c.Assert(teams, check.HasLen, 0)
}

func (s *S) TestDeleteTeam(c *check.C) {
	team := auth.Team{Name: "atreides"}
	err := s.TeamService.Insert(team)
	c.Assert(err, check.IsNil)
	err = s.TeamService.Delete(team)
	c.Assert(err, check.IsNil)
	t, err := s.TeamService.FindByName("atreides")
	c.Assert(err, check.Equals, auth.ErrTeamNotFound)
	c.Assert(t, check.IsNil)
}

func (s *S) TestDeleteTeamNotFound(c *check.C) {
	err := s.TeamService.Delete(auth.Team{Name: "myteam"})
	c.Assert(err, check.Equals, auth.ErrTeamNotFound)
}
