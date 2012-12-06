// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package repository

import (
	"github.com/globocom/commandmocker"
	"github.com/globocom/config"
	//"github.com/globocom/tsuru/log"
	. "launchpad.net/gocheck"
	//stdlog "log"
	//"os"
	"testing"
)

func Test(t *testing.T) { TestingT(t) }

type S struct {
	//logFile     *os.File
	tmpdir string
}

var _ = Suite(&S{})

func (s *S) SetUpSuite(c *C) {
	var err error
	s.tmpdir, err = commandmocker.Add("juju", "")
	c.Assert(err, IsNil)
	err = config.ReadConfigFile("../etc/tsuru.conf")
	c.Assert(err, IsNil)
	//log.SetLogger(stdlog.New(s.logFile, "[tsuru-tests]", stdlog.LstdFlags|stdlog.Llongfile))
}

func (s *S) TearDownSuite(c *C) {
	commandmocker.Remove(s.tmpdir)
	//defer s.logFile.Close()
}

func (s *S) TestGetGitServerPanicsIfTheConfigFileHasNoServer(c *C) {
	oldConfig, err := config.Get("git")
	c.Assert(err, IsNil)
	err = config.Unset("git")
	c.Assert(err, IsNil)
	defer config.Set("git", oldConfig)
	c.Assert(getGitServer, PanicMatches, "key git:host not found")
}
