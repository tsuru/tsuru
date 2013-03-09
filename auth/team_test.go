// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"labix.org/v2/mgo/bson"
	"launchpad.net/gocheck"
)

type userPresenceChecker struct{}

func (c *userPresenceChecker) Info() *gocheck.CheckerInfo {
	return &gocheck.CheckerInfo{Name: "ContainsUser", Params: []string{"team", "user"}}
}

func (c *userPresenceChecker) Check(params []interface{}, names []string) (bool, string) {
	team, ok := params[0].(*Team)
	if !ok {
		return false, "first parameter should be a pointer to a team instance"
	}

	user, ok := params[1].(*User)
	if !ok {
		return false, "second parameter should be a pointer to a user instance"
	}
	return team.ContainsUser(user), ""
}

var ContainsUser gocheck.Checker = &userPresenceChecker{}

func (s *S) TestGetTeamsNames(c *gocheck.C) {
	team := Team{Name: "cheese"}
	team2 := Team{Name: "eggs"}
	teamNames := GetTeamsNames([]Team{team, team2})
	c.Assert(teamNames, gocheck.DeepEquals, []string{"cheese", "eggs"})
}

func (s *S) TestShouldBeAbleToAddAUserToATeamReturningNoErrors(c *gocheck.C) {
	u := &User{Email: "nobody@globo.com"}
	t := new(Team)
	err := t.AddUser(u)
	c.Assert(err, gocheck.IsNil)
	c.Assert(t, ContainsUser, u)
}

func (s *S) TestShouldReturnErrorWhenTryingToAddAUserThatIsAlreadyInTheList(c *gocheck.C) {
	u := &User{Email: "nobody@globo.com"}
	t := &Team{Name: "timeredbull"}
	err := t.AddUser(u)
	c.Assert(err, gocheck.IsNil)
	err = t.AddUser(u)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err, gocheck.ErrorMatches, "^User nobody@globo.com is already in the team timeredbull.$")
}

func (s *S) TestRemoveUserFromTeam(c *gocheck.C) {
	t := &Team{Name: "timeredbull", Users: []string{"somebody@globo.com", "nobody@globo.com", "anybody@globo.com", "everybody@globo.com"}}
	err := t.RemoveUser(&User{Email: "somebody@globo.com"})
	c.Assert(err, gocheck.IsNil)
	c.Assert(t.Users, gocheck.DeepEquals, []string{"nobody@globo.com", "anybody@globo.com", "everybody@globo.com"})
	err = t.RemoveUser(&User{Email: "anybody@globo.com"})
	c.Assert(err, gocheck.IsNil)
	c.Assert(t.Users, gocheck.DeepEquals, []string{"nobody@globo.com", "everybody@globo.com"})
	err = t.RemoveUser(&User{Email: "everybody@globo.com"})
	c.Assert(err, gocheck.IsNil)
	c.Assert(t.Users, gocheck.DeepEquals, []string{"nobody@globo.com"})
}

func (s *S) TestShouldReturnErrorWhenTryingToRemoveAUserThatIsNotInTheTeam(c *gocheck.C) {
	u := &User{Email: "nobody@globo.com"}
	t := &Team{Name: "timeredbull"}
	err := t.RemoveUser(u)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err, gocheck.ErrorMatches, "^User nobody@globo.com is not in the team timeredbull.$")
}

func (s *S) TestCheckUserAccess(c *gocheck.C) {
	u1 := User{Email: "how-many-more-times@ledzeppelin.com"}
	err := u1.Create()
	c.Assert(err, gocheck.IsNil)
	u2 := User{Email: "whola-lotta-love@ledzeppelin.com"}
	err = u2.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Users().Remove(bson.M{"email": bson.M{"$in": []string{u1.Email, u2.Email}}})
	t := Team{Name: "ledzeppelin", Users: []string{u1.Email}}
	err = s.conn.Teams().Insert(t)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Teams().Remove(bson.M{"_id": t.Name})
	c.Assert(CheckUserAccess([]string{t.Name}, &u1), gocheck.Equals, true)
	c.Assert(CheckUserAccess([]string{t.Name}, &u2), gocheck.Equals, false)
}

func (s *S) TestCheckUserAccessWithMultipleUsersOnMultipleTeams(c *gocheck.C) {
	one := User{Email: "imone@thewho.com", Password: "123"}
	punk := User{Email: "punk@thewho.com", Password: "123"}
	cut := User{Email: "cutmyhair@thewho.com", Password: "123"}
	who := Team{Name: "TheWho", Users: []string{one.Email, punk.Email, cut.Email}}
	err := s.conn.Teams().Insert(who)
	defer s.conn.Teams().Remove(bson.M{"_id": who.Name})
	c.Assert(err, gocheck.IsNil)
	what := Team{Name: "TheWhat", Users: []string{one.Email, punk.Email}}
	err = s.conn.Teams().Insert(what)
	defer s.conn.Teams().Remove(bson.M{"_id": what.Name})
	c.Assert(err, gocheck.IsNil)
	where := Team{Name: "TheWhere", Users: []string{one.Email}}
	err = s.conn.Teams().Insert(where)
	defer s.conn.Teams().Remove(bson.M{"_id": where.Name})
	c.Assert(err, gocheck.IsNil)
	teams := []string{who.Name, what.Name, where.Name}
	defer s.conn.Teams().RemoveAll(bson.M{"_id": bson.M{"$in": teams}})
	c.Assert(CheckUserAccess(teams, &cut), gocheck.Equals, true)
	c.Assert(CheckUserAccess(teams, &punk), gocheck.Equals, true)
	c.Assert(CheckUserAccess(teams, &one), gocheck.Equals, true)
}
