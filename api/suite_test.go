// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"fmt"
	"github.com/fsouza/go-iam/iamtest"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/api/auth"
	"github.com/globocom/tsuru/db"
	fsTesting "github.com/globocom/tsuru/fs/testing"
	"io"
	"launchpad.net/goamz/s3/s3test"
	. "launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"strconv"
	"strings"
	"testing"
)

func Test(t *testing.T) { TestingT(t) }

type S struct {
	team      auth.Team
	user      *auth.User
	gitRoot   string
	rfs       *fsTesting.RecordingFs
	iamServer *iamtest.Server
	s3Server  *s3test.Server
	gitHost   string
	gitPort   string
	gitProt   string
}

var _ = Suite(&S{})

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

func (s *S) SetUpSuite(c *C) {
	var err error
	err = config.ReadConfigFile("../etc/tsuru.conf")
	c.Assert(err, IsNil)
	db.Session, err = db.Open("127.0.0.1:27017", "tsuru_app_test")
	c.Assert(err, IsNil)
	s.createUserAndTeam(c)
	s.rfs = &fsTesting.RecordingFs{}
	file, err := s.rfs.Open("/dev/urandom")
	c.Assert(err, IsNil)
	file.Write([]byte{16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31})
	fsystem = s.rfs
	s.startAmzS3AndIAM(c)
	s.setGitConfs(c)
}

func (s *S) TearDownSuite(c *C) {
	defer s.s3Server.Quit()
	defer s.iamServer.Quit()
	defer db.Session.Close()
	db.Session.Apps().Database.DropDatabase()
	fsystem = nil
}

func (s *S) TearDownTest(c *C) {
	s.rollbackGitConfs(c)
}

func (s *S) getTestData(p ...string) io.ReadCloser {
	p = append([]string{}, ".", "testdata")
	fp := path.Join(p...)
	f, _ := os.OpenFile(fp, os.O_RDONLY, 0)
	return f
}

func (s *S) startAmzS3AndIAM(c *C) {
	var err error
	s.s3Server, err = s3test.NewServer()
	c.Assert(err, IsNil)
	config.Set("aws:s3:endpoint", s.s3Server.URL())
	s.iamServer, err = iamtest.NewServer()
	c.Assert(err, IsNil)
	config.Set("aws:iam:endpoint", s.iamServer.URL())
	config.Unset("aws:s3:bucketEndpoint")
}

func (s *S) setGitConfs(c *C) {
	s.gitHost, _ = config.GetString("git:host")
	s.gitPort, _ = config.GetString("git:port")
	s.gitProt, _ = config.GetString("git:protocol")
}

func (s *S) rollbackGitConfs(c *C) {
	config.Set("git:host", s.gitHost)
	config.Set("git:port", s.gitPort)
	config.Set("git:protocol", s.gitProt)
}

func (s *S) createUserAndTeam(c *C) {
	s.user = &auth.User{Email: "whydidifall@thewho.com", Password: "123"}
	s.user.Create()
	s.team = auth.Team{Name: "tsuruteam", Users: []string{s.user.Email}}
	db.Session.Teams().Insert(s.team)
}

// starts a new httptest.Server and returns it
// Also changes git:host, git:port and git:protocol to match the server's url
func (s *S) startGandalfTestServer(h http.Handler) *httptest.Server {
	ts := httptest.NewServer(h)
	pieces := strings.Split(ts.URL, "://")
	protocol := pieces[0]
	hostPart := strings.Split(pieces[1], ":")
	port := hostPart[1]
	host := hostPart[0]
	config.Set("git:host", host)
	portInt, _ := strconv.ParseInt(port, 10, 0)
	config.Set("git:port", portInt)
	config.Set("git:protocol", protocol)
	return ts
}
