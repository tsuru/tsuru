// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	dto "github.com/prometheus/client_model/go"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db"
	"gopkg.in/check.v1"
)

func insertLogs(appName string, logs []interface{}) error {
	conn, err := db.LogConn()
	if err != nil {
		return err
	}
	defer conn.Close()
	return conn.Logs(appName).Insert(logs...)
}

func compareLogs(c *check.C, logs1 []Applog, logs2 []Applog) {
	for i := range logs1 {
		logs1[i].MongoID = ""
		logs1[i].Date = logs1[i].Date.UTC()
	}
	for i := range logs2 {
		logs2[i].MongoID = ""
		logs2[i].Date = logs2[i].Date.UTC()
	}
	c.Assert(logs1, check.DeepEquals, logs2)
}

func (s *S) TestNewLogListener(c *check.C) {
	app := App{Name: "myapp"}
	l, err := NewLogListener(&app, Applog{})
	c.Assert(err, check.IsNil)
	defer l.Close()
	c.Assert(l.quit, check.NotNil)
	c.Assert(l.c, check.NotNil)
	err = insertLogs("myapp", []interface{}{Applog{Message: "123"}})
	c.Assert(err, check.IsNil)
	logMsg := <-l.c
	c.Assert(logMsg.Message, check.Equals, "123")
	err = insertLogs("myapp", []interface{}{Applog{Message: "456"}})
	c.Assert(err, check.IsNil)
	logMsg = <-l.c
	c.Assert(logMsg.Message, check.Equals, "456")
}

func (s *S) TestNewLogListenerFiltered(c *check.C) {
	app := App{Name: "myapp"}
	l, err := NewLogListener(&app, Applog{Source: "web", Unit: "u1"})
	c.Assert(err, check.IsNil)
	defer l.Close()
	c.Assert(l.quit, check.NotNil)
	c.Assert(l.c, check.NotNil)
	err = insertLogs("myapp", []interface{}{
		Applog{Message: "1", Source: "web", Unit: "u1"},
		Applog{Message: "2", Source: "worker", Unit: "u1"},
		Applog{Message: "3", Source: "web", Unit: "u1"},
		Applog{Message: "4", Source: "web", Unit: "u2"},
		Applog{Message: "5", Source: "web", Unit: "u1"},
	})
	c.Assert(err, check.IsNil)
	logMsg := <-l.c
	c.Assert(logMsg.Message, check.Equals, "1")
	logMsg = <-l.c
	c.Assert(logMsg.Message, check.Equals, "3")
	logMsg = <-l.c
	c.Assert(logMsg.Message, check.Equals, "5")
}

func (s *S) TestNewLogListenerClosingChannel(c *check.C) {
	app := App{Name: "myapp"}
	l, err := NewLogListener(&app, Applog{})
	c.Assert(err, check.IsNil)
	c.Assert(l.quit, check.NotNil)
	c.Assert(l.c, check.NotNil)
	l.Close()
	_, ok := <-l.c
	c.Assert(ok, check.Equals, false)
}

func (s *S) TestLogListenerClose(c *check.C) {
	app := App{Name: "myapp"}
	l, err := NewLogListener(&app, Applog{})
	c.Assert(err, check.IsNil)
	l.Close()
	_, ok := <-l.c
	c.Assert(ok, check.Equals, false)
}

func (s *S) TestLogListenerDoubleClose(c *check.C) {
	defer func() {
		c.Assert(recover(), check.IsNil)
	}()
	app := App{Name: "yourapp"}
	l, err := NewLogListener(&app, Applog{})
	c.Assert(err, check.IsNil)
	l.Close()
	l.Close()
}

func (s *S) TestNotify(c *check.C) {
	var logs struct {
		l []Applog
		sync.Mutex
	}
	app := App{Name: "fade"}
	l, err := NewLogListener(&app, Applog{})
	c.Assert(err, check.IsNil)
	defer l.Close()
	go func() {
		for log := range l.c {
			logs.Lock()
			logs.l = append(logs.l, log)
			logs.Unlock()
		}
	}()
	t := time.Date(2014, 7, 10, 15, 0, 0, 0, time.UTC)
	ms := []interface{}{
		Applog{Date: t, Message: "Something went wrong. Check it out:", Source: "tsuru", Unit: "some"},
		Applog{Date: t, Message: "This program has performed an illegal operation.", Source: "tsuru", Unit: "some"},
	}
	insertLogs(app.Name, ms)
	done := make(chan bool, 1)
	q := make(chan bool)
	go func(quit chan bool) {
		for range time.Tick(1e3) {
			select {
			case <-quit:
				return
			default:
			}
			logs.Lock()
			if len(logs.l) == 2 {
				logs.Unlock()
				done <- true
				return
			}
			logs.Unlock()
		}
	}(q)
	select {
	case <-done:
	case <-time.After(2e9):
		defer close(q)
		c.Fatal("Timed out.")
	}
	logs.Lock()
	defer logs.Unlock()
	compareLogs(c, logs.l, []Applog{ms[0].(Applog), ms[1].(Applog)})
}

