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
	"github.com/tsuru/tsuru/repository/repositorytest"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

type LogSuite struct {
	conn    *db.Storage
	logConn *db.LogStorage
	token   auth.Token
	team    *auth.Team
}

var _ = check.Suite(&LogSuite{})

func (s *LogSuite) createUserAndTeam(c *check.C) {
	user := &auth.User{Email: "whydidifall@thewho.com", Password: "123456"}
	_, err := nativeScheme.Create(user)
	c.Assert(err, check.IsNil)
	s.team = &auth.Team{Name: "tsuruteam", Users: []string{user.Email}}
	err = s.conn.Teams().Insert(s.team)
	c.Assert(err, check.IsNil)
	s.token, err = nativeScheme.Login(map[string]string{"email": user.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
}

func (s *LogSuite) SetUpSuite(c *check.C) {
	repositorytest.Reset()
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "tsuru_log_api_tests")
	config.Set("auth:hash-cost", 4)
	config.Set("repo-manager", "fake")
	var err error
	s.conn, err = db.Conn()
	c.Assert(err, check.IsNil)
	s.logConn, err = db.LogConn()
	c.Assert(err, check.IsNil)
	s.createUserAndTeam(c)
}

func (s *LogSuite) TearDownSuite(c *check.C) {
	dbtest.ClearAllCollections(s.conn.Apps().Database)
	s.conn.Close()
	s.logConn.Close()
}

func (s *LogSuite) SetUpTest(c *check.C) {
	repositorytest.Reset()
}

func (s *LogSuite) TestLogRemoveAll(c *check.C) {
	a := app.App{Name: "words"}
	request, err := http.NewRequest("DELETE", "/logs", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	err = a.Log("last log msg", "tsuru", "")
	c.Assert(err, check.IsNil)
	err = logRemove(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	count, err := s.logConn.Logs(a.Name).Find(nil).Count()
	c.Assert(err, check.IsNil)
	c.Assert(count, check.Equals, 0)
}

func (s *LogSuite) TestLogRemoveByApp(c *check.C) {
	a := app.App{
		Name:  "words",
		Teams: []string{s.team.Name},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	err = a.Log("last log msg", "tsuru", "")
	c.Assert(err, check.IsNil)
	a2 := app.App{Name: "words2"}
	err = s.conn.Apps().Insert(a2)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a2.Name})
	err = a2.Log("last log msg2", "tsuru", "")
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/logs?app=%s", a.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = logRemove(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	count, err := s.logConn.Logs(a2.Name).Find(nil).Count()
	c.Assert(err, check.IsNil)
	c.Assert(count, check.Equals, 1)
}
