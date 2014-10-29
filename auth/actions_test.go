// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"fmt"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/testing"
	"gopkg.in/mgo.v2/bson"
	"launchpad.net/gocheck"
)

type ActionsSuite struct {
	conn *db.Storage
}

var _ = gocheck.Suite(&ActionsSuite{})

func (s *ActionsSuite) SetUpSuite(c *gocheck.C) {
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "tsuru_auth_actions_test")
	var err error
	s.conn, err = db.Conn()
	c.Assert(err, gocheck.IsNil)
}

func (s *ActionsSuite) TearDownSuite(c *gocheck.C) {
	conn, _ := db.Conn()
	defer conn.Close()
	conn.Apps().Database.DropDatabase()
}

func (s *ActionsSuite) TestAddKeyInGandalf(c *gocheck.C) {
	c.Assert(addKeyInGandalfAction.Name, gocheck.Equals, "add-key-in-gandalf")
}

func (s *ActionsSuite) TestAddKeyInDatatabase(c *gocheck.C) {
	c.Assert(addKeyInDatabaseAction.Name, gocheck.Equals, "add-key-in-database")
}

func (s *ActionsSuite) TestAddKeyInGandalfActionForward(c *gocheck.C) {
	h := testHandler{}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	key := &Key{Name: "mysshkey", Content: "my-ssh-key"}
	u := &User{Email: "me@gmail.com", Password: "123456"}
	ctx := action.FWContext{
		Params: []interface{}{key, u},
	}
	result, err := addKeyInGandalfAction.Forward(ctx)
	c.Assert(err, gocheck.IsNil)
	c.Assert(result, gocheck.IsNil)
	c.Assert(len(h.url), gocheck.Equals, 1)
	expected := fmt.Sprintf("/user/%s/key", u.Email)
	c.Assert(h.url[0], gocheck.Equals, expected)
}

func (s *ActionsSuite) TestAddKeyInGandalfActionBackward(c *gocheck.C) {
	h := testHandler{}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	key := &Key{Name: "mysshkey", Content: "my-ssh-key"}
	u := &User{Email: "me@gmail.com", Password: "123456"}
	ctx := action.BWContext{
		Params: []interface{}{key, u},
	}
	addKeyInGandalfAction.Backward(ctx)
	c.Assert(len(h.url), gocheck.Equals, 1)
	expected := fmt.Sprintf("/user/%s/key/%s", u.Email, key.Name)
	c.Assert(h.url[0], gocheck.Equals, expected)
}

func (s *ActionsSuite) TestAddKeyInDatabaseActionForward(c *gocheck.C) {
	key := &Key{Name: "mysshkey", Content: "my-ssh-key"}
	u := &User{Email: "me@gmail.com"}
	err := u.Create()
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	c.Assert(err, gocheck.IsNil)
	ctx := action.FWContext{
		Params: []interface{}{key, u},
	}
	result, err := addKeyInDatabaseAction.Forward(ctx)
	c.Assert(err, gocheck.IsNil)
	c.Assert(result, gocheck.IsNil)
	u, err = GetUserByEmail(u.Email)
	c.Assert(err, gocheck.IsNil)
	c.Assert(u.Keys, gocheck.DeepEquals, []Key{*key})
}

func (s *ActionsSuite) TestAddKeyInDatabaseActionBackward(c *gocheck.C) {
	ts := testing.StartGandalfTestServer(&testHandler{})
	defer ts.Close()
	key := Key{Name: "mysshkey", Content: "my-ssh-key"}
	u := &User{Email: "me@gmail.com"}
	u.AddKey(key)
	err := u.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	c.Assert(u.Keys, gocheck.DeepEquals, []Key{key})
	ctx := action.BWContext{
		Params: []interface{}{&key, u},
	}
	addKeyInDatabaseAction.Backward(ctx)
	u, err = GetUserByEmail(u.Email)
	c.Assert(err, gocheck.IsNil)
	c.Assert(u.Keys, gocheck.DeepEquals, []Key{})
}
