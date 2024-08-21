// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"context"
	"fmt"
	"io"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/tsuru/tsuru/api/shutdown"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/applog"
	"github.com/tsuru/tsuru/servicemanager"
	_ "github.com/tsuru/tsuru/storage/mongodb"
	appTypes "github.com/tsuru/tsuru/types/app"
	authTypes "github.com/tsuru/tsuru/types/auth"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"golang.org/x/net/websocket"
	check "gopkg.in/check.v1"
)

func compareLogs(c *check.C, logs1 []appTypes.Applog, logs2 []appTypes.Applog) {
	for i := range logs1 {
		logs1[i].MongoID = primitive.NilObjectID
		logs1[i].Date = logs1[i].Date.UTC()
	}
	for i := range logs2 {
		logs2[i].MongoID = primitive.NilObjectID
		logs2[i].Date = logs2[i].Date.UTC()
	}
	c.Assert(logs1, check.DeepEquals, logs2)
}

func (s *S) TestAddLogsHandler(c *check.C) {
	s.mockService.Team.OnList = func() ([]authTypes.Team, error) {
		return []authTypes.Team{{Name: s.team.Name}}, nil
	}
	a1 := app.App{Name: "myapp1", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a1, s.user)
	c.Assert(err, check.IsNil)
	a2 := app.App{Name: "myapp2", Platform: "zend", TeamOwner: s.team.Name}
	err = app.CreateApp(context.TODO(), &a2, s.user)
	c.Assert(err, check.IsNil)
	baseTime, err := time.Parse(time.RFC3339, "2015-06-16T15:00:00.000Z")
	c.Assert(err, check.IsNil)
	baseTime = baseTime.Local()
	bodyStr := `
	{"date": "2015-06-16T15:00:00.000Z", "message": "msg1", "source": "web", "name": "myapp1", "unit": "unit1"}
	{"date": "2015-06-16T15:00:01.000Z", "message": "msg2", "source": "web", "name": "myapp2", "unit": "unit2"}
	{"date": "2015-06-16T15:00:02.000Z", "message": "msg3", "source": "web", "name": "myapp1", "unit": "unit3"}
	{"date": "2015-06-16T15:00:03.000Z", "message": "msg4", "source": "web", "name": "myapp2", "unit": "unit4"}
	{"date": "2015-06-16T15:00:04.000Z", "message": "msg5", "source": "worker", "name": "myapp1", "unit": "unit3"}
	`

	srv := httptest.NewServer(s.testServer)
	defer srv.Close()
	testServerURL, err := url.Parse(srv.URL)
	c.Assert(err, check.IsNil)
	wsURL := fmt.Sprintf("ws://%s/logs", testServerURL.Host)
	config, err := websocket.NewConfig(wsURL, "ws://localhost/")
	c.Assert(err, check.IsNil)
	config.Header.Set("Authorization", "bearer "+s.token.GetValue())
	wsConn, err := websocket.DialConfig(config)
	c.Assert(err, check.IsNil)
	defer wsConn.Close()
	_, err = wsConn.Write([]byte(bodyStr))
	c.Assert(err, check.IsNil)
	timeout := time.After(5 * time.Second)
loop:
	for {
		var (
			logs1 []appTypes.Applog
			logs2 []appTypes.Applog
		)
		logs1, err = a1.LastLogs(context.TODO(), servicemanager.LogService, appTypes.ListLogArgs{
			Limit: 3,
		})
		c.Assert(err, check.IsNil)
		logs2, err = a2.LastLogs(context.TODO(), servicemanager.LogService, appTypes.ListLogArgs{
			Limit: 2,
		})
		c.Assert(err, check.IsNil)
		if len(logs1) == 3 && len(logs2) == 2 {
			break
		}
		select {
		case <-timeout:
			c.Fatal("timeout waiting for logs")
			break loop
		default:
		}
	}
	logs, err := a1.LastLogs(context.TODO(), servicemanager.LogService, appTypes.ListLogArgs{
		Limit: 3,
	})
	c.Assert(err, check.IsNil)
	sort.Sort(LogList(logs))
	compareLogs(c, logs, []appTypes.Applog{
		{Date: baseTime, Message: "msg1", Source: "web", Name: "myapp1", Unit: "unit1"},
		{Date: baseTime.Add(2 * time.Second), Message: "msg3", Source: "web", Name: "myapp1", Unit: "unit3"},
		{Date: baseTime.Add(4 * time.Second), Message: "msg5", Source: "worker", Name: "myapp1", Unit: "unit3"},
	})
	logs, err = a2.LastLogs(context.TODO(), servicemanager.LogService, appTypes.ListLogArgs{
		Limit: 2,
	})
	c.Assert(err, check.IsNil)
	sort.Sort(LogList(logs))
	compareLogs(c, logs, []appTypes.Applog{
		{Date: baseTime.Add(time.Second), Message: "msg2", Source: "web", Name: "myapp2", Unit: "unit2"},
		{Date: baseTime.Add(3 * time.Second), Message: "msg4", Source: "web", Name: "myapp2", Unit: "unit4"},
	})
}

