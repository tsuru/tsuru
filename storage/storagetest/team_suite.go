// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storagetest

import (
	"sort"

	"github.com/tsuru/tsuru/storage"
	"github.com/tsuru/tsuru/types/auth"
	"gopkg.in/check.v1"
)

type TeamSuite struct {
	SuiteHooks
	TeamStorage storage.TeamStorage
}

func (s *TeamSuite) SetupSuite(c *check.C) {
	driver, err := storage.GetDefaultDbDriver()
	c.Assert(err, check.IsNil)
	s.TeamStorage = driver.TeamStorage
}

func (s *TeamSuite) TestInsertTeam(c *check.C) {
	t := auth.Team{Name: "teamname", CreatingUser: "me@example.com"}
	err := s.TeamStorage.Insert(t)
	c.Assert(err, check.IsNil)
	team, err := s.TeamStorage.FindByName(t.Name)
	c.Assert(err, check.IsNil)
	c.Assert(team.Name, check.Equals, t.Name)
	c.Assert(team.CreatingUser, check.Equals, t.CreatingUser)
}

func (s *TeamSuite) TestInsertDuplicateTeam(c *check.C) {
	t := auth.Team{Name: "teamname", CreatingUser: "me@example.com"}
	err := s.TeamStorage.Insert(t)
	c.Assert(err, check.IsNil)
	err = s.TeamStorage.Insert(t)
	c.Assert(err, check.Equals, auth.ErrTeamAlreadyExists)
}

func (s *TeamSuite) TestFindAllTeams(c *check.C) {
	err := s.TeamStorage.Insert(auth.Team{Name: "corrino"})
	c.Assert(err, check.IsNil)
	err = s.TeamStorage.Insert(auth.Team{Name: "fenring"})
	c.Assert(err, check.IsNil)
	teams, err := s.TeamStorage.FindAll()
	c.Assert(err, check.IsNil)
	c.Assert(teams, check.HasLen, 2)
	names := []string{teams[0].Name, teams[1].Name}
	sort.Strings(names)
	c.Assert(names, check.DeepEquals, []string{"corrino", "fenring"})
}

func (s *TeamSuite) TestFindTeamByName(c *check.C) {
	t := auth.Team{Name: "myteam"}
	err := s.TeamStorage.Insert(t)
	c.Assert(err, check.IsNil)
	team, err := s.TeamStorage.FindByName(t.Name)
	c.Assert(err, check.IsNil)
	c.Assert(team.Name, check.Equals, t.Name)
}

func (s *TeamSuite) TestFindTeamByNameNotFound(c *check.C) {
	team, err := s.TeamStorage.FindByName("wat")
	c.Assert(err, check.Equals, auth.ErrTeamNotFound)
	c.Assert(team, check.IsNil)
}

func (s *TeamSuite) TestFindTeamByNames(c *check.C) {
	t1 := auth.Team{Name: "team1"}
	err := s.TeamStorage.Insert(t1)
	c.Assert(err, check.IsNil)
	t2 := auth.Team{Name: "team2"}
	err = s.TeamStorage.Insert(t2)
	c.Assert(err, check.IsNil)
	t3 := auth.Team{Name: "team3"}
	err = s.TeamStorage.Insert(t3)
	c.Assert(err, check.IsNil)
	teams, err := s.TeamStorage.FindByNames([]string{t1.Name, t2.Name, "unknown"})
	c.Assert(err, check.IsNil)
	c.Assert(teams, check.DeepEquals, []auth.Team{t1, t2})
}

func (s *TeamSuite) TestFindTeamByNamesNotFound(c *check.C) {
	t1 := auth.Team{Name: "team1"}
	err := s.TeamStorage.Insert(t1)
	c.Assert(err, check.IsNil)
	teams, err := s.TeamStorage.FindByNames([]string{"unknown", "otherteam"})
	c.Assert(err, check.IsNil)
	c.Assert(teams, check.HasLen, 0)
}

func (s *TeamSuite) TestDeleteTeam(c *check.C) {
	team := auth.Team{Name: "atreides"}
	err := s.TeamStorage.Insert(team)
	c.Assert(err, check.IsNil)
	err = s.TeamStorage.Delete(team)
	c.Assert(err, check.IsNil)
	t, err := s.TeamStorage.FindByName("atreides")
	c.Assert(err, check.Equals, auth.ErrTeamNotFound)
	c.Assert(t, check.IsNil)
}

func (s *TeamSuite) TestDeleteTeamNotFound(c *check.C) {
	err := s.TeamStorage.Delete(auth.Team{Name: "myteam"})
	c.Assert(err, check.Equals, auth.ErrTeamNotFound)
}
