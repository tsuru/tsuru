// Copyright 2019 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package applog

import (
	"strconv"
	"sync"
	"time"

	appTypes "github.com/tsuru/tsuru/types/app"
	"gopkg.in/check.v1"
)

var (
	_ = check.Suite(&ServiceSuite{svcFunc: storageAppLogService})
	_ = check.Suite(&ServiceSuite{svcFunc: memoryAppLogService})
)

func (s *ServiceSuite) Test_LogService_Add(c *check.C) {
	err := s.svc.Add("myapp", "last log msg", "tsuru", "outermachine", 0)
	c.Assert(err, check.IsNil)
	logs, err := s.svc.List(appTypes.ListLogArgs{AppName: "myapp"})
	c.Assert(err, check.IsNil)
	c.Assert(logs, check.HasLen, 1)
	c.Assert(logs[0].Message, check.Equals, "last log msg")
	c.Assert(logs[0].Source, check.Equals, "tsuru")
	c.Assert(logs[0].AppName, check.Equals, "myapp")
	c.Assert(logs[0].Unit, check.Equals, "outermachine")
}

func (s *ServiceSuite) Test_LogService_AddShouldAddOneRecordByLine(c *check.C) {
	err := s.svc.Add("myapp", "last log msg\nfirst log", "tsuru", "outermachine", 0)
	c.Assert(err, check.IsNil)
	logs, err := s.svc.List(appTypes.ListLogArgs{AppName: "myapp"})
	c.Assert(err, check.IsNil)
	c.Assert(logs, check.HasLen, 2)
	c.Assert(logs[0].Message, check.Equals, "last log msg")
	c.Assert(logs[1].Message, check.Equals, "first log")
}

func (s *ServiceSuite) Test_LogService_AddShouldNotLogBlankLines(c *check.C) {
	err := s.svc.Add("ich", "some message", "tsuru", "machine", 0)
	c.Assert(err, check.IsNil)
	err = s.svc.Add("ich", "", "", "", 0)
	c.Assert(err, check.IsNil)
	logs, err := s.svc.List(appTypes.ListLogArgs{AppName: "ich"})
	c.Assert(err, check.IsNil)
	c.Assert(logs, check.HasLen, 1)
}

func (s *ServiceSuite) Test_LogService_AddWithListeners(c *check.C) {
	var logs struct {
		l []appTypes.Applog
		sync.Mutex
	}
	l, err := s.svc.Watch("myapp", "", "", nil)
	c.Assert(err, check.IsNil)
	defer l.Close()
	go func() {
		for log := range l.Chan() {
			logs.Lock()
			logs.l = append(logs.l, log)
			logs.Unlock()
		}
	}()
	err = s.svc.Add("myapp", "last log msg", "tsuru", "machine", 0)
	c.Assert(err, check.IsNil)
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
	c.Assert(logs.l, check.HasLen, 1)
	log := logs.l[0]
	logs.Unlock()
	c.Assert(log.Message, check.Equals, "last log msg")
	c.Assert(log.Source, check.Equals, "tsuru")
	c.Assert(log.Unit, check.Equals, "machine")
}

func (s *ServiceSuite) Test_LogService_List(c *check.C) {
	for i := 0; i < 15; i++ {
		s.svc.Add("myapp", strconv.Itoa(i), "tsuru", "rdaneel", 0)
		time.Sleep(1e6) // let the time flow
	}
	s.svc.Add("myapp", "myapp log from circus", "circus", "rdaneel", 0)
	logs, err := s.svc.List(appTypes.ListLogArgs{Limit: 10, AppName: "myapp", Source: "tsuru"})
	c.Assert(err, check.IsNil)
	c.Assert(logs, check.HasLen, 10)
	for i := 5; i < 15; i++ {
		c.Check(logs[i-5].Message, check.Equals, strconv.Itoa(i))
		c.Check(logs[i-5].Source, check.Equals, "tsuru")
	}
}

func (s *ServiceSuite) Test_LogService_ListNegativeLimit(c *check.C) {
	for i := 0; i < 15; i++ {
		s.svc.Add("myapp", strconv.Itoa(i), "tsuru", "rdaneel", 0)
	}
	logs, err := s.svc.List(appTypes.ListLogArgs{Limit: -1, AppName: "myapp"})
	c.Assert(err, check.IsNil)
	c.Assert(logs, check.HasLen, 0)
}