func (s *S) TestNotifyFiltered(c *check.C) {
	var logs struct {
		l []Applog
		sync.Mutex
	}
	app := App{Name: "fade"}
	l, err := NewLogListener(&app, Applog{Source: "tsuru", Unit: "unit1"})
	c.Assert(err, check.IsNil)
	defer l.Close()
	go func() {
		for log := range l.c {
			logs.Lock()
			logs.l = append(logs.l, log)
			logs.Unlock()
		}
	}()
	t := time.Date(2014, 7, 10, 15, 0, 0, 0, time.UTC)
	ms := []interface{}{
		Applog{Date: t, Message: "Something went wrong. Check it out:", Source: "tsuru", Unit: "unit1"},
		Applog{Date: t, Message: "This program has performed an illegal operation.", Source: "other", Unit: "unit1"},
		Applog{Date: t, Message: "Last one.", Source: "tsuru", Unit: "unit2"},
	}
	insertLogs(app.Name, ms)
	done := make(chan bool, 1)
	q := make(chan bool)
	go func(quit chan bool) {
		for range time.Tick(1e3) {
			select {
			case <-quit:
				return
			default:
			}
			logs.Lock()
			if len(logs.l) == 1 {
				logs.Unlock()
				done <- true
				return
			}
			logs.Unlock()
		}
	}(q)
	select {
	case <-done:
	case <-time.After(2e9):
		defer close(q)
		c.Fatal("Timed out.")
	}
	logs.Lock()
	defer logs.Unlock()
	compareLogs(c, logs.l, []Applog{
		{Date: t, Message: "Something went wrong. Check it out:", Source: "tsuru", Unit: "unit1"},
	})
}

func (s *S) TestNotifySendOnClosedChannel(c *check.C) {
	defer func() {
		c.Assert(recover(), check.IsNil)
	}()
	app := App{Name: "fade"}
	l, err := NewLogListener(&app, Applog{})
	c.Assert(err, check.IsNil)
	l.Close()
	ms := []interface{}{
		Applog{Date: time.Now(), Message: "Something went wrong. Check it out:", Source: "tsuru"},
	}
	insertLogs(app.Name, ms)
}

func (s *S) TestLogDispatcherSend(c *check.C) {
	logsInQueue.Set(0)
	app := App{Name: "myapp1", Platform: "zend", TeamOwner: s.team.Name}
	err := CreateApp(&app, s.user)
	c.Assert(err, check.IsNil)
	listener, err := NewLogListener(&app, Applog{})
	c.Assert(err, check.IsNil)
	defer listener.Close()
	dispatcher := NewlogDispatcher(2000000)
	baseTime, err := time.Parse(time.RFC3339, "2015-06-16T15:00:00.000Z")
	c.Assert(err, check.IsNil)
	baseTime = baseTime.Local()
	logMsg := Applog{
		Date: baseTime, Message: "msg1", Source: "web", AppName: "myapp1", Unit: "unit1",
	}
	dispatcher.Send(&logMsg)
	dispatcher.Shutdown(context.Background())
	logs, err := app.LastLogs(1, Applog{})
	c.Assert(err, check.IsNil)
	compareLogs(c, logs, []Applog{logMsg})
	err = dispatcher.Send(&logMsg)
	c.Assert(err, check.ErrorMatches, `log dispatcher is shutting down`)
	var dtoMetric dto.Metric
	logsInQueue.Write(&dtoMetric)
	c.Assert(dtoMetric.Gauge.GetValue(), check.Equals, 0.0)
	ch := listener.ListenChan()
	recvMsg := <-ch
	recvMsg.Date = baseTime
	compareLogs(c, []Applog{recvMsg}, []Applog{logMsg})
}

