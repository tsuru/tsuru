// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storagetest

import (
	"strconv"
	"sync"
	"time"

	"github.com/tsuru/tsuru/types/app"
	check "gopkg.in/check.v1"
)

type AppLogSuite struct {
	SuiteHooks
	AppLogStorage app.AppLogStorage
}

func (s *AppLogSuite) Test_InsertApp(c *check.C) {
	err := s.AppLogStorage.InsertApp("myapp", []*app.Applog{
		{Message: "x", Source: "tsuru", AppName: "myapp", Unit: "outermachine"},
		{Message: "y", Source: "tsuru", AppName: "myapp", Unit: "outermachine"},
	}...)
	c.Assert(err, check.IsNil)
	logs, err := s.AppLogStorage.List(app.ListLogArgs{AppName: "myapp"})
	c.Assert(err, check.IsNil)
	c.Assert(logs, check.HasLen, 2)
	c.Assert(logs[0].Message, check.Equals, "x")
	c.Assert(logs[0].Source, check.Equals, "tsuru")
	c.Assert(logs[0].AppName, check.Equals, "myapp")
	c.Assert(logs[0].Unit, check.Equals, "outermachine")
	c.Assert(logs[1].Message, check.Equals, "y")
}

func addLog(c *check.C, storage app.AppLogStorage, appName, msg, source, unit string) {
	err := storage.InsertApp(appName, &app.Applog{
		Message: msg, Source: source, AppName: appName, Unit: unit,
	})
	c.Assert(err, check.IsNil)
}

func (s *AppLogSuite) TestWatch(c *check.C) {
	l, err := s.AppLogStorage.Watch("myapp", "", "")
	c.Assert(err, check.IsNil)
	defer l.Close()
	c.Assert(l.Chan(), check.NotNil)
	addLog(c, s.AppLogStorage, "myapp", "123", "", "")
	logMsg := <-l.Chan()
	c.Assert(logMsg.Message, check.Equals, "123")
	addLog(c, s.AppLogStorage, "myapp", "456", "", "")
	logMsg = <-l.Chan()
	c.Assert(logMsg.Message, check.Equals, "456")
}

func (s *AppLogSuite) TestWatchFiltered(c *check.C) {
	l, err := s.AppLogStorage.Watch("myapp", "web", "u1")
	c.Assert(err, check.IsNil)
	defer l.Close()
	c.Assert(l.Chan(), check.NotNil)
	logs := []app.Applog{
		{Message: "1", Source: "web", Unit: "u1"},
		{Message: "2", Source: "worker", Unit: "u1"},
		{Message: "3", Source: "web", Unit: "u1"},
		{Message: "4", Source: "web", Unit: "u2"},
		{Message: "5", Source: "web", Unit: "u1"},
	}
	for _, log := range logs {
		addLog(c, s.AppLogStorage, "myapp", log.Message, log.Source, log.Unit)
	}
	logMsg := <-l.Chan()
	c.Assert(logMsg.Message, check.Equals, "1")
	logMsg = <-l.Chan()
	c.Assert(logMsg.Message, check.Equals, "3")
	logMsg = <-l.Chan()
	c.Assert(logMsg.Message, check.Equals, "5")
}

func (s *AppLogSuite) TestWatchClosingChannel(c *check.C) {
	l, err := s.AppLogStorage.Watch("myapp", "", "")
	c.Assert(err, check.IsNil)
	c.Assert(l.Chan(), check.NotNil)
	l.Close()
	_, ok := <-l.Chan()
	c.Assert(ok, check.Equals, false)
}

func (s *AppLogSuite) TestWatchClose(c *check.C) {
	l, err := s.AppLogStorage.Watch("myapp", "", "")
	c.Assert(err, check.IsNil)
	l.Close()
	_, ok := <-l.Chan()
	c.Assert(ok, check.Equals, false)
}

func (s *AppLogSuite) TestWatchDoubleClose(c *check.C) {
	defer func() {
		c.Assert(recover(), check.IsNil)
	}()
	l, err := s.AppLogStorage.Watch("yourapp", "", "")
	c.Assert(err, check.IsNil)
	l.Close()
	l.Close()
}

