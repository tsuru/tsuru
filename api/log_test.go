// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

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

func (s *S) TestAddLogsHandler(c *check.C) {
	a1 := app.App{Name: "myapp1", Platform: "zend", Teams: []string{s.team.Name}}
	err := app.CreateApp(&a1, s.user)
	c.Assert(err, check.IsNil)
	defer s.deleteApp(&a1)
	a2 := app.App{Name: "myapp2", Platform: "zend", Teams: []string{s.team.Name}}
	err = app.CreateApp(&a2, s.user)
	c.Assert(err, check.IsNil)
	defer s.deleteApp(&a2)
	baseTime, err := time.Parse(time.RFC3339, "2015-06-16T15:00:00.000Z")
	c.Assert(err, check.IsNil)
	baseTime = baseTime.Local()
	bodyStr := `
	{"date": "2015-06-16T15:00:00.000Z", "message": "msg1", "source": "web", "appname": "myapp1", "unit": "unit1"}
	{"date": "2015-06-16T15:00:01.000Z", "message": "msg2", "source": "web", "appname": "myapp2", "unit": "unit2"}
	{"date": "2015-06-16T15:00:02.000Z", "message": "msg3", "source": "web", "appname": "myapp1", "unit": "unit3"}
	{"date": "2015-06-16T15:00:03.000Z", "message": "msg4", "source": "web", "appname": "myapp2", "unit": "unit4"}
	{"date": "2015-06-16T15:00:04.000Z", "message": "msg5", "source": "worker", "appname": "myapp1", "unit": "unit3"}
	`
	request, err := http.NewRequest("POST", "/logs", strings.NewReader(bodyStr))
	c.Assert(err, check.IsNil)
	token, err := nativeScheme.AppLogin(app.InternalAppName)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	logs, err := a1.LastLogs(3, app.Applog{})
	c.Assert(err, check.IsNil)
	c.Assert(logs, check.DeepEquals, []app.Applog{
		{Date: baseTime, Message: "msg1", Source: "web", AppName: "myapp1", Unit: "unit1"},
		{Date: baseTime.Add(2 * time.Second), Message: "msg3", Source: "web", AppName: "myapp1", Unit: "unit3"},
		{Date: baseTime.Add(4 * time.Second), Message: "msg5", Source: "worker", AppName: "myapp1", Unit: "unit3"},
	})
	logs, err = a2.LastLogs(2, app.Applog{})
	c.Assert(err, check.IsNil)
	c.Assert(logs, check.DeepEquals, []app.Applog{
		{Date: baseTime.Add(1 * time.Second), Message: "msg2", Source: "web", AppName: "myapp2", Unit: "unit2"},
		{Date: baseTime.Add(3 * time.Second), Message: "msg4", Source: "web", AppName: "myapp2", Unit: "unit4"},
	})
}

func (s *S) TestAddLogsHandlerInvalidToken(c *check.C) {
	request, err := http.NewRequest("POST", "/logs", strings.NewReader(""))
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}