func (s *ServiceSuite) Test_LogService_ListZeroLimit(c *check.C) {
	for i := 0; i < 15; i++ {
		s.svc.Add("myapp", strconv.Itoa(i), "tsuru", "rdaneel", 0)
	}
	logs, err := s.svc.List(appTypes.ListLogArgs{Limit: 0, AppName: "myapp"})
	c.Assert(err, check.IsNil)
	c.Assert(logs, check.HasLen, 15)
}

func (s *ServiceSuite) Test_LogService_ListAll(c *check.C) {
	for i := 0; i < 15; i++ {
		s.svc.Add("myapp", strconv.Itoa(i), "tsuru", "rdaneel", 0)
		time.Sleep(1e6) // let the time flow
	}
	s.svc.Add("myapp", "myapp log from circus", "circus", "rdaneel", 0)
	logs, err := s.svc.List(appTypes.ListLogArgs{Limit: 1000, AppName: "myapp", Source: "tsuru"})
	c.Assert(err, check.IsNil)
	c.Assert(logs, check.HasLen, 15)
	for i := 0; i < 15; i++ {
		c.Check(logs[i].Message, check.Equals, strconv.Itoa(i))
		c.Check(logs[i].Source, check.Equals, "tsuru")
	}
}

func (s *ServiceSuite) Test_LogService_ListUnitFilter(c *check.C) {
	for i := 0; i < 15; i++ {
		s.svc.Add("app3", strconv.Itoa(i), "tsuru", "rdaneel", 0)
		time.Sleep(1e6) // let the time flow
	}
	s.svc.Add("app3", "app3 log from circus", "circus", "rdaneel", 0)
	s.svc.Add("app3", "app3 log from tsuru", "tsuru", "seldon", 0)
	logs, err := s.svc.List(appTypes.ListLogArgs{Limit: 10, AppName: "app3", Source: "tsuru", Unit: "rdaneel"})
	c.Assert(err, check.IsNil)
	c.Assert(logs, check.HasLen, 10)
	for i := 5; i < 15; i++ {
		c.Check(logs[i-5].Message, check.Equals, strconv.Itoa(i))
		c.Check(logs[i-5].Source, check.Equals, "tsuru")
	}
}

func (s *ServiceSuite) Test_LogService_ListFilterInvert(c *check.C) {
	for i := 0; i < 15; i++ {
		s.svc.Add("app3", strconv.Itoa(i), "tsuru", "rdaneel", 0)
		time.Sleep(1e6) // let the time flow
	}
	s.svc.Add("app3", "app3 log from circus", "circus", "rdaneel", 0)
	s.svc.Add("app3", "app3 log from tsuru", "tsuru", "seldon", 0)
	logs, err := s.svc.List(appTypes.ListLogArgs{Limit: 10, AppName: "app3", Source: "tsuru", InvertFilters: true})
	c.Assert(err, check.IsNil)
	c.Assert(logs, check.HasLen, 1)
	c.Check(logs[0].Message, check.Equals, "app3 log from circus")
	c.Check(logs[0].Source, check.Equals, "circus")
}

func (s *ServiceSuite) Test_LogService_ListEmpty(c *check.C) {
	logs, err := s.svc.List(appTypes.ListLogArgs{Limit: 10, AppName: "myapp", Source: "tsuru"})
	c.Assert(err, check.IsNil)
	c.Assert(logs, check.DeepEquals, []appTypes.Applog{})
}

func addLog(c *check.C, svc appTypes.AppLogService, appName, message, source, unit string, level int) {
	err := svc.Add(appName, message, source, unit, level)
	c.Assert(err, check.IsNil)
}

func (s *ServiceSuite) TestWatch(c *check.C) {
	l, err := s.svc.Watch("myapp", "", "", nil)
	c.Assert(err, check.IsNil)
	defer l.Close()
	c.Assert(l.Chan(), check.NotNil)
	addLog(c, s.svc, "myapp", "123", "", "", 0)
	logMsg := <-l.Chan()
	c.Assert(logMsg.Message, check.Equals, "123")
	addLog(c, s.svc, "myapp", "456", "", "", 0)
	logMsg = <-l.Chan()
	c.Assert(logMsg.Message, check.Equals, "456")
}

