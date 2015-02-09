// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rec

import (
	"testing"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"gopkg.in/check.v1"
)

func Test(t *testing.T) {
	check.TestingT(t)
}

var _ = check.Suite(RecSuite{})

type RecSuite struct{}

func (RecSuite) SetUpSuite(c *check.C) {
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "tsuru_rec_test")
}

func (RecSuite) TearDownSuite(c *check.C) {
	conn, _ := db.Conn()
	defer conn.Close()
	dbtest.ClearAllCollections(conn.Apps().Database)
}

func (RecSuite) TestLog(c *check.C) {
	ch := Log("user@tsuru.io", "run-command", "ls", "-ltr")
	_, ok := <-ch
	c.Assert(ok, check.Equals, false)
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	query := map[string]interface{}{
		"user":   "user@tsuru.io",
		"action": "run-command",
		"extra":  []interface{}{"ls", "-ltr"},
	}
	defer conn.UserActions().RemoveAll(query)
	count, err := conn.UserActions().Find(query).Count()
	c.Assert(err, check.IsNil)
	c.Assert(count, check.Equals, 1)
}

func (RecSuite) TestLogConnError(c *check.C) {
	old, _ := config.Get("database:url")
	defer config.Set("database:url", old)
	config.Set("database:url", "127.0.0.1:29999")
	ch := Log("user@tsuru.io", "run-command", "ls", "-ltr")
	err, ok := <-ch
	c.Assert(ok, check.Equals, true)
	c.Assert(err, check.NotNil)
	config.Set("database:url", old)
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	query := map[string]interface{}{
		"user":   "user@tsuru.io",
		"action": "run-command",
		"extra":  []interface{}{"ls", "-ltr"},
	}
	defer conn.UserActions().RemoveAll(query)
	count, err := conn.UserActions().Find(query).Count()
	c.Assert(err, check.IsNil)
	c.Assert(count, check.Equals, 0)
}

func (RecSuite) TestLogInvalidData(c *check.C) {
	var tests = []struct {
		user     string
		action   string
		extra    []interface{}
		expected error
	}{
		{
			user:     "",
			action:   "",
			extra:    nil,
			expected: ErrMissingUser,
		},
		{
			user:     "gopher@golang.org",
			action:   "",
			extra:    nil,
			expected: ErrMissingAction,
		},
		{
			user:     "gopher@golang.org",
			action:   "do-something",
			extra:    nil,
			expected: nil,
		},
	}
	for _, t := range tests {
		ch := Log(t.user, t.action, t.extra...)
		err := <-ch
		c.Check(err, check.Equals, t.expected)
	}
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	var action userAction
	err = conn.UserActions().Find(nil).One(&action)
	c.Assert(err, check.IsNil)
	c.Assert(action.User, check.Equals, "gopher@golang.org")
	c.Assert(action.Action, check.Equals, "do-something")
}
