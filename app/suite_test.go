// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"fmt"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/auth"
	"github.com/globocom/tsuru/db"
	ftesting "github.com/globocom/tsuru/fs/testing"
	"github.com/globocom/tsuru/queue"
	ttesting "github.com/globocom/tsuru/testing"
	"io"
	"labix.org/v2/mgo/bson"
	. "launchpad.net/gocheck"
	"os"
	"path"
	"testing"
)

func Test(t *testing.T) { TestingT(t) }

type S struct {
	team        auth.Team
	user        *auth.User
	adminTeam   auth.Team
	admin       *auth.User
	rfs         *ftesting.RecordingFs
	t           *ttesting.T
	provisioner *ttesting.FakeProvisioner
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
	err := fmt.Sprintf("%d is not greater than %d", params[0], params[1])
	return false, err
}

var Greater Checker = &greaterChecker{}

func (s *S) createUserAndTeam(c *C) {
	s.user = &auth.User{Email: "whydidifall@thewho.com", Password: "123"}
	err := s.user.Create()
	c.Assert(err, IsNil)
	s.team = auth.Team{Name: "tsuruteam", Users: []string{s.user.Email}}
	err = db.Session.Teams().Insert(s.team)
	c.Assert(err, IsNil)
}

func (s *S) SetUpSuite(c *C) {
	var err error
	err = config.ReadConfigFile("../etc/tsuru.conf")
	c.Assert(err, IsNil)
	db.Session, err = db.Open("127.0.0.1:27017", "tsuru_app_test")
	c.Assert(err, IsNil)
	s.rfs = &ftesting.RecordingFs{}
	file, err := s.rfs.Open("/dev/urandom")
	c.Assert(err, IsNil)
	file.Write([]byte{16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31})
	fsystem = s.rfs
	s.t = &ttesting.T{}
	s.createUserAndTeam(c)
	s.t.StartAmzS3AndIAM(c)
	s.t.SetGitConfs(c)
	s.provisioner = ttesting.NewFakeProvisioner()
	Provisioner = s.provisioner
	err = nil
	for err == nil {
		err = handler.DryRun()
	}
}

func (s *S) TearDownSuite(c *C) {
	defer s.t.S3Server.Quit()
	defer s.t.IamServer.Quit()
	defer db.Session.Close()
	db.Session.Apps().Database.DropDatabase()
	fsystem = nil
	handler.Stop()
	handler.Wait()
}

func (s *S) SetUpTest(c *C) {
	var (
		err error
		msg *queue.Message
	)
	for err == nil {
		if msg, err = queue.Get(1e6); err == nil {
			err = queue.Delete(msg)
		}
	}
}

func (s *S) TearDownTest(c *C) {
	s.t.RollbackGitConfs(c)
	s.provisioner.Reset()
}

func (s *S) getTestData(p ...string) io.ReadCloser {
	p = append([]string{}, ".", "testdata")
	fp := path.Join(p...)
	f, _ := os.OpenFile(fp, os.O_RDONLY, 0)
	return f
}

func (s *S) createAdminUserAndTeam(c *C) {
	s.admin = &auth.User{Email: "superuser@gmail.com", Password: "123"}
	err := db.Session.Users().Insert(&s.admin)
	c.Assert(err, IsNil)
	adminTeamName, err := config.GetString("admin-team")
	c.Assert(err, IsNil)
	s.adminTeam = auth.Team{Name: adminTeamName, Users: []string{s.admin.Email}}
	err = db.Session.Teams().Insert(&s.adminTeam)
	c.Assert(err, IsNil)
}

func (s *S) removeAdminUserAndTeam(c *C) {
	err := db.Session.Teams().RemoveId(s.adminTeam.Name)
	c.Assert(err, IsNil)
	err = db.Session.Users().Remove(bson.M{"email": s.admin.Email})
	c.Assert(err, IsNil)
}

type MessageList []queue.Message

func (l MessageList) Len() int {
	return len(l)
}

func (l MessageList) Less(i, j int) bool {
	if l[i].Action < l[j].Action {
		return true
	} else if l[i].Action > l[j].Action {
		return false
	}
	if len(l[i].Args) == 0 {
		return true
	} else if len(l[j].Args) == 0 {
		return false
	}
	smaller := len(l[i].Args)
	if len(l[j].Args) < smaller {
		smaller = len(l[j].Args)
	}
	for k := 0; k < smaller; k++ {
		if l[i].Args[k] < l[j].Args[k] {
			return true
		} else if l[i].Args[k] > l[j].Args[k] {
			return false
		}
	}
	return false
}

func (l MessageList) Swap(i, j int) {
	l[i], l[j] = l[j], l[i]
}
