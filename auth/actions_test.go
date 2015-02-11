// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/repository"
	"github.com/tsuru/tsuru/repository/repositorytest"
	"gopkg.in/check.v1"
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

func (s *ActionsSuite) SetUpTest(c *check.C) {
	config.Set("repo-manager", "fake")
	repositorytest.Reset()
}

func (s *ActionsSuite) TestAddKeyInRepository(c *check.C) {
	c.Assert(addKeyInRepositoryAction.Name, check.Equals, "add-key-in-repository")
}

func (s *ActionsSuite) TestAddKeyInDatatabase(c *check.C) {
	c.Assert(addKeyInDatabaseAction.Name, check.Equals, "add-key-in-database")
}

func (s *ActionsSuite) TestAddKeyInRepositoryActionForward(c *check.C) {
	err := repository.Manager().CreateUser("me@gmail.com")
	c.Assert(err, check.IsNil)
	key := &Key{Name: "mysshkey", Content: "my-ssh-key"}
	u := &User{Email: "me@gmail.com", Password: "123456"}
	ctx := action.FWContext{
		Params: []interface{}{key, u},
	}
	result, err := addKeyInRepositoryAction.Forward(ctx)
	c.Assert(err, check.IsNil)
	c.Assert(result, check.IsNil)
	keys, err := repository.Manager().ListKeys("me@gmail.com")
	c.Assert(err, check.IsNil)
	c.Assert(keys, check.DeepEquals, []repository.Key{key.RepoKey()})
}

func (s *ActionsSuite) TestAddKeyInRepositoryActionBackward(c *check.C) {
	err := repository.Manager().CreateUser("me@gmail.com")
	c.Assert(err, check.IsNil)
	key := &Key{Name: "mysshkey", Content: "my-ssh-key"}
	err = repository.Manager().AddKey("me@gmail.com", key.RepoKey())
	c.Assert(err, check.IsNil)
	u := &User{Email: "me@gmail.com", Password: "123456"}
	ctx := action.BWContext{
		Params: []interface{}{key, u},
	}
	addKeyInRepositoryAction.Backward(ctx)
	keys, err := repository.Manager().ListKeys("me@gmail.com")
	c.Assert(err, check.IsNil)
	c.Assert(keys, check.HasLen, 0)
}

func (s *ActionsSuite) TestAddKeyInDatabaseActionForward(c *check.C) {
	key := &Key{Name: "mysshkey", Content: "my-ssh-key"}
	u := &User{Email: "me@gmail.com"}
	err := u.Create()
	c.Assert(err, check.IsNil)
	defer u.Delete()
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
	key := Key{Name: "mysshkey", Content: "my-ssh-key"}
	u := &User{Email: "super.me@gmail.com", Keys: []Key{key}}
	err := u.Create()
	c.Assert(err, check.IsNil)
	defer u.Delete()
	c.Assert(u.Keys, check.DeepEquals, []Key{key})
	ctx := action.BWContext{
		Params: []interface{}{&key, u},
	}
	addKeyInDatabaseAction.Backward(ctx)
	u, err = GetUserByEmail(u.Email)
	c.Assert(err, check.IsNil)
	c.Assert(u.Keys, check.HasLen, 0)
}
