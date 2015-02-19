// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gandalf

import (
	"net/http"
	"testing"

	"github.com/tsuru/config"
	"github.com/tsuru/gandalf/gandalftest"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/hc"
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
	config.Set("database:name", "repository_gandalf_test")
}

func (s *GandalfSuite) TearDownSuite(c *check.C) {
	s.server.Stop()
}

func (s *GandalfSuite) TearDownTest(c *check.C) {
	s.server.Reset()
}

func (s *GandalfSuite) TestHealthCheck(c *check.C) {
	err := healthCheck()
	c.Assert(err, check.IsNil)
}

func (s *GandalfSuite) TestHealthCheckStatusFailure(c *check.C) {
	s.server.PrepareFailure(gandalftest.Failure{Path: "/healthcheck", Method: "GET", Response: "epic fail"})
	err := healthCheck()
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "epic fail\n")
}

func (s *GandalfSuite) TestHealthCheckContentFailure(c *check.C) {
	s.server.PrepareFailure(gandalftest.Failure{Code: http.StatusOK, Path: "/healthcheck", Method: "GET", Response: "epic fail"})
	err := healthCheck()
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "unexpected status - epic fail\n")
}

func (s *GandalfSuite) TestHealthCheckDisabled(c *check.C) {
	old, err := config.Get("git:api-server")
	c.Assert(err, check.IsNil)
	defer config.Set("git:api-server", old)
	config.Unset("git:api-server")
	err = healthCheck()
	c.Assert(err, check.Equals, hc.ErrDisabledComponent)
}

func (s *GandalfSuite) TestInitialize(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	defer dbtest.ClearAllCollections(conn.Apps().Database)
	var manager gandalfManager
	user1 := auth.User{Email: "user1@company.com"}
	user2 := auth.User{Email: "user2@company.com"}
	err = conn.Users().Insert(user1, user2)
	c.Assert(err, check.IsNil)
	err = manager.CreateUser(user1.Email)
	c.Assert(err, check.IsNil)
	team := auth.Team{Name: "superteam", Users: []string{user1.Email, user2.Email}}
	err = conn.Teams().Insert(team)
	c.Assert(err, check.IsNil)
	app1 := app.App{Name: "myapp", Teams: []string{team.Name}}
	app2 := app.App{Name: "yourapp", Teams: []string{team.Name}}
	app3 := app.App{Name: "hisapp", Teams: []string{team.Name}}
	err = conn.Apps().Insert(app1, app2, app3)
	c.Assert(err, check.IsNil)
	err = manager.CreateRepository(app2.Name, []string{user1.Email})
	c.Assert(err, check.IsNil)
	err = manager.Initialize()
	c.Assert(err, check.IsNil)
	c.Assert(s.server.Users(), check.DeepEquals, []string{user1.Email, user2.Email})
	expectedRepos := []gandalftest.Repository{
		{
			Name:         "yourapp",
			Users:        []string{user1.Email, user2.Email},
			ReadOnlyURL:  "git://localhost/yourapp.git",
			ReadWriteURL: "git@localhost:yourapp.git",
			IsPublic:     true,
		},
		{
			Name:         "myapp",
			Users:        []string{user1.Email, user2.Email},
			ReadOnlyURL:  "git://localhost/myapp.git",
			ReadWriteURL: "git@localhost:myapp.git",
			IsPublic:     true,
		},
		{
			Name:         "hisapp",
			Users:        []string{user1.Email, user2.Email},
			ReadOnlyURL:  "git://localhost/hisapp.git",
			ReadWriteURL: "git@localhost:hisapp.git",
			IsPublic:     true,
		},
	}
	repositories := s.server.Repositories()
	for i, repo := range repositories {
		repo.Diffs = nil
		repositories[i] = repo
	}
	c.Assert(repositories, check.DeepEquals, expectedRepos)
}

func (s *GandalfSuite) TestCreateUser(c *check.C) {
	var manager gandalfManager
	err := manager.CreateUser("myself@tsuru.io")
	c.Assert(err, check.IsNil)
	users := s.server.Users()
	c.Assert(users, check.DeepEquals, []string{"myself@tsuru.io"})
}

