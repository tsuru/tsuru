// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"time"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

type WriterSuite struct {
	conn *db.Storage
}

var _ = check.Suite(&WriterSuite{})

func (s *WriterSuite) SetUpSuite(c *check.C) {
	var err error
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "tsuru_api_writer_test")
	s.conn, err = db.Conn()
	c.Assert(err, check.IsNil)
}

func (s *WriterSuite) TearDownSuite(c *check.C) {
	dbtest.ClearAllCollections(s.conn.Apps().Database)
}

func (s *WriterSuite) TestLogWriter(c *check.C) {
	a := App{Name: "down"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	writer := LogWriter{App: &a}
	data := []byte("ble")
	_, err = writer.Write(data)
	c.Assert(err, check.IsNil)
	instance := App{}
	err = s.conn.Apps().Find(bson.M{"name": a.Name}).One(&instance)
	logs, err := instance.LastLogs(1, Applog{})
	c.Assert(err, check.IsNil)
	c.Assert(logs[0].Message, check.Equals, string(data))
	c.Assert(logs[0].Source, check.Equals, "tsuru")
}

func (s *WriterSuite) TestLogWriterCustomSource(c *check.C) {
	a := App{Name: "down"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	writer := LogWriter{App: &a, Source: "cool-test"}
	data := []byte("ble")
	_, err = writer.Write(data)
	c.Assert(err, check.IsNil)
	instance := App{}
	err = s.conn.Apps().Find(bson.M{"name": a.Name}).One(&instance)
	logs, err := instance.LastLogs(1, Applog{})
	c.Assert(err, check.IsNil)
	c.Assert(logs[0].Message, check.Equals, string(data))
	c.Assert(logs[0].Source, check.Equals, "cool-test")
}

func (s *WriterSuite) TestLogWriterShouldReturnTheDataSize(c *check.C) {
	a := App{Name: "down"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	var apps []App
	s.conn.Apps().Find(bson.M{"name": "down"}).All(&apps)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	writer := LogWriter{App: &a}
	data := []byte("ble")
	n, err := writer.Write(data)
	c.Assert(err, check.IsNil)
	c.Assert(n, check.Equals, len(data))
}

func (s *WriterSuite) TestLogWriterAsync(c *check.C) {
	a := App{Name: "down"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	writer := LogWriter{App: &a}
	writer.Async()
	data := []byte("ble")
	_, err = writer.Write(data)
	c.Assert(err, check.IsNil)
	writer.Close()
	err = writer.Wait(5 * time.Second)
	c.Assert(err, check.IsNil)
	instance := App{}
	err = s.conn.Apps().Find(bson.M{"name": a.Name}).One(&instance)
	logs, err := instance.LastLogs(1, Applog{})
	c.Assert(err, check.IsNil)
	c.Assert(logs[0].Message, check.Equals, string(data))
	c.Assert(logs[0].Source, check.Equals, "tsuru")
}

func (s *WriterSuite) TestLogWriterAsyncTimeout(c *check.C) {
	a := App{Name: "down"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	writer := LogWriter{App: &a}
	writer.Async()
	for i := 0; i < 900; i++ {
		data := []byte("ble")
		_, err = writer.Write(data)
		c.Assert(err, check.IsNil)
	}
	writer.Close()
	err = writer.Wait(0)
	c.Assert(err, check.ErrorMatches, "timeout waiting for writer to finish")
	err = writer.Wait(10 * time.Second)
	c.Assert(err, check.IsNil)
}
