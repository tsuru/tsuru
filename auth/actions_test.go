// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"fmt"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/action"
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
	config.Set("database:name", "tsuru_auth_actions_test")
	var err error
	s.conn, err = db.Conn()
	c.Assert(err, check.IsNil)
}

func (s *ActionsSuite) TearDownSuite(c *check.C) {
	conn, _ := db.Conn()
	defer conn.Close()
	dbtest.ClearAllCollections(conn.Apps().Database)
}

func (s *ActionsSuite) TestAddKeyInGandalf(c *check.C) {
	c.Assert(addKeyInGandalfAction.Name, check.Equals, "add-key-in-gandalf")
}

func (s *ActionsSuite) TestAddKeyInDatatabase(c *check.C) {
	c.Assert(addKeyInDatabaseAction.Name, check.Equals, "add-key-in-database")
}

func (s *ActionsSuite) TestAddKeyInGandalfActionForward(c *check.C) {
	h := testHandler{}
	ts := repositorytest.StartGandalfTestServer(&h)
	defer ts.Close()
	key := &Key{Name: "mysshkey", Content: "my-ssh-key"}
	u := &User{Email: "me@gmail.com", Password: "123456"}
	ctx := action.FWContext{
		Params: []interface{}{key, u},
	}
	result, err := addKeyInGandalfAction.Forward(ctx)
	c.Assert(err, check.IsNil)
	c.Assert(result, check.IsNil)
	c.Assert(len(h.url), check.Equals, 1)
	expected := fmt.Sprintf("/user/%s/key", u.Email)
	c.Assert(h.url[0], check.Equals, expected)
}

func (s *ActionsSuite) TestAddKeyInGandalfActionBackward(c *check.C) {
	h := testHandler{}
	ts := repositorytest.StartGandalfTestServer(&h)
	defer ts.Close()
	key := &Key{Name: "mysshkey", Content: "my-ssh-key"}
	u := &User{Email: "me@gmail.com", Password: "123456"}
	ctx := action.BWContext{
		Params: []interface{}{key, u},
	}
	addKeyInGandalfAction.Backward(ctx)
	c.Assert(len(h.url), check.Equals, 1)
	expected := fmt.Sprintf("/user/%s/key/%s", u.Email, key.Name)
	c.Assert(h.url[0], check.Equals, expected)
}

func (s *ActionsSuite) TestAddKeyInDatabaseActionForward(c *check.C) {
	key := &Key{Name: "mysshkey", Content: "my-ssh-key"}
	u := &User{Email: "me@gmail.com"}
	err := u.Create()
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	c.Assert(err, check.IsNil)
	ctx := action.FWContext{
		Params: []interface{}{key, u},
	}
	result, err := addKeyInDatabaseAction.Forward(ctx)
	c.Assert(err, check.IsNil)
	c.Assert(result, check.IsNil)
	u, err = GetUserByEmail(u.Email)
	c.Assert(err, check.IsNil)
	c.Assert(u.Keys, check.DeepEquals, []Key{*key})
}

func (s *ActionsSuite) TestAddKeyInDatabaseActionBackward(c *check.C) {
	ts := repositorytest.StartGandalfTestServer(&testHandler{})
	defer ts.Close()
	key := Key{Name: "mysshkey", Content: "my-ssh-key"}
	u := &User{Email: "me@gmail.com", Keys: []Key{key}}
	err := u.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	c.Assert(u.Keys, check.DeepEquals, []Key{key})
	ctx := action.BWContext{
		Params: []interface{}{&key, u},
	}
	addKeyInDatabaseAction.Backward(ctx)
	u, err = GetUserByEmail(u.Email)
	c.Assert(err, check.IsNil)
	c.Assert(u.Keys, check.DeepEquals, []Key{})
}
