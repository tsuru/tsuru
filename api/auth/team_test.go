package auth

import (
	. "launchpad.net/gocheck"
)

type userPresenceChecker struct{}

func (c *userPresenceChecker) Info() *CheckerInfo {
	return &CheckerInfo{Name: "ContainsUser", Params: []string{"team", "user"}}
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

var ContainsUser Checker = &userPresenceChecker{}

func (s *S) TestShouldBeAbleToAddAUserToATeamReturningNoErrors(c *C) {
	u := &User{Email: "nobody@globo.com"}
	t := new(Team)
	err := t.AddUser(u)
	c.Assert(err, IsNil)
	c.Assert(t, ContainsUser, u)
}

func (s *S) TestShouldReturnErrorWhenTryingToAddAUserThatIsAlreadyInTheList(c *C) {
	u := &User{Email: "nobody@globo.com"}
	t := &Team{Name: "timeredbull"}
	err := t.AddUser(u)
	c.Assert(err, IsNil)
	err = t.AddUser(u)
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^User nobody@globo.com is alread in the team timeredbull.$")
}

func (s *S) TestShouldBeAbleToRemoveAUserFromATeamReturningNoErrors(c *C) {
	u := &User{Email: "nobody@globo.com"}
	t := &Team{Name: "timeredbull"}
	err := t.AddUser(u)
	c.Assert(err, IsNil)
	err = t.RemoveUser(u)
	c.Assert(err, IsNil)
	c.Assert(t, Not(ContainsUser), u)
}

func (s *S) TestShouldReturnErrorWhenTryingToRemoveAUserThatIsNotInTheTeam(c *C) {
	u := &User{Email: "nobody@globo.com"}
	t := &Team{Name: "timeredbull"}
	err := t.RemoveUser(u)
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^User nobody@globo.com is not in the team timeredbull.$")
}
