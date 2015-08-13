// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bs

import (
	"os"
	"strings"
	"testing"

	dtesting "github.com/fsouza/go-dockerclient/testing"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/auth/native"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"gopkg.in/check.v1"
)

func Test(t *testing.T) {
	check.TestingT(t)
}

var _ = check.Suite(&S{})

type S struct {
	p           *fakeDockerProvisioner
	server      *dtesting.DockerServer
	extraServer *dtesting.DockerServer
}

func (s *S) SetUpSuite(c *check.C) {
	config.Set("database:url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("database:name", "docker_provision_bs_tests")
	config.Set("admin-team", "admin")
	nativeScheme := auth.ManagedScheme(native.NativeScheme{})
	app.AuthScheme = nativeScheme
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
}

func (s *S) SetUpTest(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	dbtest.ClearAllCollections(conn.Apps().Database)
	s.server, err = dtesting.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	s.p, err = newFakeDockerProvisioner(s.server.URL())
	c.Assert(err, check.IsNil)
	os.Setenv("TSURU_TARGET", "http://localhost")
}

func (s *S) TearDownTest(c *check.C) {
	s.server.Stop()
	if s.extraServer != nil {
		s.extraServer.Stop()
		s.extraServer = nil
	}
	os.Unsetenv("TSURU_TARGET")
}

func (s *S) startMultipleServersCluster() (*fakeDockerProvisioner, error) {
	var err error
	s.extraServer, err = dtesting.NewServer("localhost:0", nil, nil)
	if err != nil {
		return nil, err
	}
	otherUrl := strings.Replace(s.extraServer.URL(), "127.0.0.1", "localhost", 1)
	return newFakeDockerProvisioner(s.server.URL(), otherUrl)
}
