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
	c.Assert(repository.Manager().(*fakeManager), check.Equals, &manager)
}

func (Suite) TestManagerUser(c *check.C) {
	err := manager.CreateUser("gopher")
	c.Check(err, check.IsNil)
	c.Check(Users(), check.DeepEquals, []string{"gopher"})
	err = manager.CreateUser("gopher")
	c.Check(err.Error(), check.Equals, "user already exists")
	err = manager.RemoveUser("gopher")
	c.Check(err, check.IsNil)
	c.Check(Users(), check.HasLen, 0)
	err = manager.RemoveUser("gopher")
	c.Check(err.Error(), check.Equals, "user not found")
}

func (Suite) TestManagerRepository(c *check.C) {
	err := manager.CreateRepository("myrepo")
	c.Check(err, check.IsNil)
	err = manager.CreateRepository("myrepo")
	c.Check(err.Error(), check.Equals, "repository already exists")
	repo, err := manager.GetRepository("myrepo")
	c.Check(err, check.IsNil)
	c.Check(repo.Name, check.Equals, "myrepo")
	c.Check(repo.ReadOnlyURL, check.Equals, "git://"+ServerHost+"/myrepo.git")
	c.Check(repo.ReadWriteURL, check.Equals, "git@"+ServerHost+":myrepo.git")
	err = manager.RemoveRepository("myrepo")
	c.Check(err, check.IsNil)
	_, err = manager.GetRepository("myrepo")
	c.Check(err.Error(), check.Equals, "repository not found")
}

func (Suite) TestManagerGrants(c *check.C) {
	err := manager.CreateRepository("myrepo")
	c.Check(err, check.IsNil)
	err = manager.CreateRepository("kernel")
	c.Check(err, check.IsNil)
	err = manager.CreateUser("gopher")
	c.Check(err, check.IsNil)
	err = manager.CreateUser("gophera")
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
	c.Check(grants, check.HasLen, 0)
	grants, err = Granted("kernell")
	c.Check(grants, check.IsNil)
	c.Check(err.Error(), check.Equals, "repository not found")
	err = manager.RevokeAccess("myrepo", "gopher")
	c.Check(err, check.IsNil)
	grants, err = Granted("myrepo")
	c.Check(err, check.IsNil)
	c.Check(grants, check.DeepEquals, []string{"gophera"})
	err = manager.RevokeAccess("myrepo", "gopher")
	c.Check(err, check.IsNil)
	err = manager.GrantAccess("watrepo", "gopher")
	c.Check(err.Error(), check.Equals, "repository not found")
	err = manager.RevokeAccess("watrepo", "gopher")
	c.Check(err.Error(), check.Equals, "repository not found")
	err = manager.GrantAccess("myrepo", "watuser")
	c.Check(err.Error(), check.Equals, "user not found")
	err = manager.RevokeAccess("myrepo", "watuser")
	c.Check(err.Error(), check.Equals, "user not found")
}

func (Suite) TestManagerKeys(c *check.C) {
	err := manager.CreateUser("gopher")
	c.Check(err, check.IsNil)
	err = manager.CreateUser("gophera")
	c.Check(err, check.IsNil)
	err = manager.AddKey("gopher", repository.Key{Name: "name", Body: "body"})
	c.Check(err, check.IsNil)
	err = manager.AddKey("gopher", repository.Key{Name: "name", Body: "other"})
	c.Check(err.Error(), check.Equals, "user already have a key with this name")
	err = manager.AddKey("wateee", repository.Key{Name: "name", Body: "body"})
	c.Check(err.Error(), check.Equals, "user not found")
	keys, err := manager.ListKeys("gopher")
	c.Check(err, check.IsNil)
	c.Check(keys, check.DeepEquals, []repository.Key{{Name: "name", Body: "body"}})
	keys, err = manager.ListKeys("gophera")
	c.Check(err, check.IsNil)
	c.Check(keys, check.HasLen, 0)
	keys, err = manager.ListKeys("watuser")
	c.Check(keys, check.IsNil)
	c.Check(err.Error(), check.Equals, "user not found")
	err = manager.RemoveKey("gopher", repository.Key{Name: "name"})
	c.Check(err, check.IsNil)
	keys, err = manager.ListKeys("gopher")
	c.Check(err, check.IsNil)
	c.Check(keys, check.HasLen, 0)
	err = manager.RemoveKey("gophera", repository.Key{Name: "name"})
	c.Check(err.Error(), check.Equals, "key not found")
	err = manager.RemoveKey("gopheraa", repository.Key{Name: "name"})
	c.Check(err.Error(), check.Equals, "user not found")
}
