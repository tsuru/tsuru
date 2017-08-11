// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	"sort"

	"github.com/tsuru/tsuru/types/auth"
	"gopkg.in/check.v1"
)

var Service = &TeamService{}

func (s *S) TestInsert(c *check.C) {
	t := auth.Team{Name: "teamname", CreatingUser: "me@example.com"}
	err := Service.Insert(t)
	c.Assert(err, check.IsNil)
	team, err := Service.FindByName(t.Name)
	c.Assert(err, check.IsNil)
	c.Assert(team.Name, check.Equals, t.Name)
	c.Assert(team.CreatingUser, check.Equals, t.CreatingUser)
}

func (s *S) TestInsertDuplicateTeam(c *check.C) {
	t := auth.Team{Name: "teamname", CreatingUser: "me@example.com"}
	err := Service.Insert(t)
	c.Assert(err, check.IsNil)
	err = Service.Insert(t)
	c.Assert(err, check.Equals, auth.ErrTeamAlreadyExists)
}

func (s *S) TestFindAll(c *check.C) {
	err := Service.Insert(auth.Team{Name: "corrino"})
	c.Assert(err, check.IsNil)
	err = Service.Insert(auth.Team{Name: "fenring"})
	c.Assert(err, check.IsNil)
	teams, err := Service.FindAll()
	c.Assert(err, check.IsNil)
	c.Assert(teams, check.HasLen, 2)
	names := []string{teams[0].Name, teams[1].Name}
	sort.Strings(names)
	c.Assert(names, check.DeepEquals, []string{"corrino", "fenring"})
}

func (s *S) TestFindByName(c *check.C) {
	t := auth.Team{Name: "myteam"}
	err := Service.Insert(t)
	c.Assert(err, check.IsNil)
	team, err := Service.FindByName(t.Name)
	c.Assert(err, check.IsNil)
	c.Assert(team.Name, check.Equals, t.Name)
}

func (s *S) TestFindByNameNotFound(c *check.C) {
	team, err := Service.FindByName("wat")
	c.Assert(err, check.Equals, auth.ErrTeamNotFound)
	c.Assert(team, check.IsNil)
}

func (s *S) TestFindByNames(c *check.C) {
	t1 := auth.Team{Name: "team1"}
	err := Service.Insert(t1)
	c.Assert(err, check.IsNil)
	t2 := auth.Team{Name: "team2"}
	err = Service.Insert(t2)
	c.Assert(err, check.IsNil)
	t3 := auth.Team{Name: "team3"}
	err = Service.Insert(t3)
	c.Assert(err, check.IsNil)
	teams, err := Service.FindByNames([]string{t1.Name, t2.Name, "unknown"})
	c.Assert(err, check.IsNil)
	c.Assert(teams, check.DeepEquals, []auth.Team{t1, t2})
}

func (s *S) TestFindByNamesNotFound(c *check.C) {
	t1 := auth.Team{Name: "team1"}
	err := Service.Insert(t1)
	c.Assert(err, check.IsNil)
	teams, err := Service.FindByNames([]string{"unknown", "otherteam"})
	c.Assert(err, check.IsNil)
	c.Assert(teams, check.HasLen, 0)
}

func (s *S) TestDelete(c *check.C) {
	team := auth.Team{Name: "atreides"}
	err := Service.Insert(team)
	c.Assert(err, check.IsNil)
	err = Service.Delete(team)
	c.Assert(err, check.IsNil)
	t, err := Service.FindByName("atreides")
	c.Assert(err, check.Equals, auth.ErrTeamNotFound)
	c.Assert(t, check.IsNil)
}

func (s *S) TestDeleteTeamNotFound(c *check.C) {
	err := Service.Delete(auth.Team{Name: "myteam"})
	c.Assert(err, check.Equals, auth.ErrTeamNotFound)
}
