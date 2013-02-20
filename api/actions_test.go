// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/action"
	"github.com/globocom/tsuru/auth"
	"github.com/globocom/tsuru/db"
	"labix.org/v2/mgo/bson"
	. "launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
)

type ActionsSuite struct {
	conn *db.Storage
}

var _ = Suite(&ActionsSuite{})

func (s *ActionsSuite) SetUpSuite(c *C) {
	err := config.ReadConfigFile("../etc/tsuru.conf")
	c.Assert(err, IsNil)
	config.Set("database:name", "tsuru_api_actions_test")
	s.conn, err = db.Conn()
	c.Assert(err, IsNil)
}

func (s *ActionsSuite) startGandalfTestServer(h http.Handler) *httptest.Server {
	ts := httptest.NewServer(h)
	pieces := strings.Split(ts.URL, "://")
	protocol := pieces[0]
	hostPart := strings.Split(pieces[1], ":")
	port := hostPart[1]
	host := hostPart[0]
	config.Set("git:host", host)
	portInt, _ := strconv.ParseInt(port, 10, 0)
	config.Set("git:port", portInt)
	config.Set("git:protocol", protocol)
	return ts
}

func (s *ActionsSuite) TestAddKeyInGandalfActionForward(c *C) {
	h := testHandler{}
	ts := s.startGandalfTestServer(&h)
	defer ts.Close()
	key := &auth.Key{Name: "mysshkey", Content: "my-ssh-key"}
	u := &auth.User{Email: "me@gmail.com", Password: "123456"}
	ctx := action.FWContext{
		Params: []interface{}{key, u},
	}
	result, err := addKeyInGandalfAction.Forward(ctx)
	c.Assert(err, IsNil)
	c.Assert(result, IsNil) // we're not gonna need the result
	c.Assert(len(h.url), Equals, 1)
	expected := fmt.Sprintf("/user/%s/key", u.Email)
	c.Assert(h.url[0], Equals, expected)
}

func (s *ActionsSuite) TestAddKeyInGandalfActionBackward(c *C) {
	h := testHandler{}
	ts := s.startGandalfTestServer(&h)
	defer ts.Close()
	key := &auth.Key{Name: "mysshkey", Content: "my-ssh-key"}
	u := &auth.User{Email: "me@gmail.com", Password: "123456"}
	ctx := action.BWContext{
		Params: []interface{}{key, u},
	}
	addKeyInGandalfAction.Backward(ctx)
	c.Assert(len(h.url), Equals, 1)
	expected := fmt.Sprintf("/user/%s/key/%s", u.Email, key.Name)
	c.Assert(h.url[0], Equals, expected)
}

func (s *ActionsSuite) TestAddKeyInDatabaseActionForward(c *C) {
	key := &auth.Key{Name: "mysshkey", Content: "my-ssh-key"}
	u := &auth.User{Email: "me@gmail.com", Password: "123456"}
	err := u.Create()
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	c.Assert(err, IsNil)
	ctx := action.FWContext{
		Params: []interface{}{key, u},
	}
	result, err := addKeyInDatabaseAction.Forward(ctx)
	c.Assert(err, IsNil)
	c.Assert(result, IsNil) // we do not need it
	u.Get()
	c.Assert(u.Keys, DeepEquals, []auth.Key{*key})
}

func (s *ActionsSuite) TestAddKeyInDatabaseActionBackward(c *C) {
	key := &auth.Key{Name: "mysshkey", Content: "my-ssh-key"}
	u := &auth.User{Email: "me@gmail.com", Password: "123456"}
	u.AddKey(*key)
	err := u.Create()
	c.Assert(err, IsNil)
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	c.Assert(u.Keys, DeepEquals, []auth.Key{*key}) // just in case
	ctx := action.BWContext{
		Params: []interface{}{key, u},
	}
	addKeyInDatabaseAction.Backward(ctx)
	u.Get()
	c.Assert(u.Keys, DeepEquals, []auth.Key{})
}
