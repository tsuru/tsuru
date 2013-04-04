// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/app"
	"github.com/globocom/tsuru/db"
	"labix.org/v2/mgo/bson"
	"launchpad.net/gocheck"
	"net/http/httptest"
)

type WriterSuite struct {
	conn *db.Storage
}

var _ = gocheck.Suite(&WriterSuite{})

func (s *WriterSuite) SetUpSuite(c *gocheck.C) {
	var err error
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "tsuru_api_writer_test")
	s.conn, err = db.Conn()
	c.Assert(err, gocheck.IsNil)
}

func (s *WriterSuite) TearDownSuite(c *gocheck.C) {
	s.conn.Apps().Database.DropDatabase()
}

func (s *WriterSuite) TestLogWriter(c *gocheck.C) {
	var b bytes.Buffer
	a := app.App{Name: "down"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	writer := LogWriter{&a, &b}
	data := []byte("ble")
	_, err = writer.Write(data)
	c.Assert(err, gocheck.IsNil)
	c.Assert(b.Bytes(), gocheck.DeepEquals, data)
	instance := app.App{}
	err = s.conn.Apps().Find(bson.M{"name": a.Name}).One(&instance)
	logs, err := instance.LastLogs(1, "")
	c.Assert(err, gocheck.IsNil)
	c.Assert(logs[0].Message, gocheck.Equals, string(data))
}

func (s *WriterSuite) TestLogWriterShouldReturnTheDataSize(c *gocheck.C) {
	var b bytes.Buffer
	a := app.App{Name: "down"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	var apps []App
	s.conn.Apps().Find(bson.M{"name": "down"}).All(&apps)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	writer := LogWriter{&a, &b}
	data := []byte("ble")
	n, err := writer.Write(data)
	c.Assert(err, gocheck.IsNil)
	c.Assert(n, gocheck.Equals, len(data))
}

func (s *WriterSuite) TestFlushingWriter(c *gocheck.C) {
	recorder := httptest.NewRecorder()
	writer := FlushingWriter{recorder, false}
	data := []byte("ble")
	_, err := writer.Write(data)
	c.Assert(err, gocheck.IsNil)
	c.Assert(recorder.Body.Bytes(), gocheck.DeepEquals, data)
	c.Assert(writer.wrote, gocheck.Equals, true)
}

func (s *WriterSuite) TestFlushingWriterShouldReturnTheDataSize(c *gocheck.C) {
	recorder := httptest.NewRecorder()
	writer := FlushingWriter{recorder, false}
	data := []byte("ble")
	n, err := writer.Write(data)
	c.Assert(err, gocheck.IsNil)
	c.Assert(n, gocheck.Equals, len(data))
}

func (s *WriterSuite) TestFlushingWriterHeader(c *gocheck.C) {
	recorder := httptest.NewRecorder()
	writer := FlushingWriter{recorder, false}
	writer.Header().Set("Content-Type", "application/xml")
	c.Assert(recorder.Header().Get("Content-Type"), gocheck.Equals, "application/xml")
}

func (s *WriterSuite) TestFlushingWriterWriteHeader(c *gocheck.C) {
	recorder := httptest.NewRecorder()
	writer := FlushingWriter{recorder, false}
	expectedCode := 333
	writer.WriteHeader(expectedCode)
	c.Assert(recorder.Code, gocheck.Equals, expectedCode)
	c.Assert(writer.wrote, gocheck.Equals, true)
}
