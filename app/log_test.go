// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"labix.org/v2/mgo/bson"
	"launchpad.net/gocheck"
	"sync"
	"time"
)

func (s *S) TestNewLogListener(c *gocheck.C) {
	app := App{Name: "myapp"}
	l := NewLogListener(&app)
	c.Assert(l.appname, gocheck.Equals, "myapp")
	c.Assert(l.state, gocheck.Equals, open)
	c.Assert(l.C, gocheck.NotNil)
	close(l.c)
	_, ok := <-l.C
	c.Assert(ok, gocheck.Equals, false)
	ls := listeners.m["myapp"]
	c.Assert(ls, gocheck.HasLen, 1)
	c.Assert(ls[0], gocheck.Equals, l)
	delete(listeners.m, "myapp")
}

func (s *S) TestLogListenerClose(c *gocheck.C) {
	app := App{Name: "yourapp"}
	l := NewLogListener(&app)
	err := l.Close()
	c.Assert(err, gocheck.IsNil)
	c.Assert(l.state, gocheck.Equals, closed)
	_, ok := <-l.C
	c.Assert(ok, gocheck.Equals, false)
	ls := listeners.m["yourapp"]
	c.Assert(ls, gocheck.HasLen, 0)
}

func (s *S) TestLogListenerDoubleClose(c *gocheck.C) {
	app := App{Name: "yourapp"}
	l := NewLogListener(&app)
	err := l.Close()
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
	l := NewLogListener(&app)
	defer l.Close()
	go func() {
		for log := range l.C {
			logs.Lock()
			logs.l = append(logs.l, log)
			logs.Unlock()
		}
	}()
	ms := []interface{}{
		Applog{Date: time.Now(), Message: "Something went wrong. Check it out:", Source: "tsuru"},
		Applog{Date: time.Now(), Message: "This program has performed an illegal operation.", Source: "tsuru"},
	}
	notify(app.Name, ms)
	done := make(chan bool, 1)
	q := make(chan bool)
	go func(quit chan bool) {
		for _ = range time.Tick(1e3) {
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

func (s *S) TestNotifySendOnClosedChannel(c *gocheck.C) {
	defer func() {
		c.Assert(recover(), gocheck.IsNil)
	}()
	app := App{Name: "fade"}
	l := NewLogListener(&app)
	close(l.c)
	ms := []interface{}{
		Applog{Date: time.Now(), Message: "Something went wrong. Check it out:", Source: "tsuru"},
	}
	notify(app.Name, ms)
}

func (s *S) TestLogRemove(c *gocheck.C) {
	a := App{Name: "newApp"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	err = a.Log("last log msg", "tsuru")
	c.Assert(err, gocheck.IsNil)
	err = LogRemove(nil)
	c.Assert(err, gocheck.IsNil)
	count, err := s.conn.Logs().Find(nil).Count()
	c.Assert(err, gocheck.IsNil)
	c.Assert(count, gocheck.Equals, 0)
}

func (s *S) TestLogRemoveByApp(c *gocheck.C) {
	a := App{Name: "newApp"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	err = a.Log("last log msg", "tsuru")
	c.Assert(err, gocheck.IsNil)
	a2 := App{Name: "oldApp"}
	err = s.conn.Apps().Insert(a2)
	c.Assert(err, gocheck.IsNil)
	defer func() {
		s.conn.Apps().Remove(bson.M{"name": a.Name})
		s.conn.Apps().Remove(bson.M{"name": a2.Name})
		s.conn.Logs().RemoveAll(nil)
	}()
	err = a2.Log("last log msg", "tsuru")
	c.Assert(err, gocheck.IsNil)
	err = LogRemove(&a)
	c.Assert(err, gocheck.IsNil)
	count, err := s.conn.Logs().Find(nil).Count()
	c.Assert(err, gocheck.IsNil)
	c.Assert(count, gocheck.Equals, 1)
}
