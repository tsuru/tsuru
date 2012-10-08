// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package provision

import (
	"github.com/globocom/tsuru/api/auth"
	"github.com/globocom/tsuru/api/service"
	"github.com/globocom/tsuru/db"
	. "launchpad.net/gocheck"
	"testing"
)

func Test(t *testing.T) { TestingT(t) }

type S struct {
	service         *service.Service
	serviceInstance *service.ServiceInstance
	team            *auth.Team
	user            *auth.User
}

var _ = Suite(&S{})

type hasAccessToChecker struct{}

func (c *hasAccessToChecker) Info() *CheckerInfo {
	return &CheckerInfo{Name: "HasAccessTo", Params: []string{"team", "service"}}
}

func (c *hasAccessToChecker) Check(params []interface{}, names []string) (bool, string) {
	if len(params) != 2 {
		return false, "you must provide two parameters"
	}
	team, ok := params[0].(auth.Team)
	if !ok {
		return false, "first parameter should be a team instance"
	}
	srv, ok := params[1].(service.Service)
	if !ok {
		return false, "second parameter should be service instance"
	}
	return srv.HasTeam(&team), ""
}

var HasAccessTo Checker = &hasAccessToChecker{}

func (s *S) SetUpSuite(c *C) {
	var err error
	db.Session, err = db.Open("127.0.0.1:27017", "tsuru_service_provision_test")
	c.Assert(err, IsNil)
	s.user = &auth.User{Email: "cidade@raul.com", Password: "123"}
	err = s.user.Create()
	c.Assert(err, IsNil)
	s.team = &auth.Team{Name: "Raul", Users: []string{s.user.Email}}
	err = db.Session.Teams().Insert(s.team)
	c.Assert(err, IsNil)
	if err != nil {
		c.Fail()
	}
}

func (s *S) TearDownSuite(c *C) {
	defer db.Session.Close()
	db.Session.Apps().Database.DropDatabase()
}

func (s *S) TearDownTest(c *C) {
	_, err := db.Session.Services().RemoveAll(nil)
	c.Assert(err, IsNil)

	_, err = db.Session.ServiceInstances().RemoveAll(nil)
	c.Assert(err, IsNil)
}