func (s *S) TestAddLogsHandlerConcurrent(c *check.C) {
	s.mockService.Team.OnList = func() ([]authTypes.Team, error) {
		return []authTypes.Team{{Name: s.team.Name}}, nil
	}
	a1 := app.App{Name: "myapp1", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a1, s.user)
	c.Assert(err, check.IsNil)
	a2 := app.App{Name: "myapp2", Platform: "zend", TeamOwner: s.team.Name}
	err = app.CreateApp(context.TODO(), &a2, s.user)
	c.Assert(err, check.IsNil)
	baseTime, err := time.Parse(time.RFC3339, "2015-06-16T15:00:00.000Z")
	c.Assert(err, check.IsNil)
	baseTime = baseTime.Local()
	bodyPart := `
	{"date": "2015-06-16T15:00:00.000Z", "message": "msg1", "source": "web", "name": "myapp1", "unit": "unit1"}
	{"date": "2015-06-16T15:00:01.000Z", "message": "msg2", "source": "web", "name": "myapp2", "unit": "unit2"}
	`
	srv := httptest.NewServer(s.testServer)
	defer srv.Close()
	testServerURL, err := url.Parse(srv.URL)
	c.Assert(err, check.IsNil)
	wsURL := fmt.Sprintf("ws://%s/logs", testServerURL.Host)
	wg := sync.WaitGroup{}
	nConcurrency := 100
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			config, wsErr := websocket.NewConfig(wsURL, "ws://localhost/")
			c.Assert(wsErr, check.IsNil)
			config.Header.Set("Authorization", "bearer "+s.token.GetValue())
			wsConn, wsErr := websocket.DialConfig(config)
			c.Assert(wsErr, check.IsNil)
			defer wsConn.Close()
			_, wsErr = wsConn.Write([]byte(bodyPart))
			c.Assert(wsErr, check.IsNil)
		}()
	}
	wg.Wait()
	timeout := time.After(5 * time.Second)
loop:
	for {
		var logs1 []appTypes.Applog
		logs1, err = a1.LastLogs(context.TODO(), servicemanager.LogService, appTypes.ListLogArgs{
			Limit: nConcurrency,
		})
		c.Assert(err, check.IsNil)
		if len(logs1) == nConcurrency {
			break
		}
		select {
		case <-timeout:
			c.Fatal("timeout waiting for logs")
			break loop
		default:
		}
	}
	logs, err := a1.LastLogs(context.TODO(), servicemanager.LogService, appTypes.ListLogArgs{
		Limit: 1,
	})
	c.Assert(err, check.IsNil)
	compareLogs(c, logs, []appTypes.Applog{
		{Date: baseTime, Message: "msg1", Source: "web", Name: "myapp1", Unit: "unit1"},
	})
}

func (s *S) BenchmarkScanLogs(c *check.C) {
	c.StopTimer()
	var apps []app.App
	for i := 0; i < 100; i++ {
		a := app.App{Name: fmt.Sprintf("myapp-%d", i), Platform: "zend", TeamOwner: s.team.Name}
		apps = append(apps, a)
		err := app.CreateApp(context.TODO(), &a, s.user)
		c.Assert(err, check.IsNil)
	}
	baseMsg := `{"date": "2015-06-16T15:00:00.000Z", "message": "msg-%d", "source": "web", "appname": "%s", "unit": "unit1"}` + "\n"
	for i := range apps {
		// Remove overhead for first message from app from benchmark.
		err := scanLogs(strings.NewReader(fmt.Sprintf(baseMsg, 0, apps[i].Name)))
		c.Assert(err, check.IsNil)
	}
	c.StartTimer()
	r, w := io.Pipe()
	done := make(chan struct{})
	go func() {
		defer close(done)
		err := scanLogs(r)
		if err != nil {
			c.Fatal(err)
		}
	}()
	for i := 0; i < c.N; i++ {
		msg := fmt.Sprintf(baseMsg, i, apps[i%len(apps)].Name)
		_, err := w.Write([]byte(msg))
		if err != nil {
			c.Fatal(err)
		}
	}
	w.Close()
	<-done
	c.StopTimer()
	servicemanager.LogService.(shutdown.Shutdownable).Shutdown(context.Background())
	var err error
	servicemanager.LogService, err = applog.AppLogService()
	c.Assert(err, check.IsNil)
}
