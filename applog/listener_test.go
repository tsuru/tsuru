// Copyright 2019 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package applog

import (
	"sync"
	"time"

	appTypes "github.com/tsuru/tsuru/types/app"
	"gopkg.in/check.v1"
)

func (s *S) TestNewLogListener(c *check.C) {
	err := createAppLogCollection("myapp")
	c.Assert(err, check.IsNil)
	svc, err := StorageAppLogService()
	c.Assert(err, check.IsNil)
	l, err := newLogListener(svc, "myapp", appTypes.Applog{})
	c.Assert(err, check.IsNil)
	defer l.Close()
	c.Assert(l.quit, check.NotNil)
	c.Assert(l.c, check.NotNil)
	err = insertLogs("myapp", []interface{}{appTypes.Applog{Message: "123"}})
	c.Assert(err, check.IsNil)
	logMsg := <-l.c
	c.Assert(logMsg.Message, check.Equals, "123")
	err = insertLogs("myapp", []interface{}{appTypes.Applog{Message: "456"}})
	c.Assert(err, check.IsNil)
	logMsg = <-l.c
	c.Assert(logMsg.Message, check.Equals, "456")
}

func (s *S) TestNewLogListenerFiltered(c *check.C) {
	err := createAppLogCollection("myapp")
	c.Assert(err, check.IsNil)
	svc, err := StorageAppLogService()
	c.Assert(err, check.IsNil)
	l, err := newLogListener(svc, "myapp", appTypes.Applog{Source: "web", Unit: "u1"})
	c.Assert(err, check.IsNil)
	defer l.Close()
	c.Assert(l.quit, check.NotNil)
	c.Assert(l.c, check.NotNil)
	err = insertLogs("myapp", []interface{}{
		appTypes.Applog{Message: "1", Source: "web", Unit: "u1"},
		appTypes.Applog{Message: "2", Source: "worker", Unit: "u1"},
		appTypes.Applog{Message: "3", Source: "web", Unit: "u1"},
		appTypes.Applog{Message: "4", Source: "web", Unit: "u2"},
		appTypes.Applog{Message: "5", Source: "web", Unit: "u1"},
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
	err := createAppLogCollection("myapp")
	c.Assert(err, check.IsNil)
	svc, err := StorageAppLogService()
	c.Assert(err, check.IsNil)
	l, err := newLogListener(svc, "myapp", appTypes.Applog{})
	c.Assert(err, check.IsNil)
	c.Assert(l.quit, check.NotNil)
	c.Assert(l.c, check.NotNil)
	l.Close()
	_, ok := <-l.c
	c.Assert(ok, check.Equals, false)
}

func (s *S) TestLogListenerClose(c *check.C) {
	err := createAppLogCollection("myapp")
	c.Assert(err, check.IsNil)
	svc, err := StorageAppLogService()
	c.Assert(err, check.IsNil)
	l, err := newLogListener(svc, "myapp", appTypes.Applog{})
	c.Assert(err, check.IsNil)
	l.Close()
	_, ok := <-l.c
	c.Assert(ok, check.Equals, false)
}

func (s *S) TestLogListenerDoubleClose(c *check.C) {
	defer func() {
		c.Assert(recover(), check.IsNil)
	}()
	err := createAppLogCollection("yourapp")
	c.Assert(err, check.IsNil)
	svc, err := StorageAppLogService()
	c.Assert(err, check.IsNil)
	l, err := newLogListener(svc, "yourapp", appTypes.Applog{})
	c.Assert(err, check.IsNil)
	l.Close()
	l.Close()
}

func (s *S) TestNotify(c *check.C) {
	var logs struct {
		l []appTypes.Applog
		sync.Mutex
	}
	err := createAppLogCollection("fade")
	c.Assert(err, check.IsNil)
	svc, err := StorageAppLogService()
	c.Assert(err, check.IsNil)
	l, err := newLogListener(svc, "fade", appTypes.Applog{})
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
		appTypes.Applog{Date: t, Message: "Something went wrong. Check it out:", Source: "tsuru", Unit: "some"},
		appTypes.Applog{Date: t, Message: "This program has performed an illegal operation.", Source: "tsuru", Unit: "some"},
	}
	insertLogs("fade", ms)
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
	compareLogs(c, logs.l, []appTypes.Applog{ms[0].(appTypes.Applog), ms[1].(appTypes.Applog)})
}

func (s *S) TestNotifyFiltered(c *check.C) {
	var logs struct {
		l []appTypes.Applog
		sync.Mutex
	}
	err := createAppLogCollection("fade")
	c.Assert(err, check.IsNil)
	svc, err := StorageAppLogService()
	c.Assert(err, check.IsNil)
	l, err := newLogListener(svc, "fade", appTypes.Applog{Source: "tsuru", Unit: "unit1"})
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
		appTypes.Applog{Date: t, Message: "Something went wrong. Check it out:", Source: "tsuru", Unit: "unit1"},
		appTypes.Applog{Date: t, Message: "This program has performed an illegal operation.", Source: "other", Unit: "unit1"},
		appTypes.Applog{Date: t, Message: "Last one.", Source: "tsuru", Unit: "unit2"},
	}
	insertLogs("fade", ms)
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
	compareLogs(c, logs.l, []appTypes.Applog{
		{Date: t, Message: "Something went wrong. Check it out:", Source: "tsuru", Unit: "unit1"},
	})
}

func (s *S) TestNotifySendOnClosedChannel(c *check.C) {
	defer func() {
		c.Assert(recover(), check.IsNil)
	}()
	err := createAppLogCollection("fade")
	c.Assert(err, check.IsNil)
	svc, err := StorageAppLogService()
	c.Assert(err, check.IsNil)
	l, err := newLogListener(svc, "fade", appTypes.Applog{})
	c.Assert(err, check.IsNil)
	l.Close()
	ms := []interface{}{
		appTypes.Applog{Date: time.Now(), Message: "Something went wrong. Check it out:", Source: "tsuru"},
	}
	insertLogs("fade", ms)
}
