// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/auth/native"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/repository/repositorytest"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

type AutoScaleSuite struct {
	conn  *db.Storage
	token auth.Token
}

var _ = check.Suite(&AutoScaleSuite{})

func (s *AutoScaleSuite) SetUpSuite(c *check.C) {
	repositorytest.Reset()
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "tsuru_autoscale_api_tests")
	config.Set("aut:hash-cost", 4)
	config.Set("admin-team", "tsuruteam")
	config.Set("repo-manager", "fake")
	var err error
	s.conn, err = db.Conn()
	c.Assert(err, check.IsNil)
	user := &auth.User{Email: "whydidifall@thewho.com", Password: "123456"}
	nativeScheme := auth.ManagedScheme(native.NativeScheme{})
	app.AuthScheme = nativeScheme
	_, err = nativeScheme.Create(user)
	c.Assert(err, check.IsNil)
	team := &auth.Team{Name: "tsuruteam", Users: []string{user.Email}}
	err = s.conn.Teams().Insert(team)
	c.Assert(err, check.IsNil)
	s.token, err = nativeScheme.Login(map[string]string{"email": user.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
}

func (s *AutoScaleSuite) TearDownSuite(c *check.C) {
	defer s.conn.Close()
	s.conn.Apps().Database.DropDatabase()
}

func (s *AutoScaleSuite) SetUpTest(c *check.C) {
	repositorytest.Reset()
}

func (s *AutoScaleSuite) TearDownTest(c *check.C) {
	s.conn.AutoScale().RemoveAll(nil)
}

func (s *AutoScaleSuite) TestAutoScaleHistoryHandler(c *check.C) {
	a := app.App{Name: "myApp", Platform: "Django"}
	_, err := app.NewAutoScaleEvent(&a, "increase")
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/autoscale", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	body := recorder.Body.Bytes()
	events := []app.AutoScaleEvent{}
	err = json.Unmarshal(body, &events)
	c.Assert(err, check.IsNil)
	c.Assert(events, check.HasLen, 1)
	c.Assert(events[0].Type, check.Equals, "increase")
	c.Assert(events[0].AppName, check.Equals, a.Name)
	c.Assert(events[0].StartTime, check.Not(check.DeepEquals), time.Time{})
}

func (s *AutoScaleSuite) TestAutoScaleHistoryHandlerByApp(c *check.C) {
	a := app.App{Name: "myApp", Platform: "Django"}
	_, err := app.NewAutoScaleEvent(&a, "increase")
	c.Assert(err, check.IsNil)
	a = app.App{Name: "another", Platform: "Django"}
	_, err = app.NewAutoScaleEvent(&a, "increase")
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/autoscale?app=another", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	body := recorder.Body.Bytes()
	events := []app.AutoScaleEvent{}
	err = json.Unmarshal(body, &events)
	c.Assert(err, check.IsNil)
	c.Assert(events, check.HasLen, 1)
	c.Assert(events[0].Type, check.Equals, "increase")
	c.Assert(events[0].AppName, check.Equals, a.Name)
	c.Assert(events[0].StartTime, check.Not(check.DeepEquals), time.Time{})
}

func (s *AutoScaleSuite) TestAutoScaleEnable(c *check.C) {
	a := app.App{Name: "myApp", Platform: "Django"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("PUT", "/autoscale/myApp/enable", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var gotApp app.App
	err = s.conn.Apps().Find(bson.M{"name": "myApp"}).One(&gotApp)
	c.Assert(err, check.IsNil)
	c.Assert(gotApp.AutoScaleConfig.Enabled, check.Equals, true)
}

func (s *AutoScaleSuite) TestAutoScaleDisable(c *check.C) {
	a := app.App{Name: "myApp", Platform: "Django"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("PUT", "/autoscale/myApp/disable", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var gotApp app.App
	err = s.conn.Apps().Find(bson.M{"name": "myApp"}).One(&gotApp)
	c.Assert(err, check.IsNil)
	c.Assert(gotApp.AutoScaleConfig.Enabled, check.Equals, false)
}

func (s *AutoScaleSuite) TestAutoScaleConfig(c *check.C) {
	a := app.App{Name: "myApp", Platform: "Django"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	recorder := httptest.NewRecorder()
	config := app.AutoScaleConfig{
		Enabled:  true,
		MinUnits: 2,
		MaxUnits: 10,
	}
	body, err := json.Marshal(&config)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("PUT", "/autoscale/myApp", bytes.NewReader(body))
	request.Header.Add("Content-Type", "application/json")
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var gotApp app.App
	err = s.conn.Apps().Find(bson.M{"name": "myApp"}).One(&gotApp)
	c.Assert(err, check.IsNil)
	c.Assert(gotApp.AutoScaleConfig, check.DeepEquals, &config)
}
