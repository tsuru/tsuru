// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"sync"
	"time"

	"gopkg.in/mgo.v2/bson"
	"launchpad.net/gocheck"
)

func (s *S) TestNewLogListener(c *gocheck.C) {
	app := App{Name: "myapp"}
	l, err := NewLogListener(&app, Applog{})
	defer l.Close()
	c.Assert(err, gocheck.IsNil)
	c.Assert(l.q, gocheck.NotNil)
	c.Assert(l.C, gocheck.NotNil)
	notify("myapp", []interface{}{Applog{Message: "123"}})
	logMsg := <-l.C
	c.Assert(logMsg.Message, gocheck.Equals, "123")
}

func (s *S) TestNewLogListenerClosingChannel(c *gocheck.C) {
	app := App{Name: "myapp"}
	l, err := NewLogListener(&app, Applog{})
	c.Assert(err, gocheck.IsNil)
	c.Assert(l.q, gocheck.NotNil)
	c.Assert(l.C, gocheck.NotNil)
	l.Close()
	_, ok := <-l.C
	c.Assert(ok, gocheck.Equals, false)
}

func (s *S) TestLogListenerClose(c *gocheck.C) {
	app := App{Name: "myapp"}
	l, err := NewLogListener(&app, Applog{})
	c.Assert(err, gocheck.IsNil)
	err = l.Close()
	c.Assert(err, gocheck.IsNil)
	_, ok := <-l.C
	c.Assert(ok, gocheck.Equals, false)
}

func (s *S) TestLogListenerDoubleClose(c *gocheck.C) {
	app := App{Name: "yourapp"}
	l, err := NewLogListener(&app, Applog{})
	c.Assert(err, gocheck.IsNil)
	err = l.Close()
	c.Assert(err, gocheck.IsNil)
	err = l.Close()
	c.Assert(err, gocheck.NotNil)
}

func (s *S) TestNotify(c *gocheck.C) {
	var logs struct {
		l []interface{}
		sync.Mutex
	}
	app := App{Name: "fade"}
	l, err := NewLogListener(&app, Applog{})
	c.Assert(err, gocheck.IsNil)
	defer l.Close()
	go func() {
		for log := range l.C {
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
	notify(app.Name, ms)
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
	c.Assert(logs.l, gocheck.DeepEquals, ms)
}

func (s *S) TestNotifyFiltered(c *gocheck.C) {
	var logs struct {
		l []interface{}
		sync.Mutex
	}
	app := App{Name: "fade"}
	l, err := NewLogListener(&app, Applog{Source: "tsuru", Unit: "unit1"})
	c.Assert(err, gocheck.IsNil)
	defer l.Close()
	go func() {
		for log := range l.C {
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
	notify(app.Name, ms)
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
	expected := []interface{}{
		Applog{Date: t, Message: "Something went wrong. Check it out:", Source: "tsuru", Unit: "unit1"},
	}
	c.Assert(logs.l, gocheck.DeepEquals, expected)
}

func (s *S) TestNotifySendOnClosedChannel(c *gocheck.C) {
	defer func() {
		c.Assert(recover(), gocheck.IsNil)
	}()
	app := App{Name: "fade"}
	l, err := NewLogListener(&app, Applog{})
	c.Assert(err, gocheck.IsNil)
	err = l.Close()
	c.Assert(err, gocheck.IsNil)
	ms := []interface{}{
		Applog{Date: time.Now(), Message: "Something went wrong. Check it out:", Source: "tsuru"},
	}
	notify(app.Name, ms)
}

func (s *S) TestLogRemove(c *gocheck.C) {
	a := App{Name: "newApp"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	a2 := App{Name: "newApp2"}
	err = s.conn.Apps().Insert(a2)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().RemoveAll(nil)
	err = a.Log("last log msg", "tsuru", "hari")
	c.Assert(err, gocheck.IsNil)
	err = LogRemove(nil)
	c.Assert(err, gocheck.IsNil)
	count, err := s.conn.Logs(a.Name).Find(nil).Count()
	c.Assert(err, gocheck.IsNil)
	c.Assert(count, gocheck.Equals, 0)
	count, err = s.conn.Logs(a2.Name).Find(nil).Count()
	c.Assert(err, gocheck.IsNil)
	c.Assert(count, gocheck.Equals, 0)
}

func (s *S) TestLogRemoveByApp(c *gocheck.C) {
	a := App{Name: "newApp"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	err = a.Log("last log msg", "tsuru", "hari")
	c.Assert(err, gocheck.IsNil)
	a2 := App{Name: "oldApp"}
	err = s.conn.Apps().Insert(a2)
	c.Assert(err, gocheck.IsNil)
	defer func() {
		s.conn.Apps().Remove(bson.M{"name": a.Name})
		s.conn.Apps().Remove(bson.M{"name": a2.Name})
		s.conn.Logs(a.Name).DropCollection()
		s.conn.Logs(a2.Name).DropCollection()
	}()
	err = a2.Log("last log msg", "tsuru", "hari")
	c.Assert(err, gocheck.IsNil)
	err = LogRemove(&a)
	c.Assert(err, gocheck.IsNil)
	count, err := s.conn.Logs(a2.Name).Find(nil).Count()
	c.Assert(err, gocheck.IsNil)
	c.Assert(count, gocheck.Equals, 1)
}
