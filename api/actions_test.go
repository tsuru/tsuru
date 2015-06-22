// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/repository/repositorytest"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

type ActionsSuite struct {
	conn *db.Storage
}

var _ = check.Suite(&ActionsSuite{})

func (s *ActionsSuite) SetUpSuite(c *check.C) {
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "tsuru_api_actions_test")
	config.Set("repo-manager", "fake")
}

func (s *ActionsSuite) SetUpTest(c *check.C) {
	var err error
	s.conn, err = db.Conn()
	c.Assert(err, check.IsNil)
	dbtest.ClearAllCollections(s.conn.Apps().Database)
	repositorytest.Reset()
}

func (s *ActionsSuite) TearDownTest(c *check.C) {
	s.conn.Close()
}

func (s *ActionsSuite) TestAddUserToTeamInRepository(c *check.C) {
	c.Assert(addUserToTeamInRepositoryAction.Name, check.Equals, "add-user-to-team-in-repository")
}

func (s *ActionsSuite) TestAddUserToTeamInDatabase(c *check.C) {
	c.Assert(addUserToTeamInDatabaseAction.Name, check.Equals, "add-user-to-team-in-database")
}

func (s *ActionsSuite) TestAddUserToTeamInRepositoryActionForward(c *check.C) {
	u := &auth.User{Email: "nobody@gmail.com"}
	err := u.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	t := &auth.Team{Name: "myteam"}
	ctx := action.FWContext{
		Params: []interface{}{u, t},
	}
	result, err := addUserToTeamInRepositoryAction.Forward(ctx)
	c.Assert(err, check.IsNil)
	c.Assert(result, check.IsNil)
}

func (s *ActionsSuite) TestAddUserToTeamInRepositoryActionBackward(c *check.C) {
	u := &auth.User{Email: "nobody@gmail.com"}
	err := u.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	t := &auth.Team{Name: "myteam"}
	ctx := action.BWContext{
		Params: []interface{}{u, t},
	}
	addUserToTeamInRepositoryAction.Backward(ctx)
}

func (s *ActionsSuite) TestAddUserToTeamInDatabaseActionForward(c *check.C) {
	newUser := &auth.User{Email: "me@gmail.com", Password: "123456"}
	err := newUser.Create()
	c.Assert(err, check.IsNil)
	t := &auth.Team{Name: "myteam"}
	err = s.conn.Teams().Insert(t)
	c.Assert(err, check.IsNil)
	defer s.conn.Teams().RemoveId(t.Name)
	defer s.conn.Users().Remove(bson.M{"email": newUser.Email})
	ctx := action.FWContext{
		Params: []interface{}{newUser, t},
	}
	result, err := addUserToTeamInDatabaseAction.Forward(ctx)
	c.Assert(err, check.IsNil)
	c.Assert(result, check.IsNil)
	err = s.conn.Teams().FindId(t.Name).One(&t)
	c.Assert(err, check.IsNil)
	c.Assert(t, ContainsUser, newUser)
}

func (s *ActionsSuite) TestAddUserToTeamInDatabaseActionBackward(c *check.C) {
	u := &auth.User{Email: "nobody@gmail.com"}
	err := u.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	t := &auth.Team{Name: "myteam", Users: []string{u.Email}}
	err = s.conn.Teams().Insert(t)
	c.Assert(err, check.IsNil)
	defer s.conn.Teams().RemoveId(t.Name)
	ctx := action.BWContext{
		Params: []interface{}{u, t},
	}
	addUserToTeamInDatabaseAction.Backward(ctx)
	err = s.conn.Teams().FindId(t.Name).One(&t)
	c.Assert(err, check.IsNil)
	c.Assert(t, check.Not(ContainsUser), &auth.User{Email: u.Email})
}
