// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"github.com/globocom/config"
	"github.com/globocom/tsuru/db"
	ftesting "github.com/globocom/tsuru/fs/testing"
	"launchpad.net/gocheck"
	"os"
	"testing"
)

func Test(t *testing.T) { gocheck.TestingT(t) }

type S struct {
	collName      string
	imageCollName string
	conn          *db.Storage
	gitHost       string
	repoNamespace string
	deployCmd     string
	runBin        string
	runArgs       string
	port          string
	hostAddr      string
}

var _ = gocheck.Suite(&S{})

func (s *S) SetUpSuite(c *gocheck.C) {
	s.collName = "docker_unit"
	s.imageCollName = "docker_image"
	s.gitHost = "my.gandalf.com"
	s.repoNamespace = "tsuru"
	s.hostAddr = "10.0.0.4"
	config.Set("git:host", s.gitHost)
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "docker_provision_tests_s")
	config.Set("docker:repository-namespace", s.repoNamespace)
	config.Set("docker:binary", "docker")
	config.Set("docker:router", "fake")
	config.Set("docker:collection", s.collName)
	config.Set("docker:host-address", hostAddr)
	config.Set("docker:deploy-cmd", "/var/lib/tsuru/deploy")
	config.Set("docker:run-cmd:bin", "/usr/local/bin/circusd")
	config.Set("docker:run-cmd:args", "/etc/circus/circus.ini")
	config.Set("docker:run-cmd:port", "8888")
	config.Set("docker:ssh:add-key-cmd", "/var/lib/tsuru/add-key")
	s.deployCmd = "/var/lib/tsuru/deploy"
	s.runBin = "/usr/local/bin/circusd"
	s.runArgs = "/etc/circus/circus.ini"
	s.port = "8888"
	var err error
	s.conn, err = db.Conn()
	c.Assert(err, gocheck.IsNil)
	fsystem = &ftesting.RecordingFs{}
	f, err := fsystem.Create(os.ExpandEnv("${HOME}/.ssh/id_rsa.pub"))
	c.Assert(err, gocheck.IsNil)
	f.Write([]byte("key-content"))
	f.Close()
}

func (s *S) TearDownSuite(c *gocheck.C) {
	s.conn.Collection(s.collName).Database.DropDatabase()
	fsystem = nil
}
