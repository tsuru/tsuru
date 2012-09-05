package auth

import (
	"github.com/timeredbull/tsuru/db"
	"labix.org/v2/mgo/bson"
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
	return team.containsUser(user), ""
}

var ContainsUser Checker = &userPresenceChecker{}

func (s *S) TestGetTeamsNames(c *C) {
	team := Team{Name: "cheese"}
	team2 := Team{Name: "eggs"}
	teamNames := GetTeamsNames([]Team{team, team2})
	c.Assert(teamNames, DeepEquals, []string{"cheese", "eggs"})
}

func (s *S) TestShouldBeAbleToAddAUserToATeamReturningNoErrors(c *C) {
	u := &User{Email: "nobody@globo.com"}
	t := new(Team)
	err := t.addUser(u)
	c.Assert(err, IsNil)
	c.Assert(t, ContainsUser, u)
}

func (s *S) TestShouldReturnErrorWhenTryingToAddAUserThatIsAlreadyInTheList(c *C) {
	u := &User{Email: "nobody@globo.com"}
	t := &Team{Name: "timeredbull"}
	err := t.addUser(u)
	c.Assert(err, IsNil)
	err = t.addUser(u)
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^User nobody@globo.com is already in the team timeredbull.$")
}

func (s *S) TestShouldBeAbleToRemoveAUserFromATeamReturningNoErrors(c *C) {
	u := &User{Email: "nobody@globo.com"}
	t := &Team{Name: "timeredbull"}
	err := t.addUser(u)
	c.Assert(err, IsNil)
	err = t.removeUser(u)
	c.Assert(err, IsNil)
	c.Assert(t, Not(ContainsUser), u)
}

func (s *S) TestShouldReturnErrorWhenTryingToRemoveAUserThatIsNotInTheTeam(c *C) {
	u := &User{Email: "nobody@globo.com"}
	t := &Team{Name: "timeredbull"}
	err := t.removeUser(u)
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^User nobody@globo.com is not in the team timeredbull.$")
}

func (s *S) TestCheckUserAccess(c *C) {
	u1 := User{Email: "how-many-more-times@ledzeppelin.com"}
	err := u1.Create()
	c.Assert(err, IsNil)
	u2 := User{Email: "whola-lotta-love@ledzeppelin.com"}
	err = u2.Create()
	c.Assert(err, IsNil)
	defer db.Session.Users().Remove(bson.M{"email": bson.M{"$in": []string{u1.Email, u2.Email}}})
	t := Team{Name: "ledzeppelin", Users: []string{u1.Email}}
	err = db.Session.Teams().Insert(t)
	c.Assert(err, IsNil)
	defer db.Session.Teams().Remove(bson.M{"_id": t.Name})
	c.Assert(CheckUserAccess([]string{t.Name}, &u1), Equals, true)
	c.Assert(CheckUserAccess([]string{t.Name}, &u2), Equals, false)
}

func (s *S) TestCheckUserAccessWithMultipleUsersOnMultipleTeams(c *C) {
	one := User{Email: "imone@thewho.com", Password: "123"}
	punk := User{Email: "punk@thewho.com", Password: "123"}
	cut := User{Email: "cutmyhair@thewho.com", Password: "123"}
	who := Team{Name: "TheWho", Users: []string{one.Email, punk.Email, cut.Email}}
	err := db.Session.Teams().Insert(who)
	defer db.Session.Teams().Remove(bson.M{"_id": who.Name})
	c.Assert(err, IsNil)
	what := Team{Name: "TheWhat", Users: []string{one.Email, punk.Email}}
	err = db.Session.Teams().Insert(what)
	defer db.Session.Teams().Remove(bson.M{"_id": what.Name})
	c.Assert(err, IsNil)
	where := Team{Name: "TheWhere", Users: []string{one.Email}}
	err = db.Session.Teams().Insert(where)
	defer db.Session.Teams().Remove(bson.M{"_id": where.Name})
	c.Assert(err, IsNil)
	teams := []string{who.Name, what.Name, where.Name}
	defer db.Session.Teams().RemoveAll(bson.M{"_id": bson.M{"$in": teams}})
	c.Assert(CheckUserAccess(teams, &cut), Equals, true)
	c.Assert(CheckUserAccess(teams, &punk), Equals, true)
	c.Assert(CheckUserAccess(teams, &one), Equals, true)
}
