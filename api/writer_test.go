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
	. "launchpad.net/gocheck"
	"net/http/httptest"
)

type WriterSuite struct {
	conn *db.Storage
}

var _ = Suite(&WriterSuite{})

func (s *WriterSuite) SetUpSuite(c *C) {
	var err error
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "tsuru_api_writer_test")
	s.conn, err = db.Conn()
	c.Assert(err, IsNil)
}

func (s *WriterSuite) TearDownSuite(c *C) {
	defer s.conn.Close()
	s.conn.Apps().Database.DropDatabase()
}

func (s *WriterSuite) TestLogWriter(c *C) {
	var b bytes.Buffer
	a := app.App{Name: "down"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	writer := LogWriter{&a, &b}
	data := []byte("ble")
	_, err = writer.Write(data)
	c.Assert(err, IsNil)
	c.Assert(b.Bytes(), DeepEquals, data)
	instance := app.App{}
	err = s.conn.Apps().Find(bson.M{"name": a.Name}).One(&instance)
	logLen := len(instance.Logs)
	c.Assert(instance.Logs[logLen-1].Message, Equals, string(data))
}

func (s *WriterSuite) TestLogWriterShouldReturnTheDataSize(c *C) {
	var b bytes.Buffer
	a := app.App{Name: "down"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, IsNil)
	var apps []App
	s.conn.Apps().Find(bson.M{"name": "down"}).All(&apps)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	writer := LogWriter{&a, &b}
	data := []byte("ble")
	n, err := writer.Write(data)
	c.Assert(err, IsNil)
	c.Assert(n, Equals, len(data))
}

func (s *WriterSuite) TestFlushingWriter(c *C) {
	recorder := httptest.NewRecorder()
	writer := FlushingWriter{recorder, false}
	data := []byte("ble")
	_, err := writer.Write(data)
	c.Assert(err, IsNil)
	c.Assert(recorder.Body.Bytes(), DeepEquals, data)
	c.Assert(writer.wrote, Equals, true)
}

func (s *WriterSuite) TestFlushingWriterShouldReturnTheDataSize(c *C) {
	recorder := httptest.NewRecorder()
	writer := FlushingWriter{recorder, false}
	data := []byte("ble")
	n, err := writer.Write(data)
	c.Assert(err, IsNil)
	c.Assert(n, Equals, len(data))
}

func (s *WriterSuite) TestFlushingWriterHeader(c *C) {
	recorder := httptest.NewRecorder()
	writer := FlushingWriter{recorder, false}
	writer.Header().Set("Content-Type", "application/xml")
	c.Assert(recorder.Header().Get("Content-Type"), Equals, "application/xml")
}

func (s *WriterSuite) TestFlushingWriterWriteHeader(c *C) {
	recorder := httptest.NewRecorder()
	writer := FlushingWriter{recorder, false}
	expectedCode := 333
	writer.WriteHeader(expectedCode)
	c.Assert(recorder.Code, Equals, expectedCode)
	c.Assert(writer.wrote, Equals, true)
}
