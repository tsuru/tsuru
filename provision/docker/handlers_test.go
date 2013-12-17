// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"bytes"
	"github.com/globocom/tsuru/db"
	"launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
)

type HandlersSuite struct {
	conn *db.Storage
}

var _ = gocheck.Suite(&HandlersSuite{})

func (s *HandlersSuite) SetUpSuite(c *gocheck.C) {
	var err error
	s.conn, err = db.Conn()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Collection(schedulerCollection).RemoveAll(nil)
}

func (s *HandlersSuite) TearDownSuite(c *gocheck.C) {
	s.conn.Close()
}

func (s *HandlersSuite) TestAddNodeHandler(c *gocheck.C) {
	b := bytes.NewBufferString(`{"address": "host.com:4243", "ID": "server01", "teams": "myteam"}`)
	req, err := http.NewRequest("POST", "/node/add", b)
	c.Assert(err, gocheck.IsNil)
	rec := httptest.NewRecorder()
	err = AddNodeHandler(rec, req)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Collection(schedulerCollection).RemoveId("server01")
	n, err := s.conn.Collection(schedulerCollection).FindId("server01").Count()
	c.Assert(err, gocheck.IsNil)
	c.Assert(n, gocheck.Equals, 1)
}

func (s *HandlersSuite) TestRemoveNodeHandler(c *gocheck.C) {
	err := s.conn.Collection(schedulerCollection).Insert(map[string]string{"address": "host.com:4243", "_id": "server01", "teams": "myteam"})
	c.Assert(err, gocheck.IsNil)
	n, err := s.conn.Collection(schedulerCollection).FindId("server01").Count()
	c.Assert(err, gocheck.IsNil)
	c.Assert(n, gocheck.Equals, 1)
	b := bytes.NewBufferString(`{"ID": "server01"}`)
	req, err := http.NewRequest("POST", "/node/remove", b)
	c.Assert(err, gocheck.IsNil)
	rec := httptest.NewRecorder()
	err = RemoveNodeHandler(rec, req)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Collection(schedulerCollection).RemoveId("server01")
	n, err = s.conn.Collection(schedulerCollection).FindId("server01").Count()
	c.Assert(err, gocheck.IsNil)
	c.Assert(n, gocheck.Equals, 0)
}
