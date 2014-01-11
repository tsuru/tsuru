// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/app"
	"github.com/globocom/tsuru/auth"
	"github.com/globocom/tsuru/db"
	"io/ioutil"
	"launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
	"time"
)

type DeploySuite struct {
	conn  *db.TsrStorage
	token *auth.Token
	team  *auth.Team
}

var _ = gocheck.Suite(&DeploySuite{})

func (s *DeploySuite) createUserAndTeam(c *gocheck.C) {
	user := &auth.User{Email: "whydidifall@thewho.com", Password: "123456"}
	err := user.Create()
	c.Assert(err, gocheck.IsNil)
	s.team = &auth.Team{Name: "tsuruteam", Users: []string{user.Email}}
	err = s.conn.Teams().Insert(s.team)
	c.Assert(err, gocheck.IsNil)
	s.token, err = user.CreateToken("123456")
	c.Assert(err, gocheck.IsNil)
}

func (s *DeploySuite) SetUpSuite(c *gocheck.C) {
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "tsuru_deploy_api_tests")
	config.Set("aut:hash-cost", 4)
	var err error
	s.conn, err = db.NewStorage()
	c.Assert(err, gocheck.IsNil)
	s.createUserAndTeam(c)
}

func (s *DeploySuite) TearDownSuite(c *gocheck.C) {
	conn, err := db.NewStorage()
	c.Assert(err, gocheck.IsNil)
	defer conn.Close()
	conn.Apps().Database.DropDatabase()
}

func (s *DeploySuite) TestDeployList(c *gocheck.C) {
	var result []app.Deploy
	conn, err := db.NewStorage()
	c.Assert(err, gocheck.IsNil)
	defer conn.Close()
	request, err := http.NewRequest("GET", "/deploys", nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	timestamp := time.Date(2013, time.November, 1, 0, 0, 0, 0, time.Local)
	duration := time.Since(timestamp)
	err = s.conn.Deploys().Insert(app.Deploy{App: "g1", Timestamp: timestamp, Duration: duration})
	c.Assert(err, gocheck.IsNil)
	err = s.conn.Deploys().Insert(app.Deploy{App: "ge", Timestamp: timestamp, Duration: duration})
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Deploys().RemoveAll(nil)
	err = deploysList(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, gocheck.IsNil)
	err = json.Unmarshal(body, &result)
	c.Assert(err, gocheck.IsNil)
	c.Assert(result[0].App, gocheck.Equals, "g1")
	c.Assert(result[0].Timestamp.In(time.UTC), gocheck.DeepEquals, timestamp.In(time.UTC))
	c.Assert(result[0].Duration, gocheck.DeepEquals, duration)
	c.Assert(result[1].App, gocheck.Equals, "ge")
	c.Assert(result[1].Timestamp.In(time.UTC), gocheck.DeepEquals, timestamp.In(time.UTC))
	c.Assert(result[1].Duration, gocheck.DeepEquals, duration)
}
