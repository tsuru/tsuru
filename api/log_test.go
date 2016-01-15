// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"fmt"
	"io"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/repository/repositorytest"
	"golang.org/x/net/websocket"
	"gopkg.in/check.v1"
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
	s.team = &auth.Team{Name: "tsuruteam"}
	err = s.conn.Teams().Insert(s.team)
	c.Assert(err, check.IsNil)
	s.token, err = nativeScheme.Login(map[string]string{"email": user.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
}

func (s *LogSuite) SetUpSuite(c *check.C) {
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "tsuru_log_api_tests")
	config.Set("auth:hash-cost", 4)
	config.Set("repo-manager", "fake")
	app.LogPubSubQueuePrefix = "pubsub:api-log-test:"
}

func (s *LogSuite) SetUpTest(c *check.C) {
	repositorytest.Reset()
	var err error
	s.conn, err = db.Conn()
	c.Assert(err, check.IsNil)
	dbtest.ClearAllCollections(s.conn.Apps().Database)
	s.logConn, err = db.LogConn()
	c.Assert(err, check.IsNil)
	s.createUserAndTeam(c)
}

func (s *LogSuite) TearDownTest(c *check.C) {
	s.conn.Close()
	s.logConn.Close()
}

func (s *LogSuite) TearDownSuite(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	conn.Apps().Database.DropDatabase()
	logConn, err := db.LogConn()
	c.Assert(err, check.IsNil)
	defer logConn.Close()
	logConn.Logs("myapp").Database.DropDatabase()
}

func (s *S) TestAddLogsHandler(c *check.C) {
	a1 := app.App{Name: "myapp1", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(&a1, s.user)
	c.Assert(err, check.IsNil)
	a2 := app.App{Name: "myapp2", Platform: "zend", TeamOwner: s.team.Name}
	err = app.CreateApp(&a2, s.user)
	c.Assert(err, check.IsNil)
	baseTime, err := time.Parse(time.RFC3339, "2016-06-16T15:00:00.000Z")
	c.Assert(err, check.IsNil)
	baseTime = baseTime.Local()
	bodyStr := `
	{"date": "2016-06-16T15:00:00.000Z", "message": "msg1", "source": "web", "appname": "myapp1", "unit": "unit1"}
	{"date": "2016-06-16T15:00:01.000Z", "message": "msg2", "source": "web", "appname": "myapp2", "unit": "unit2"}
	{"date": "2016-06-16T15:00:02.000Z", "message": "msg3", "source": "web", "appname": "myapp1", "unit": "unit3"}
	{"date": "2016-06-16T15:00:03.000Z", "message": "msg4", "source": "web", "appname": "myapp2", "unit": "unit4"}
	{"date": "2016-06-16T15:00:04.000Z", "message": "msg5", "source": "worker", "appname": "myapp1", "unit": "unit3"}
	`
	token, err := nativeScheme.AppLogin(app.InternalAppName)
	c.Assert(err, check.IsNil)
	m := RunServer(true)
	srv := httptest.NewServer(m)
	defer srv.Close()
	testServerUrl, err := url.Parse(srv.URL)
	c.Assert(err, check.IsNil)
	wsUrl := fmt.Sprintf("ws://%s/logs", testServerUrl.Host)
	config, err := websocket.NewConfig(wsUrl, "ws://localhost/")
	c.Assert(err, check.IsNil)
	config.Header.Set("Authorization", "bearer "+token.GetValue())
	wsConn, err := websocket.DialConfig(config)
	c.Assert(err, check.IsNil)
	defer wsConn.Close()
	_, err = wsConn.Write([]byte(bodyStr))
	c.Assert(err, check.IsNil)
	timeout := time.After(5 * time.Second)
	for {
		var (
			logs1 []app.Applog
			logs2 []app.Applog
		)
		logs1, err = a1.LastLogs(3, app.Applog{})
		c.Assert(err, check.IsNil)
		logs2, err = a2.LastLogs(2, app.Applog{})
		c.Assert(err, check.IsNil)
		if len(logs1) == 3 && len(logs2) == 2 {
			break
		}
		select {
		case <-timeout:
			c.Fatal("timeout waiting for logs")
			break
		default:
		}
	}
	logs, err := a1.LastLogs(3, app.Applog{})
	c.Assert(err, check.IsNil)
	sort.Sort(LogList(logs))
	c.Assert(logs, check.DeepEquals, []app.Applog{
		{Date: baseTime, Message: "msg1", Source: "web", AppName: "myapp1", Unit: "unit1"},
		{Date: baseTime.Add(2 * time.Second), Message: "msg3", Source: "web", AppName: "myapp1", Unit: "unit3"},
		{Date: baseTime.Add(4 * time.Second), Message: "msg5", Source: "worker", AppName: "myapp1", Unit: "unit3"},
	})
	logs, err = a2.LastLogs(2, app.Applog{})
	c.Assert(err, check.IsNil)
	sort.Sort(LogList(logs))
	c.Assert(logs, check.DeepEquals, []app.Applog{
		{Date: baseTime.Add(time.Second), Message: "msg2", Source: "web", AppName: "myapp2", Unit: "unit2"},
		{Date: baseTime.Add(3 * time.Second), Message: "msg4", Source: "web", AppName: "myapp2", Unit: "unit4"},
	})
}

func (s *S) TestAddLogsHandlerInvalidToken(c *check.C) {
	m := RunServer(true)
	srv := httptest.NewServer(m)
	defer srv.Close()
	testServerUrl, err := url.Parse(srv.URL)
	c.Assert(err, check.IsNil)
	wsUrl := fmt.Sprintf("ws://%s/logs", testServerUrl.Host)
	config, err := websocket.NewConfig(wsUrl, "ws://localhost/")
	c.Assert(err, check.IsNil)
	config.Header.Set("Authorization", "bearer "+s.token.GetValue())
	wsConn, err := websocket.DialConfig(config)
	c.Assert(err, check.IsNil)
	defer wsConn.Close()
	_, err = wsConn.Write([]byte("a"))
	buffer := make([]byte, 1024)
	n, err := wsConn.Read(buffer)
	c.Assert(err, check.IsNil)
	c.Assert(string(buffer[:n]), check.Equals, `{"error":"wslogs: invalid token app name: \"\""}`)
}

func (s *S) BenchmarkScanLogs(c *check.C) {
	c.StopTimer()
	var apps []app.App
	for i := 0; i < 200; i++ {
		a := app.App{Name: fmt.Sprintf("myapp-%d", i), Platform: "zend", TeamOwner: s.team.Name}
		apps = append(apps, a)
		err := app.CreateApp(&a, s.user)
		c.Assert(err, check.IsNil)
		defer app.Delete(&a, nil)
	}
	baseMsg := `{"date": "2016-06-16T15:00:00.000Z", "message": "msg-%d", "source": "web", "appname": "%s", "unit": "unit1"}` + "\n"
	for i := range apps {
		// Remove overhead for first message from app from benchmark.
		err := scanLogs(strings.NewReader(fmt.Sprintf(baseMsg, 0, apps[i].Name)))
		c.Assert(err, check.IsNil)
	}
	r, w := io.Pipe()
	go func() {
		for i := 0; i < c.N; i++ {
			msg := fmt.Sprintf(baseMsg, i, apps[i%len(apps)].Name)
			_, err := w.Write([]byte(msg))
			c.Assert(err, check.IsNil)
		}
		w.Close()
	}()
	c.StartTimer()
	err := scanLogs(r)
	c.Assert(err, check.IsNil)
	c.StopTimer()
}
