// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/app"
	"github.com/globocom/tsuru/auth"
	"github.com/globocom/tsuru/db"
	fsTesting "github.com/globocom/tsuru/fs/testing"
	"github.com/globocom/tsuru/queue"
	"github.com/globocom/tsuru/service"
	tsuruTesting "github.com/globocom/tsuru/testing"
	"io"
	. "launchpad.net/gocheck"
	"os"
	"path"
	"testing"
)

func Test(t *testing.T) { TestingT(t) }

type S struct {
	team            *auth.Team
	user            *auth.User
	token           *auth.Token
	rfs             *fsTesting.RecordingFs
	t               *tsuruTesting.T
	provisioner     *tsuruTesting.FakeProvisioner
	service         *service.Service
	serviceInstance *service.ServiceInstance
	tmpdir          string
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

type greaterChecker struct{}

func (c *greaterChecker) Info() *CheckerInfo {
	return &CheckerInfo{Name: "Greater", Params: []string{"expected", "obtained"}}
}

func (c *greaterChecker) Check(params []interface{}, names []string) (bool, string) {
	if len(params) != 2 {
		return false, "you should pass two values to compare"
	}
	n1, ok := params[0].(int)
	if !ok {
		return false, "first parameter should be int"
	}
	n2, ok := params[1].(int)
	if !ok {
		return false, "second parameter should be int"
	}
	if n1 > n2 {
		return true, ""
	}
	err := fmt.Sprintf("%s is not greater than %s", params[0], params[1])
	return false, err
}

func (s *S) createUserAndTeam(c *C) {
	s.user = &auth.User{Email: "whydidifall@thewho.com", Password: "123"}
	err := s.user.Create()
	c.Assert(err, IsNil)
	s.token, _ = s.user.CreateToken()
	s.team = &auth.Team{Name: "tsuruteam", Users: []string{s.user.Email}}
	err = db.Session.Teams().Insert(s.team)
	c.Assert(err, IsNil)
}

func (s *S) SetUpSuite(c *C) {
	var err error
	err = config.ReadConfigFile("../etc/tsuru.conf")
	c.Assert(err, IsNil)
	db.Session, err = db.Open("127.0.0.1:27017", "tsuru_api_test")
	c.Assert(err, IsNil)
	s.createUserAndTeam(c)
	s.rfs = &fsTesting.RecordingFs{}
	file, err := s.rfs.Open("/dev/urandom")
	c.Assert(err, IsNil)
	file.Write([]byte{16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31})
	fsystem = s.rfs
	s.t = &tsuruTesting.T{}
	s.t.StartAmzS3AndIAM(c)
	s.t.SetGitConfs(c)
	s.provisioner = tsuruTesting.NewFakeProvisioner()
	app.Provisioner = s.provisioner
}

func (s *S) TearDownSuite(c *C) {
	defer s.t.S3Server.Quit()
	defer s.t.IamServer.Quit()
	defer db.Session.Close()
	db.Session.Apps().Database.DropDatabase()
	fsystem = nil
	queue.Preempt()
}

func (s *S) TearDownTest(c *C) {
	s.t.RollbackGitConfs(c)
	s.provisioner.Reset()
	_, err := db.Session.Services().RemoveAll(nil)
	c.Assert(err, IsNil)
	_, err = db.Session.ServiceInstances().RemoveAll(nil)
	c.Assert(err, IsNil)
}

func (s *S) getTestData(p ...string) io.ReadCloser {
	p = append([]string{}, ".", "testdata")
	fp := path.Join(p...)
	f, _ := os.OpenFile(fp, os.O_RDONLY, 0)
	return f
}
