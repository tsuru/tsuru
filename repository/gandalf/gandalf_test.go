// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gandalf

import (
	"testing"

	"github.com/tsuru/config"
	"github.com/tsuru/gandalf/gandalftest"
	"github.com/tsuru/tsuru/repository"
	"gopkg.in/check.v1"
)

func Test(t *testing.T) {
	check.TestingT(t)
}

var _ = check.Suite(&GandalfSuite{})

type GandalfSuite struct {
	server *gandalftest.GandalfServer
}

func (s *GandalfSuite) SetUpSuite(c *check.C) {
	var err error
	s.server, err = gandalftest.NewServer("127.0.0.1:0")
	s.server.Host = "localhost"
	c.Assert(err, check.IsNil)
	config.Set("git:api-server", s.server.URL())
}

func (s *GandalfSuite) TearDownSuite(c *check.C) {
	s.server.Stop()
}

func (s *GandalfSuite) TearDownTest(c *check.C) {
	s.server.Reset()
}

func (s *GandalfSuite) TestCreateUser(c *check.C) {
	var manager gandalfManager
	err := manager.CreateUser("myself@tsuru.io")
	c.Assert(err, check.IsNil)
	users := s.server.Users()
	c.Assert(users, check.DeepEquals, []string{"myself@tsuru.io"})
}

func (s *GandalfSuite) TestRemoveUser(c *check.C) {
	var manager gandalfManager
	err := manager.CreateUser("myself@tsuru.io")
	c.Assert(err, check.IsNil)
	err = manager.CreateUser("theirself@tsuru.io")
	c.Assert(err, check.IsNil)
	err = manager.RemoveUser("myself@tsuru.io")
	c.Assert(err, check.IsNil)
	c.Assert(s.server.Users(), check.DeepEquals, []string{"theirself@tsuru.io"})
}

func (s *GandalfSuite) TestCreateRepository(c *check.C) {
	var manager gandalfManager
	err := manager.CreateRepository("myrepo")
	c.Assert(err, check.IsNil)
	repos := s.server.Repositories()
	c.Assert(repos, check.HasLen, 1)
	c.Assert(repos[0].Name, check.Equals, "myrepo")
	c.Assert(repos[0].Users, check.HasLen, 0)
	c.Assert(repos[0].IsPublic, check.Equals, true)
}

func (s *GandalfSuite) TestRemoveRepository(c *check.C) {
	var manager gandalfManager
	err := manager.CreateRepository("myrepo")
	c.Assert(err, check.IsNil)
	err = manager.CreateRepository("yourrepo")
	c.Assert(err, check.IsNil)
	err = manager.RemoveRepository("yourrepo")
	c.Assert(err, check.IsNil)
	repos := s.server.Repositories()
	c.Assert(repos, check.HasLen, 1)
	c.Assert(repos[0].Name, check.Equals, "myrepo")
}

func (s *GandalfSuite) TestGetRepository(c *check.C) {
	var manager gandalfManager
	err := manager.CreateRepository("myrepo")
	c.Assert(err, check.IsNil)
	repo, err := manager.GetRepository("myrepo")
	c.Assert(err, check.IsNil)
	c.Assert(repo.Name, check.Equals, "myrepo")
	c.Assert(repo.ReadOnlyURL, check.Equals, "git://localhost/myrepo.git")
	c.Assert(repo.ReadWriteURL, check.Equals, "git@localhost:myrepo.git")
}

func (s *GandalfSuite) TestGrantAccess(c *check.C) {
	var manager gandalfManager
	err := manager.CreateRepository("myrepo")
	c.Assert(err, check.IsNil)
	err = manager.CreateUser("myuser")
	c.Assert(err, check.IsNil)
	err = manager.GrantAccess("myrepo", "myuser")
	c.Assert(err, check.IsNil)
	grants := s.server.Grants()
	expected := map[string][]string{"myrepo": {"myuser"}}
	c.Assert(grants, check.DeepEquals, expected)
}

func (s *GandalfSuite) TestRevokeAccess(c *check.C) {
	var manager gandalfManager
	err := manager.CreateRepository("myrepo")
	c.Assert(err, check.IsNil)
	err = manager.CreateUser("myuser")
	c.Assert(err, check.IsNil)
	err = manager.CreateUser("otheruser")
	c.Assert(err, check.IsNil)
	err = manager.GrantAccess("myrepo", "myuser")
	c.Assert(err, check.IsNil)
	err = manager.GrantAccess("myrepo", "otheruser")
	c.Assert(err, check.IsNil)
	err = manager.RevokeAccess("myrepo", "myuser")
	grants := s.server.Grants()
	expected := map[string][]string{"myrepo": {"otheruser"}}
	c.Assert(grants, check.DeepEquals, expected)
}

func (s *GandalfSuite) TestAddKey(c *check.C) {
	var manager gandalfManager
	err := manager.CreateUser("myuser")
	c.Assert(err, check.IsNil)
	err = manager.AddKey("myuser", repository.Key{Name: "mykey", Body: publicKey})
	c.Assert(err, check.IsNil)
	keys, err := s.server.Keys("myuser")
	c.Assert(err, check.IsNil)
	expected := map[string]string{"mykey": publicKey}
	c.Assert(keys, check.DeepEquals, expected)
}

func (s *GandalfSuite) TestRemoveKey(c *check.C) {
	var manager gandalfManager
	err := manager.CreateUser("myuser")
	c.Assert(err, check.IsNil)
	err = manager.AddKey("myuser", repository.Key{Name: "mykey", Body: publicKey})
	c.Assert(err, check.IsNil)
	err = manager.RemoveKey("myuser", repository.Key{Name: "mykey"})
	keys, err := s.server.Keys("myuser")
	c.Assert(err, check.IsNil)
	c.Assert(keys, check.HasLen, 0)
}

func (s *GandalfSuite) TestListKeys(c *check.C) {
	var manager gandalfManager
	err := manager.CreateUser("myuser")
	c.Assert(err, check.IsNil)
	err = manager.AddKey("myuser", repository.Key{Name: "mykey", Body: publicKey})
	c.Assert(err, check.IsNil)
	keys, err := manager.ListKeys("myuser")
	c.Assert(err, check.IsNil)
	expected := []repository.Key{{Name: "mykey", Body: publicKey}}
	c.Assert(keys, check.DeepEquals, expected)
}

const publicKey = "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQDD91CO+YIU6nIb+l+JewPMLbUB9IZx4g6IUuqyLbmCi+8DNliEjE/KWUISPlkPWoDK4ibEY/gZPLPRMT3acA+2cAf3uApBwegvDgtDv1lgtTbkMc8QJaT044Vg+JtVDFraXU4T8fn/apVMMXro0Kr/DaLzUsxSigGrCIRyT1vkMCnya8oaQHu1Qa/wnOjd6tZzvzIsxJirAbQvzlLOb89c7LTPhUByySTQmgSnoNR6ZdPpjDwnaQgyAjbsPKjhkQ1AkcxOxBi0GwwSCO7aZ+T3F/mJ1bUhEE5BMh+vO3HQ3gGkc1xeQW4H7ZL33sJkP0Tb9zslaE1lT+fuOi7NBUK5 f@somewhere"