func (s *S) TestLogDispatcherSendConcurrent(c *check.C) {
	app1 := App{Name: "myapp1", Platform: "zend", TeamOwner: s.team.Name}
	err := CreateApp(&app1, s.user)
	c.Assert(err, check.IsNil)
	app2 := App{Name: "myapp2", Platform: "zend", TeamOwner: s.team.Name}
	err = CreateApp(&app2, s.user)
	c.Assert(err, check.IsNil)
	dispatcher := NewlogDispatcher(2000000)
	baseTime, err := time.Parse(time.RFC3339, "2015-06-16T15:00:00.000Z")
	c.Assert(err, check.IsNil)
	baseTime = baseTime.Local()
	logMsg := []Applog{
		{Date: baseTime, Message: "msg1", Source: "web", AppName: "myapp1", Unit: "unit1"},
		{Date: baseTime, Message: "msg2", Source: "web", AppName: "myapp2", Unit: "unit1"},
	}
	nConcurrent := 100
	wg := sync.WaitGroup{}
	for i := 0; i < nConcurrent; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			dispatcher.Send(&logMsg[i%len(logMsg)])
		}(i)
	}
	wg.Wait()
	dispatcher.Shutdown(context.Background())
	logs, err := app1.LastLogs(nConcurrent/2, Applog{})
	c.Assert(err, check.IsNil)
	c.Assert(logs, check.HasLen, nConcurrent/2)
	logs, err = app2.LastLogs(nConcurrent/2, Applog{})
	c.Assert(err, check.IsNil)
	c.Assert(logs, check.HasLen, nConcurrent/2)
}

func (s *S) TestLogDispatcherShutdownConcurrent(c *check.C) {
	logsInQueue.Set(0)
	app1 := App{Name: "myapp1", Platform: "zend", TeamOwner: s.team.Name}
	err := CreateApp(&app1, s.user)
	c.Assert(err, check.IsNil)
	app2 := App{Name: "myapp2", Platform: "zend", TeamOwner: s.team.Name}
	err = CreateApp(&app2, s.user)
	c.Assert(err, check.IsNil)
	dispatcher := NewlogDispatcher(2000000)
	baseTime, err := time.Parse(time.RFC3339, "2015-06-16T15:00:00.000Z")
	c.Assert(err, check.IsNil)
	baseTime = baseTime.Local()
	logMsg := []Applog{
		{Date: baseTime, Message: "msg1", Source: "web", AppName: "myapp1", Unit: "unit1"},
		{Date: baseTime, Message: "msg2", Source: "web", AppName: "myapp2", Unit: "unit1"},
	}
	nConcurrent := 100
	for i := 0; i < nConcurrent; i++ {
		go func(i int) {
			dispatcher.Send(&logMsg[i%len(logMsg)])
		}(i)
	}
	dispatcher.Shutdown(context.Background())
	var dtoMetric dto.Metric
	logsInQueue.Write(&dtoMetric)
	c.Assert(dtoMetric.Gauge.GetValue(), check.Equals, 0.0)
}

func (s *S) TestLogDispatcherSendDBFailure(c *check.C) {
	app := App{Name: "myapp1", Platform: "zend", TeamOwner: s.team.Name}
	err := CreateApp(&app, s.user)
	c.Assert(err, check.IsNil)
	dispatcher := NewlogDispatcher(2000000)
	baseTime, err := time.Parse(time.RFC3339, "2015-06-16T15:00:00.000Z")
	c.Assert(err, check.IsNil)
	baseTime = baseTime.Local()
	logMsg := Applog{
		Date: baseTime, Message: "msg1", Source: "web", AppName: "myapp1", Unit: "unit1",
	}
	oldDbURL, err := config.Get("database:url")
	c.Assert(err, check.IsNil)
	var count int32
	dbOk := make(chan bool)
	config.Set("database:url", func() interface{} {
		val := atomic.AddInt32(&count, 1)
		if val == 1 {
			close(dbOk)
			return "localhost:44556"
		}
		return oldDbURL
	})
	defer config.Set("database:url", oldDbURL)
	for i := 0; i < 10; i++ {
		dispatcher.Send(&logMsg)
	}
	<-dbOk
	timeout := time.After(10 * time.Second)
loop:
	for {
		logs, logsErr := app.LastLogs(10, Applog{})
		c.Assert(logsErr, check.IsNil)
		if len(logs) == 10 {
			break
		}
		select {
		case <-timeout:
			c.Fatalf("timeout waiting for all logs, last count: %d", len(logs))
			break loop
		default:
			time.Sleep(100 * time.Millisecond)
		}
	}
	dispatcher.Shutdown(context.Background())
}
