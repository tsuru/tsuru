// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	// "fmt"
	"github.com/globocom/tsuru/app"
	// "github.com/globocom/tsuru/app/bind"
	"github.com/globocom/tsuru/auth"
	"github.com/globocom/tsuru/errors"
	// "github.com/globocom/tsuru/provision"
	// "github.com/globocom/tsuru/quota"
	// "github.com/globocom/tsuru/repository"
	// "github.com/globocom/tsuru/service"
	// "github.com/globocom/tsuru/testing"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/db"
	// "io"
	"io/ioutil"
	"labix.org/v2/mgo/bson"
	"launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
	// "sort"
	// "strconv"
	// "strings"
	// "sync/atomic"
	"time"
)

type DeploySuite struct {
	conn  *db.Storage
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
	s.conn, err = db.Conn()
	c.Assert(err, gocheck.IsNil)
	s.createUserAndTeam(c)
}

func (s *DeploySuite) TearDownSuite(c *gocheck.C) {
	conn, err := db.Conn()
	c.Assert(err, gocheck.IsNil)
	defer conn.Close()
	conn.Apps().Database.DropDatabase()
}

func (s *DeploySuite) TestDeployList(c *gocheck.C) {
	var result []app.Deploy
	conn, err := db.Conn()
	c.Assert(err, gocheck.IsNil)
	defer conn.Close()
	a := app.App{Name: "g1", Teams: []string{s.team.Name}}
	request, err := http.NewRequest("GET", "/deploys?:app=g1", nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer conn.Apps().Remove(bson.M{"name": a.Name})
	timestamp := time.Date(2013, time.November, 1, 0, 0, 0, 0, time.Local)
	err = s.conn.Deploys().Insert(app.Deploy{App: "g1", Timestamp: timestamp})
	err = deploysList(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, gocheck.IsNil)
	err = json.Unmarshal(body, &result)
	c.Assert(err, gocheck.IsNil)
	c.Assert(result[0].App, gocheck.Equals, "g1")
	c.Assert(result[0].Timestamp, gocheck.Equals, timestamp)
}

func (s *DeploySuite) TestAppNotFoundListDeploy(c *gocheck.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/deploys?:app=g1", nil)
	c.Assert(err, gocheck.IsNil)
	err = deploysList(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusNotFound)
	c.Assert(e, gocheck.ErrorMatches, "^App g1 not found.$")
}