func (s *ServiceSuite) TestWatchFiltered(c *check.C) {
	l, err := s.svc.Watch("myapp", "web", "u1", nil)
	c.Assert(err, check.IsNil)
	defer l.Close()
	c.Assert(l.Chan(), check.NotNil)
	logs := []appTypes.Applog{
		{Message: "1", Source: "web", Unit: "u1"},
		{Message: "2", Source: "worker", Unit: "u1"},
		{Message: "3", Source: "web", Unit: "u1"},
		{Message: "4", Source: "web", Unit: "u2"},
		{Message: "5", Source: "web", Unit: "u1"},
	}
	for _, log := range logs {
		addLog(c, s.svc, "myapp", log.Message, log.Source, log.Unit, log.Level)
	}
	logMsg := <-l.Chan()
	c.Assert(logMsg.Message, check.Equals, "1")
	logMsg = <-l.Chan()
	c.Assert(logMsg.Message, check.Equals, "3")
	logMsg = <-l.Chan()
	c.Assert(logMsg.Message, check.Equals, "5")
}

func (s *ServiceSuite) TestWatchClosingChannel(c *check.C) {
	l, err := s.svc.Watch("myapp", "", "", nil)
	c.Assert(err, check.IsNil)
	c.Assert(l.Chan(), check.NotNil)
	l.Close()
	_, ok := <-l.Chan()
	c.Assert(ok, check.Equals, false)
}

func (s *ServiceSuite) TestWatchClose(c *check.C) {
	l, err := s.svc.Watch("myapp", "", "", nil)
	c.Assert(err, check.IsNil)
	l.Close()
	_, ok := <-l.Chan()
	c.Assert(ok, check.Equals, false)
}

func (s *ServiceSuite) TestWatchDoubleClose(c *check.C) {
	defer func() {
		c.Assert(recover(), check.IsNil)
	}()
	l, err := s.svc.Watch("yourapp", "", "", nil)
	c.Assert(err, check.IsNil)
	l.Close()
	l.Close()
}

func (s *ServiceSuite) TestWatchNotify(c *check.C) {
	var logs struct {
		l []appTypes.Applog
		sync.Mutex
	}
	l, err := s.svc.Watch("fade", "", "", nil)
	c.Assert(err, check.IsNil)
	defer l.Close()
	go func() {
		for log := range l.Chan() {
			logs.Lock()
			logs.l = append(logs.l, log)
			logs.Unlock()
		}
	}()
	ms := []appTypes.Applog{
		{Message: "Something went wrong. Check it out:", Source: "tsuru", Unit: "some", AppName: "fade"},
		{Message: "This program has performed an illegal operation.", Source: "tsuru", Unit: "some", AppName: "fade"},
	}
	for _, log := range ms {
		addLog(c, s.svc, "fade", log.Message, log.Source, log.Unit, log.Level)
	}
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
	compareLogsNoDate(c, logs.l, ms)
}

func (s *ServiceSuite) TestWatchNotifyFiltered(c *check.C) {
	var logs struct {
		l []appTypes.Applog
		sync.Mutex
	}
	l, err := s.svc.Watch("fade", "tsuru", "unit1", nil)
	c.Assert(err, check.IsNil)
	defer l.Close()
	go func() {
		for log := range l.Chan() {
			logs.Lock()
			logs.l = append(logs.l, log)
			logs.Unlock()
		}
	}()
	ms := []appTypes.Applog{
		{Message: "Something went wrong. Check it out:", Source: "tsuru", Unit: "unit1"},
		{Message: "This program has performed an illegal operation.", Source: "other", Unit: "unit1"},
		{Message: "Last one.", Source: "tsuru", Unit: "unit2"},
	}
	for _, log := range ms {
		addLog(c, s.svc, "fade", log.Message, log.Source, log.Unit, log.Level)
	}
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
	compareLogsNoDate(c, logs.l, []appTypes.Applog{
		{Message: "Something went wrong. Check it out:", Source: "tsuru", Unit: "unit1", AppName: "fade"},
	})
}

func (s *ServiceSuite) TestWatchNotifySendOnClosedChannel(c *check.C) {
	defer func() {
		c.Assert(recover(), check.IsNil)
	}()
	l, err := s.svc.Watch("fade", "", "", nil)
	c.Assert(err, check.IsNil)
	l.Close()
	addLog(c, s.svc, "fade", "Something went wrong. Check it out:", "tsuru", "", 0)
}
