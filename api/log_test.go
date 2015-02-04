// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"gopkg.in/mgo.v2/bson"
	"launchpad.net/gocheck"
)

type LogSuite struct {
	conn  *db.Storage
	token auth.Token
	team  *auth.Team
}

var _ = gocheck.Suite(&LogSuite{})

func (s *LogSuite) createUserAndTeam(c *gocheck.C) {
	user := &auth.User{Email: "whydidifall@thewho.com", Password: "123456"}
	_, err := nativeScheme.Create(user)
	c.Assert(err, gocheck.IsNil)
	s.team = &auth.Team{Name: "tsuruteam", Users: []string{user.Email}}
	err = s.conn.Teams().Insert(s.team)
	c.Assert(err, gocheck.IsNil)
	s.token, err = nativeScheme.Login(map[string]string{"email": user.Email, "password": "123456"})
	c.Assert(err, gocheck.IsNil)
}

func (s *LogSuite) SetUpSuite(c *gocheck.C) {
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "tsuru_log_api_tests")
	config.Set("auth:hash-cost", 4)
	var err error
	s.conn, err = db.Conn()
	c.Assert(err, gocheck.IsNil)
	s.createUserAndTeam(c)
}

func (s *LogSuite) TearDownSuite(c *gocheck.C) {
	dbtest.ClearAllCollections(s.conn.Apps().Database)
}

func (s *LogSuite) TestLogRemoveAll(c *gocheck.C) {
	a := app.App{Name: "words"}
	request, err := http.NewRequest("DELETE", "/logs", nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	err = a.Log("last log msg", "tsuru", "")
	c.Assert(err, gocheck.IsNil)
	err = logRemove(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	count, err := s.conn.Logs(a.Name).Find(nil).Count()
	c.Assert(err, gocheck.IsNil)
	c.Assert(count, gocheck.Equals, 0)
}

func (s *LogSuite) TestLogRemoveByApp(c *gocheck.C) {
	a := app.App{
		Name:  "words",
		Teams: []string{s.team.Name},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	err = a.Log("last log msg", "tsuru", "")
	c.Assert(err, gocheck.IsNil)
	a2 := app.App{Name: "words2"}
	err = s.conn.Apps().Insert(a2)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a2.Name})
	err = a2.Log("last log msg2", "tsuru", "")
	c.Assert(err, gocheck.IsNil)
	url := fmt.Sprintf("/logs?app=%s", a.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = logRemove(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	count, err := s.conn.Logs(a2.Name).Find(nil).Count()
	c.Assert(err, gocheck.IsNil)
	c.Assert(count, gocheck.Equals, 1)
}
