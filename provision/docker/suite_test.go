// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"github.com/globocom/config"
	"github.com/globocom/tsuru/db"
	"launchpad.net/gocheck"
	"testing"
)

func Test(t *testing.T) { gocheck.TestingT(t) }

type S struct {
	collName      string
	imageCollName string
	conn          *db.Storage
	gitHost       string
	repoNamespace string
}

var _ = gocheck.Suite(&S{})

func (s *S) SetUpSuite(c *gocheck.C) {
	s.collName = "docker_unit"
	s.imageCollName = "docker_image"
	s.gitHost = "my.gandalf.com"
	s.repoNamespace = "tsuru"
	config.Set("git:host", s.gitHost)
	config.Set("docker:repository-namespace", s.repoNamespace)
	config.Set("docker:binary", "docker")
	config.Set("docker:router", "fake")
	config.Set("docker:collection", s.collName)
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "juju_provision_tests_s")
	config.Set("docker:deploy-cmd", "/var/lib/tsuru/deploy")
	config.Set("docker:run-cmd:bin", "/usr/local/bin/circusd")
	config.Set("docker:run-cmd:args", "/etc/circus/circus.ini")
	config.Set("docker:run-cmd:port", "8888")
	var err error
	s.conn, err = db.Conn()
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TearDownSuite(c *gocheck.C) {
	s.conn.Collection(s.collName).Database.DropDatabase()
}
