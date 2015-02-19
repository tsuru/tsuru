// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package repositorytest

import (
	"testing"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/repository"
	"gopkg.in/check.v1"
)

func Test(t *testing.T) {
	check.TestingT(t)
}

var _ = check.Suite(&Suite{})

type Suite struct{}

func (Suite) SetUpTest(c *check.C) {
	Reset()
}

func (Suite) TestRegistration(c *check.C) {
	if old, err := config.Get("repo-manager"); err == nil {
		defer config.Set("repo-manager", old)
	} else {
		defer config.Unset("repo-manager")
	}
	config.Set("repo-manager", "fake")
	c.Check(repository.Manager().(*fakeManager), check.Equals, &manager)
}

func (Suite) TestManagerUser(c *check.C) {
	err := manager.CreateUser("gopher")
	c.Check(err, check.IsNil)
	c.Check(Users(), check.DeepEquals, []string{"gopher"})
	err = manager.CreateUser("gopher")
	c.Check(err, check.Equals, repository.ErrUserAlreadyExists)
	err = manager.RemoveUser("gopher")
	c.Check(err, check.IsNil)
	c.Check(Users(), check.HasLen, 0)
	err = manager.RemoveUser("gopher")
	c.Check(err, check.Equals, repository.ErrUserNotFound)
}

func (Suite) TestManagerRepository(c *check.C) {
	err := manager.CreateRepository("myrepo", nil)
	c.Check(err, check.IsNil)
	err = manager.CreateRepository("myrepo", nil)
	c.Check(err, check.Equals, repository.ErrRepositoryAlreadExists)
	repo, err := manager.GetRepository("myrepo")
	c.Check(err, check.IsNil)
	c.Check(repo.Name, check.Equals, "myrepo")
	c.Check(repo.ReadOnlyURL, check.Equals, "git://"+ServerHost+"/myrepo.git")
	c.Check(repo.ReadWriteURL, check.Equals, "git@"+ServerHost+":myrepo.git")
	err = manager.RemoveRepository("myrepo")
	c.Check(err, check.IsNil)
	_, err = manager.GetRepository("myrepo")
	c.Check(err, check.Equals, repository.ErrRepositoryNotFound)
}

func (Suite) TestManagerGrants(c *check.C) {
	err := manager.CreateUser("gopher")
	c.Check(err, check.IsNil)
	err = manager.CreateUser("gophera")
	c.Check(err, check.IsNil)
	err = manager.CreateUser("woot")
	c.Check(err, check.IsNil)
	err = manager.CreateRepository("myrepo", nil)
	c.Check(err, check.IsNil)
	err = manager.CreateRepository("kernel", []string{"woot"})
	c.Check(err, check.IsNil)
	err = manager.GrantAccess("myrepo", "gopher")
	c.Check(err, check.IsNil)
	err = manager.GrantAccess("myrepo", "gophera")
	c.Check(err, check.IsNil)
	grants, err := Granted("myrepo")
	c.Check(err, check.IsNil)
	c.Check(grants, check.DeepEquals, []string{"gopher", "gophera"})
	grants, err = Granted("kernel")
	c.Check(err, check.IsNil)
	c.Check(grants, check.DeepEquals, []string{"woot"})
	grants, err = Granted("kernell")
	c.Check(grants, check.IsNil)
	c.Check(err, check.Equals, repository.ErrRepositoryNotFound)
	err = manager.RevokeAccess("myrepo", "gopher")
	c.Check(err, check.IsNil)
	grants, err = Granted("myrepo")
	c.Check(err, check.IsNil)
	c.Check(grants, check.DeepEquals, []string{"gophera"})
	err = manager.RevokeAccess("myrepo", "gopher")
	c.Check(err, check.IsNil)
	err = manager.GrantAccess("watrepo", "gopher")
	c.Check(err, check.Equals, repository.ErrRepositoryNotFound)
	err = manager.RevokeAccess("watrepo", "gopher")
	c.Check(err, check.Equals, repository.ErrRepositoryNotFound)
	err = manager.GrantAccess("myrepo", "watuser")
	c.Check(err, check.Equals, repository.ErrUserNotFound)
	err = manager.RevokeAccess("myrepo", "watuser")
	c.Check(err, check.Equals, repository.ErrUserNotFound)
	err = manager.CreateRepository("somerepo", []string{"watuser"})
	c.Check(err, check.Equals, repository.ErrUserNotFound)
}

func (Suite) TestManagerKeys(c *check.C) {
	err := manager.CreateUser("gopher")
	c.Check(err, check.IsNil)
	err = manager.CreateUser("gophera")
	c.Check(err, check.IsNil)
	err = manager.AddKey("gopher", repository.Key{Name: "name", Body: "body"})
	c.Check(err, check.IsNil)
	err = manager.AddKey("gopher", repository.Key{Name: "name", Body: "other"})
	c.Check(err, check.Equals, repository.ErrKeyAlreadyExists)
	err = manager.AddKey("wateee", repository.Key{Name: "name", Body: "body"})
	c.Check(err, check.Equals, repository.ErrUserNotFound)
	keys, err := manager.ListKeys("gopher")
	c.Check(err, check.IsNil)
	c.Check(keys, check.DeepEquals, []repository.Key{{Name: "name", Body: "body"}})
	keys, err = manager.ListKeys("gophera")
	c.Check(err, check.IsNil)
	c.Check(keys, check.HasLen, 0)
	keys, err = manager.ListKeys("watuser")
	c.Check(keys, check.IsNil)
	c.Check(err, check.Equals, repository.ErrUserNotFound)
	err = manager.RemoveKey("gopher", repository.Key{Name: "name"})
	c.Check(err, check.IsNil)
	keys, err = manager.ListKeys("gopher")
	c.Check(err, check.IsNil)
	c.Check(keys, check.HasLen, 0)
	err = manager.RemoveKey("gophera", repository.Key{Name: "name"})
	c.Check(err, check.Equals, repository.ErrKeyNotFound)
	err = manager.RemoveKey("gopheraa", repository.Key{Name: "name"})
	c.Check(err, check.Equals, repository.ErrUserNotFound)
}

func (Suite) TestManagerDiff(c *check.C) {
	err := manager.CreateRepository("mycode", nil)
	c.Check(err, check.IsNil)
	diff, err := manager.Diff("mycode", "1.0", "2.0")
	c.Check(err, check.IsNil)
	c.Check(diff, check.Equals, "")
	_, err = manager.Diff("yourcode", "1.0", "2.0")
	c.Check(err, check.Equals, repository.ErrRepositoryNotFound)
}
