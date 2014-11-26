// Copyright 2014 tsuru authors. All rights reserved.
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
	"gopkg.in/mgo.v2/bson"
	"launchpad.net/gocheck"
)

type AutoScaleSuite struct {
	conn  *db.Storage
	token auth.Token
}

var _ = gocheck.Suite(&AutoScaleSuite{})

func (s *AutoScaleSuite) SetUpSuite(c *gocheck.C) {
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "tsuru_autoscale_api_tests")
	config.Set("aut:hash-cost", 4)
	config.Set("admin-team", "tsuruteam")
	var err error
	s.conn, err = db.Conn()
	c.Assert(err, gocheck.IsNil)
	user := &auth.User{Email: "whydidifall@thewho.com", Password: "123456"}
	nativeScheme := auth.ManagedScheme(native.NativeScheme{})
	app.AuthScheme = nativeScheme
	_, err = nativeScheme.Create(user)
	c.Assert(err, gocheck.IsNil)
	team := &auth.Team{Name: "tsuruteam", Users: []string{user.Email}}
	err = s.conn.Teams().Insert(team)
	c.Assert(err, gocheck.IsNil)
	s.token, err = nativeScheme.Login(map[string]string{"email": user.Email, "password": "123456"})
	c.Assert(err, gocheck.IsNil)
}

func (s *AutoScaleSuite) TearDownSuite(c *gocheck.C) {
	defer s.conn.Close()
	s.conn.Apps().Database.DropDatabase()
}

func (s *AutoScaleSuite) TearDownTest(c *gocheck.C) {
	s.conn.AutoScale().RemoveAll(nil)
}

func (s *AutoScaleSuite) TestAutoScaleHistoryHandler(c *gocheck.C) {
	a := app.App{Name: "myApp", Platform: "Django"}
	_, err := app.NewAutoScaleEvent(&a, "increase")
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/autoscale", nil)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusOK)
	body := recorder.Body.Bytes()
	events := []app.AutoScaleEvent{}
	err = json.Unmarshal(body, &events)
	c.Assert(err, gocheck.IsNil)
	c.Assert(events, gocheck.HasLen, 1)
	c.Assert(events[0].Type, gocheck.Equals, "increase")
	c.Assert(events[0].AppName, gocheck.Equals, a.Name)
	c.Assert(events[0].StartTime, gocheck.Not(gocheck.DeepEquals), time.Time{})
}

func (s *AutoScaleSuite) TestAutoScaleHistoryHandlerByApp(c *gocheck.C) {
	a := app.App{Name: "myApp", Platform: "Django"}
	_, err := app.NewAutoScaleEvent(&a, "increase")
	c.Assert(err, gocheck.IsNil)
	a = app.App{Name: "another", Platform: "Django"}
	_, err = app.NewAutoScaleEvent(&a, "increase")
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/autoscale?app=another", nil)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusOK)
	body := recorder.Body.Bytes()
	events := []app.AutoScaleEvent{}
	err = json.Unmarshal(body, &events)
	c.Assert(err, gocheck.IsNil)
	c.Assert(events, gocheck.HasLen, 1)
	c.Assert(events[0].Type, gocheck.Equals, "increase")
	c.Assert(events[0].AppName, gocheck.Equals, a.Name)
	c.Assert(events[0].StartTime, gocheck.Not(gocheck.DeepEquals), time.Time{})
}

func (s *AutoScaleSuite) TestAutoScaleEnable(c *gocheck.C) {
	a := app.App{Name: "myApp", Platform: "Django"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("PUT", "/autoscale/myApp/enable", nil)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusOK)
	var gotApp app.App
	err = s.conn.Apps().Find(bson.M{"name": "myApp"}).One(&gotApp)
	c.Assert(err, gocheck.IsNil)
	c.Assert(gotApp.AutoScaleConfig.Enabled, gocheck.Equals, true)
}

func (s *AutoScaleSuite) TestAutoScaleDisable(c *gocheck.C) {
	a := app.App{Name: "myApp", Platform: "Django"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("PUT", "/autoscale/myApp/disable", nil)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusOK)
	var gotApp app.App
	err = s.conn.Apps().Find(bson.M{"name": "myApp"}).One(&gotApp)
	c.Assert(err, gocheck.IsNil)
	c.Assert(gotApp.AutoScaleConfig.Enabled, gocheck.Equals, false)
}

func (s *AutoScaleSuite) TestAutoScaleConfig(c *gocheck.C) {
	a := app.App{Name: "myApp", Platform: "Django"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	recorder := httptest.NewRecorder()
	config := app.AutoScaleConfig{
		Enabled:  true,
		MinUnits: 2,
		MaxUnits: 10,
	}
	body, err := json.Marshal(&config)
	c.Assert(err, gocheck.IsNil)
	request, err := http.NewRequest("PUT", "/autoscale/myApp", bytes.NewReader(body))
	request.Header.Add("Content-Type", "application/json")
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusOK)
	var gotApp app.App
	err = s.conn.Apps().Find(bson.M{"name": "myApp"}).One(&gotApp)
	c.Assert(err, gocheck.IsNil)
	c.Assert(gotApp.AutoScaleConfig, gocheck.DeepEquals, &config)
}
