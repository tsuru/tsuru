// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import (
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"launchpad.net/gocheck"
)

type S struct {
	conn    *db.Storage
	service *Service
	team    *auth.Team
	user    *auth.User
	tmpdir  string
}

var _ = gocheck.Suite(&S{})

type hasAccessToChecker struct{}

func (c *hasAccessToChecker) Info() *gocheck.CheckerInfo {
	return &gocheck.CheckerInfo{Name: "HasAccessTo", Params: []string{"team", "service"}}
}

func (c *hasAccessToChecker) Check(params []interface{}, names []string) (bool, string) {
	if len(params) != 2 {
		return false, "you must provide two parameters"
	}
	team, ok := params[0].(auth.Team)
	if !ok {
		return false, "first parameter should be a team instance"
	}
	service, ok := params[1].(Service)
	if !ok {
		return false, "second parameter should be service instance"
	}
	return service.HasTeam(&team), ""
}

var HasAccessTo gocheck.Checker = &hasAccessToChecker{}

func (s *S) SetUpSuite(c *gocheck.C) {
	var err error
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "tsuru_service_test")
	s.conn, err = db.Conn()
	c.Assert(err, gocheck.IsNil)
	s.user = &auth.User{Email: "cidade@raul.com"}
	err = s.user.Create()
	c.Assert(err, gocheck.IsNil)
	s.team = &auth.Team{Name: "Raul", Users: []string{s.user.Email}}
	err = s.conn.Teams().Insert(s.team)
	c.Assert(err, gocheck.IsNil)
	if err != nil {
		c.Fail()
	}
}

func (s *S) TearDownSuite(c *gocheck.C) {
	dbtest.ClearAllCollections(s.conn.Apps().Database)
}

func (s *S) TearDownTest(c *gocheck.C) {
	_, err := s.conn.Services().RemoveAll(nil)
	c.Assert(err, gocheck.IsNil)
}
