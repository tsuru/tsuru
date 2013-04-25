// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rec

import (
	"github.com/globocom/config"
	"github.com/globocom/tsuru/db"
	"launchpad.net/gocheck"
	"testing"
)

func Test(t *testing.T) {
	gocheck.TestingT(t)
}

var _ = gocheck.Suite(RecSuite{})

type RecSuite struct{}

func (RecSuite) SetUpSuite(c *gocheck.C) {
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "tsuru_rec_test")
}

func (RecSuite) TearDownSuite(c *gocheck.C) {
	conn, _ := db.Conn()
	defer conn.Close()
	conn.Apps().Database.DropDatabase()
}

func (RecSuite) TestLog(c *gocheck.C) {
	ch := Log("user@tsuru.io", "run-command", "ls", "-ltr")
	_, ok := <-ch
	c.Assert(ok, gocheck.Equals, false)
	conn, err := db.Conn()
	c.Assert(err, gocheck.IsNil)
	defer conn.Close()
	query := map[string]interface{}{
		"user":   "user@tsuru.io",
		"action": "run-command",
		"extra":  []interface{}{"ls", "-ltr"},
	}
	defer conn.UserActions().RemoveAll(query)
	count, err := conn.UserActions().Find(query).Count()
	c.Assert(err, gocheck.IsNil)
	c.Assert(count, gocheck.Equals, 1)
}

func (RecSuite) TestLogConnError(c *gocheck.C) {
	old, _ := config.Get("database:url")
	defer config.Set("database:url", old)
	config.Set("database:url", "127.0.0.1:29999")
	ch := Log("user@tsuru.io", "run-command", "ls", "-ltr")
	err, ok := <-ch
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(err, gocheck.NotNil)
	config.Set("database:url", old)
	conn, err := db.Conn()
	c.Assert(err, gocheck.IsNil)
	defer conn.Close()
	query := map[string]interface{}{
		"user":   "user@tsuru.io",
		"action": "run-command",
		"extra":  []interface{}{"ls", "-ltr"},
	}
	defer conn.UserActions().RemoveAll(query)
	count, err := conn.UserActions().Find(query).Count()
	c.Assert(err, gocheck.IsNil)
	c.Assert(count, gocheck.Equals, 0)
}

func (RecSuite) TestLogInvalidData(c *gocheck.C) {
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
		c.Check(err, gocheck.Equals, t.expected)
	}
	conn, err := db.Conn()
	c.Assert(err, gocheck.IsNil)
	defer conn.Close()
	var action userAction
	err = conn.UserActions().Find(nil).One(&action)
	c.Assert(err, gocheck.IsNil)
	c.Assert(action.User, gocheck.Equals, "gopher@golang.org")
	c.Assert(action.Action, gocheck.Equals, "do-something")
}
