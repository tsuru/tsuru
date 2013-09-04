// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"github.com/globocom/config"
	"github.com/globocom/tsuru/app"
	"github.com/globocom/tsuru/auth"
	"github.com/globocom/tsuru/db"
	"labix.org/v2/mgo/bson"
	"launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
)

type LogSuite struct {
	conn  *db.Storage
	token *auth.Token
}

var _ = gocheck.Suite(&LogSuite{})

func (s *LogSuite) SetUpSuite(c *gocheck.C) {
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "tsuru_log_api_tests")
	var err error
	s.conn, err = db.Conn()
	c.Assert(err, gocheck.IsNil)
}

func (s *LogSuite) TearDownSuite(c *gocheck.C) {
	s.conn.Apps().Database.DropDatabase()
	s.conn.Logs().Database.DropDatabase()
}

func (s *LogSuite) TestLogRemoveAll(c *gocheck.C) {
	a := app.App{Name: "words"}
	request, err := http.NewRequest("DELETE", "/log", nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	err = a.Log("last log msg", "tsuru")
	c.Assert(err, gocheck.IsNil)
	err = logRemove(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	count, err := s.conn.Logs().Find(nil).Count()
	c.Assert(err, gocheck.IsNil)
	c.Assert(count, gocheck.Equals, 0)
}