func (s *GandalfSuite) TestCreateUserAlreadyExists(c *check.C) {
	var manager gandalfManager
	err := manager.CreateUser("myself@tsuru.io")
	c.Assert(err, check.IsNil)
	err = manager.CreateUser("myself@tsuru.io")
	c.Assert(err, check.Equals, repository.ErrUserAlreadyExists)
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

func (s *GandalfSuite) TestRemoveUserNotFound(c *check.C) {
	var manager gandalfManager
	err := manager.RemoveUser("myself@tsuru.io")
	c.Assert(err, check.Equals, repository.ErrUserNotFound)
}

func (s *GandalfSuite) TestCreateRepository(c *check.C) {
	var manager gandalfManager
	err := manager.CreateUser("user1")
	c.Assert(err, check.IsNil)
	err = manager.CreateUser("user2")
	c.Assert(err, check.IsNil)
	err = manager.CreateRepository("myrepo", []string{"user1", "user2"})
	c.Assert(err, check.IsNil)
	repos := s.server.Repositories()
	c.Assert(repos, check.HasLen, 1)
	c.Assert(repos[0].Name, check.Equals, "myrepo")
	c.Assert(repos[0].Users, check.DeepEquals, []string{"user1", "user2"})
	c.Assert(repos[0].IsPublic, check.Equals, true)
}

func (s *GandalfSuite) TestCreateRepositoryDuplicate(c *check.C) {
	var manager gandalfManager
	err := manager.CreateUser("user1")
	c.Assert(err, check.IsNil)
	err = manager.CreateRepository("myrepo", []string{"user1"})
	c.Assert(err, check.IsNil)
	err = manager.CreateRepository("myrepo", []string{"user1"})
	c.Assert(err, check.Equals, repository.ErrRepositoryAlreadExists)
}

func (s *GandalfSuite) TestRemoveRepository(c *check.C) {
	var manager gandalfManager
	err := manager.CreateUser("user1")
	c.Assert(err, check.IsNil)
	err = manager.CreateRepository("myrepo", []string{"user1"})
	c.Assert(err, check.IsNil)
	err = manager.CreateRepository("yourrepo", []string{"user1"})
	c.Assert(err, check.IsNil)
	err = manager.RemoveRepository("yourrepo")
	c.Assert(err, check.IsNil)
	repos := s.server.Repositories()
	c.Assert(repos, check.HasLen, 1)
	c.Assert(repos[0].Name, check.Equals, "myrepo")
}

func (s *GandalfSuite) TestRemoveRepositoryNotFound(c *check.C) {
	var manager gandalfManager
	err := manager.RemoveRepository("myrepo")
	c.Assert(err, check.Equals, repository.ErrRepositoryNotFound)
}

func (s *GandalfSuite) TestGetRepository(c *check.C) {
	var manager gandalfManager
	err := manager.CreateUser("user1")
	c.Assert(err, check.IsNil)
	err = manager.CreateRepository("myrepo", []string{"user1"})
	c.Assert(err, check.IsNil)
	repo, err := manager.GetRepository("myrepo")
	c.Assert(err, check.IsNil)
	c.Assert(repo.Name, check.Equals, "myrepo")
	c.Assert(repo.ReadOnlyURL, check.Equals, "git://localhost/myrepo.git")
	c.Assert(repo.ReadWriteURL, check.Equals, "git@localhost:myrepo.git")
}

func (s *GandalfSuite) TestGetRepositoryNotFound(c *check.C) {
	var manager gandalfManager
	_, err := manager.GetRepository("myrepo")
	c.Assert(err, check.Equals, repository.ErrRepositoryNotFound)
}

func (s *GandalfSuite) TestGrantAccess(c *check.C) {
	var manager gandalfManager
	err := manager.CreateUser("user1")
	c.Assert(err, check.IsNil)
	err = manager.CreateRepository("myrepo", []string{"user1"})
	c.Assert(err, check.IsNil)
	err = manager.CreateUser("myuser")
	c.Assert(err, check.IsNil)
	err = manager.GrantAccess("myrepo", "myuser")
	c.Assert(err, check.IsNil)
	grants := s.server.Grants()
	expected := map[string][]string{"myrepo": {"user1", "myuser"}}
	c.Assert(grants, check.DeepEquals, expected)
}

func (s *GandalfSuite) TestRevokeAccess(c *check.C) {
	var manager gandalfManager
	err := manager.CreateUser("user1")
	c.Assert(err, check.IsNil)
	err = manager.CreateRepository("myrepo", []string{"user1"})
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
	expected := map[string][]string{"myrepo": {"user1", "otheruser"}}
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

func (s *GandalfSuite) TestDiff(c *check.C) {
	var manager gandalfManager
	err := manager.CreateUser("user1")
	c.Assert(err, check.IsNil)
	err = manager.CreateRepository("myrepo", []string{"user1"})
	c.Assert(err, check.IsNil)
	s.server.PrepareDiff("myrepo", "some diff")
	diff, err := manager.Diff("myrepo", "10", "11")
	c.Assert(err, check.IsNil)
	c.Assert(diff, check.Equals, "some diff")
}

const publicKey = "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQDD91CO+YIU6nIb+l+JewPMLbUB9IZx4g6IUuqyLbmCi+8DNliEjE/KWUISPlkPWoDK4ibEY/gZPLPRMT3acA+2cAf3uApBwegvDgtDv1lgtTbkMc8QJaT044Vg+JtVDFraXU4T8fn/apVMMXro0Kr/DaLzUsxSigGrCIRyT1vkMCnya8oaQHu1Qa/wnOjd6tZzvzIsxJirAbQvzlLOb89c7LTPhUByySTQmgSnoNR6ZdPpjDwnaQgyAjbsPKjhkQ1AkcxOxBi0GwwSCO7aZ+T3F/mJ1bUhEE5BMh+vO3HQ3gGkc1xeQW4H7ZL33sJkP0Tb9zslaE1lT+fuOi7NBUK5 f@somewhere"