func (s *AppLogSuite) TestWatchNotify(c *check.C) {
	var logs struct {
		l []app.Applog
		sync.Mutex
	}
	l, err := s.AppLogStorage.Watch("fade", "", "")
	c.Assert(err, check.IsNil)
	defer l.Close()
	go func() {
		for log := range l.Chan() {
			logs.Lock()
			logs.l = append(logs.l, log)
			logs.Unlock()
		}
	}()
	ms := []app.Applog{
		{Message: "Something went wrong. Check it out:", Source: "tsuru", Unit: "some", AppName: "fade"},
		{Message: "This program has performed an illegal operation.", Source: "tsuru", Unit: "some", AppName: "fade"},
	}
	for _, log := range ms {
		addLog(c, s.AppLogStorage, "fade", log.Message, log.Source, log.Unit)
	}
	done := make(chan bool, 1)
	q := make(chan bool)
	go func(quit chan bool) {
		ticker := time.NewTicker(1e3)
		defer ticker.Stop()
		for range ticker.C {
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
	compareLogsNoDate(c, logs.l, ms)
}

func (s *AppLogSuite) TestWatchNotifyFiltered(c *check.C) {
	var logs struct {
		l []app.Applog
		sync.Mutex
	}
	l, err := s.AppLogStorage.Watch("fade", "tsuru", "unit1")
	c.Assert(err, check.IsNil)
	defer l.Close()
	go func() {
		for log := range l.Chan() {
			logs.Lock()
			logs.l = append(logs.l, log)
			logs.Unlock()
		}
	}()
	ms := []app.Applog{
		{Message: "Something went wrong. Check it out:", Source: "tsuru", Unit: "unit1"},
		{Message: "This program has performed an illegal operation.", Source: "other", Unit: "unit1"},
		{Message: "Last one.", Source: "tsuru", Unit: "unit2"},
	}
	for _, log := range ms {
		addLog(c, s.AppLogStorage, "fade", log.Message, log.Source, log.Unit)
	}
	done := make(chan bool, 1)
	q := make(chan bool)
	go func(quit chan bool) {
		ticker := time.NewTicker(1e3)
		defer ticker.Stop()
		for range ticker.C {
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
	compareLogsNoDate(c, logs.l, []app.Applog{
		{Message: "Something went wrong. Check it out:", Source: "tsuru", Unit: "unit1", AppName: "fade"},
	})
}

func (s *AppLogSuite) TestWatchNotifySendOnClosedChannel(c *check.C) {
	defer func() {
		c.Assert(recover(), check.IsNil)
	}()
	l, err := s.AppLogStorage.Watch("fade", "", "")
	c.Assert(err, check.IsNil)
	l.Close()
	addLog(c, s.AppLogStorage, "fade", "Something went wrong. Check it out:", "tsuru", "")
}

func (s *AppLogSuite) TestLogStorageList(c *check.C) {
	for i := 0; i < 15; i++ {
		addLog(c, s.AppLogStorage, "myapp", strconv.Itoa(i), "tsuru", "rdaneel")
	}
	addLog(c, s.AppLogStorage, "myapp", "myapp log from circus", "circus", "rdaneel")
	logs, err := s.AppLogStorage.List(app.ListLogArgs{Limit: 10, AppName: "myapp", Source: "tsuru"})
	c.Assert(err, check.IsNil)
	c.Assert(logs, check.HasLen, 10)
	for i := 5; i < 15; i++ {
		c.Check(logs[i-5].Message, check.Equals, strconv.Itoa(i))
		c.Check(logs[i-5].Source, check.Equals, "tsuru")
	}
}

func (s *AppLogSuite) TestLogStorageListUnitFilter(c *check.C) {
	for i := 0; i < 15; i++ {
		addLog(c, s.AppLogStorage, "app3", strconv.Itoa(i), "tsuru", "rdaneel")
	}
	addLog(c, s.AppLogStorage, "app3", "app3 log from circus", "circus", "rdaneel")
	addLog(c, s.AppLogStorage, "app3", "app3 log from tsuru", "tsuru", "seldon")
	logs, err := s.AppLogStorage.List(app.ListLogArgs{Limit: 10, AppName: "app3", Source: "tsuru", Unit: "rdaneel"})
	c.Assert(err, check.IsNil)
	c.Assert(logs, check.HasLen, 10)
	for i := 5; i < 15; i++ {
		c.Check(logs[i-5].Message, check.Equals, strconv.Itoa(i))
		c.Check(logs[i-5].Source, check.Equals, "tsuru")
	}
}

func (s *AppLogSuite) TestLogStorageListEmpty(c *check.C) {
	logs, err := s.AppLogStorage.List(app.ListLogArgs{Limit: 10, AppName: "myapp", Source: "tsuru"})
	c.Assert(err, check.IsNil)
	c.Assert(logs, check.DeepEquals, []app.Applog{})
}

func compareLogsNoDate(c *check.C, logs1 []app.Applog, logs2 []app.Applog) {
	for i := range logs1 {
		logs1[i].MongoID = ""
		logs1[i].Date = time.Time{}
	}
	for i := range logs2 {
		logs2[i].MongoID = ""
		logs2[i].Date = time.Time{}
	}
	c.Assert(logs1, check.DeepEquals, logs2)
}
