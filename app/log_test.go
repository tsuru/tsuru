// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"sync"
	"time"

	"gopkg.in/check.v1"
)

func (s *S) TestNewLogListener(c *check.C) {
	app := App{Name: "myapp"}
	l, err := NewLogListener(&app, Applog{})
	defer l.Close()
	c.Assert(err, check.IsNil)
	c.Assert(l.q, check.NotNil)
	c.Assert(l.c, check.NotNil)
	notify("myapp", []interface{}{Applog{Message: "123"}})
	logMsg := <-l.c
	c.Assert(logMsg.Message, check.Equals, "123")
}

func (s *S) TestNewLogListenerClosingChannel(c *check.C) {
	app := App{Name: "myapp"}
	l, err := NewLogListener(&app, Applog{})
	c.Assert(err, check.IsNil)
	c.Assert(l.q, check.NotNil)
	c.Assert(l.c, check.NotNil)
	l.Close()
	_, ok := <-l.c
	c.Assert(ok, check.Equals, false)
}

func (s *S) TestLogListenerClose(c *check.C) {
	app := App{Name: "myapp"}
	l, err := NewLogListener(&app, Applog{})
	c.Assert(err, check.IsNil)
	err = l.Close()
	c.Assert(err, check.IsNil)
	_, ok := <-l.c
	c.Assert(ok, check.Equals, false)
}

func (s *S) TestLogListenerDoubleClose(c *check.C) {
	app := App{Name: "yourapp"}
	l, err := NewLogListener(&app, Applog{})
	c.Assert(err, check.IsNil)
	err = l.Close()
	c.Assert(err, check.IsNil)
	err = l.Close()
	c.Assert(err, check.NotNil)
}

func (s *S) TestNotify(c *check.C) {
	var logs struct {
		l []interface{}
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
	c.Assert(logs.l, check.DeepEquals, ms)
}

func (s *S) TestNotifyFiltered(c *check.C) {
	var logs struct {
		l []interface{}
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
	c.Assert(logs.l, check.DeepEquals, expected)
}

func (s *S) TestNotifySendOnClosedChannel(c *check.C) {
	defer func() {
		c.Assert(recover(), check.IsNil)
	}()
	app := App{Name: "fade"}
	l, err := NewLogListener(&app, Applog{})
	c.Assert(err, check.IsNil)
	err = l.Close()
	c.Assert(err, check.IsNil)
	ms := []interface{}{
		Applog{Date: time.Now(), Message: "Something went wrong. Check it out:", Source: "tsuru"},
	}
	notify(app.Name, ms)
}
