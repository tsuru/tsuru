// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"errors"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/repository/repositorytest"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

type PlatformSuite struct {
	provisioner *provisiontest.FakeProvisioner
}

var _ = check.Suite(&PlatformSuite{})

func (s *PlatformSuite) SetUpSuite(c *check.C) {
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "platform_tests")
	s.provisioner = provisiontest.NewFakeProvisioner()
	Provisioner = s.provisioner
}

func (s *PlatformSuite) TearDownSuite(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	dbtest.ClearAllCollections(conn.Apps().Database)
	conn.Close()
}

func (s *PlatformSuite) SetUpTest(c *check.C) {
	repositorytest.Reset()
}

func (s *PlatformSuite) TestPlatforms(c *check.C) {
	want := []Platform{
		{Name: "dea"},
		{Name: "pecuniae"},
		{Name: "money"},
		{Name: "raise"},
		{Name: "glass"},
	}
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	for _, p := range want {
		conn.Platforms().Insert(p)
		defer conn.Platforms().Remove(p)
	}
	got, err := Platforms()
	c.Assert(err, check.IsNil)
	c.Assert(got, check.DeepEquals, want)
}

func (s *PlatformSuite) TestPlatformsEmpty(c *check.C) {
	got, err := Platforms()
	c.Assert(err, check.IsNil)
	c.Assert(got, check.HasLen, 0)
}

func (s *PlatformSuite) TestGetPlatform(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	p := Platform{Name: "dea"}
	conn.Platforms().Insert(p)
	defer conn.Platforms().Remove(p)
	got, err := getPlatform(p.Name)
	c.Assert(err, check.IsNil)
	c.Assert(*got, check.DeepEquals, p)
	got, err = getPlatform("WAT")
	c.Assert(got, check.IsNil)
	_, ok := err.(InvalidPlatformError)
	c.Assert(ok, check.Equals, true)
}

func (s *PlatformSuite) TestPlatformAdd(c *check.C) {
	provisioner := provisiontest.ExtensibleFakeProvisioner{
		FakeProvisioner: provisiontest.NewFakeProvisioner(),
	}
	Provisioner = &provisioner
	defer func() {
		Provisioner = s.provisioner
	}()
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	name := "test_platform_add"
	args := make(map[string]string)
	args["dockerfile"] = "http://localhost/Dockerfile"
	err = PlatformAdd(name, args, nil)
	defer conn.Platforms().Remove(bson.M{"_id": name})
	c.Assert(err, check.IsNil)
	platform := provisioner.GetPlatform(name)
	c.Assert(platform.Name, check.Equals, name)
	c.Assert(platform.Args, check.DeepEquals, args)
	c.Assert(platform.Version, check.Equals, 1)
}

func (s *PlatformSuite) TestPlatformAddDuplicate(c *check.C) {
	provisioner := provisiontest.ExtensibleFakeProvisioner{
		FakeProvisioner: provisiontest.NewFakeProvisioner(),
	}
	Provisioner = &provisioner
	defer func() {
		Provisioner = s.provisioner
	}()
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	name := "test_platform_add"
	args := make(map[string]string)
	args["dockerfile"] = "http://localhost/Dockerfile"
	err = PlatformAdd(name, args, nil)
	defer conn.Platforms().Remove(bson.M{"_id": name})
	c.Assert(err, check.IsNil)
	provisioner.PlatformRemove(name)
	err = PlatformAdd(name, args, nil)
	_, ok := err.(DuplicatePlatformError)
	c.Assert(ok, check.Equals, true)
}

func (s *PlatformSuite) TestPlatformAddWithProvisionerError(c *check.C) {
	provisioner := provisiontest.ExtensibleFakeProvisioner{
		FakeProvisioner: provisiontest.NewFakeProvisioner(),
	}
	provisioner.PrepareFailure("PlatformAdd", errors.New("build error"))
	Provisioner = &provisioner
	defer func() {
		Provisioner = s.provisioner
	}()
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	name := "test_platform_add"
	args := make(map[string]string)
	args["dockerfile"] = "http://localhost/Dockerfile"
	err = PlatformAdd(name, args, nil)
	defer conn.Platforms().Remove(bson.M{"_id": name})
	c.Assert(err, check.NotNil)
	count, err := conn.Platforms().FindId(name).Count()
	c.Assert(err, check.IsNil)
	c.Assert(count, check.Equals, 0)
}

func (s *PlatformSuite) TestPlatformAddNotExtensibleProvisioner(c *check.C) {
	err := PlatformAdd("python", nil, nil)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Provisioner is not extensible")
}

func (s *PlatformSuite) TestPlatformAddWithoutName(c *check.C) {
	provisioner := provisiontest.ExtensibleFakeProvisioner{
		FakeProvisioner: provisiontest.NewFakeProvisioner(),
	}
	Provisioner = &provisioner
	defer func() {
		Provisioner = s.provisioner
	}()
	err := PlatformAdd("", nil, nil)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Platform name is required.")
}

func (s *PlatformSuite) TestPlatformUpdate(c *check.C) {
	provisioner := provisiontest.ExtensibleFakeProvisioner{
		FakeProvisioner: provisiontest.NewFakeProvisioner(),
	}
	Provisioner = &provisioner
	defer func() {
		Provisioner = s.provisioner
	}()
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	name := "test_platform_update"
	args := make(map[string]string)
	args["dockerfile"] = "http://localhost/Dockerfile"
	err = PlatformUpdate(name, args, nil)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Platform doesn't exist.")
	err = PlatformAdd(name, nil, nil)
	c.Assert(err, check.IsNil)
	defer conn.Platforms().Remove(bson.M{"_id": name})
	err = PlatformUpdate(name, args, nil)
	c.Assert(err, check.IsNil)
}

func (s *PlatformSuite) TestPlatformUpdateWithoutName(c *check.C) {
	provisioner := provisiontest.ExtensibleFakeProvisioner{
		FakeProvisioner: provisiontest.NewFakeProvisioner(),
	}
	Provisioner = &provisioner
	defer func() {
		Provisioner = s.provisioner
	}()
	err := PlatformUpdate("", nil, nil)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Platform name is required.")
}

func (s *PlatformSuite) TestPlatformUpdateShouldSetUpdatePlatformFlagOnApps(c *check.C) {
	provisioner := provisiontest.ExtensibleFakeProvisioner{
		FakeProvisioner: provisiontest.NewFakeProvisioner(),
	}
	Provisioner = &provisioner
	defer func() {
		Provisioner = s.provisioner
	}()
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	name := "test_platform_update"
	args := make(map[string]string)
	args["dockerfile"] = "http://localhost/Dockerfile"
	err = PlatformAdd(name, nil, nil)
	c.Assert(err, check.IsNil)
	defer conn.Platforms().Remove(bson.M{"_id": name})
	appName := "test_app"
	app := App{
		Name:     appName,
		Platform: name,
	}
	err = conn.Apps().Insert(app)
	c.Assert(err, check.IsNil)
	defer conn.Apps().Remove(bson.M{"_id": appName})
	err = PlatformUpdate(name, args, nil)
	c.Assert(err, check.IsNil)
	a, err := GetByName(appName)
	c.Assert(err, check.IsNil)
	c.Assert(a.UpdatePlatform, check.Equals, true)
}

func (s *PlatformSuite) TestPlatformRemove(c *check.C) {
	provisioner := provisiontest.ExtensibleFakeProvisioner{
		FakeProvisioner: provisiontest.NewFakeProvisioner(),
	}
	Provisioner = &provisioner
	defer func() {
		Provisioner = s.provisioner
	}()
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	err = PlatformRemove("platform_dont_exists")
	c.Assert(err, check.NotNil)
	name := "test_platform_update"
	args := make(map[string]string)
	args["dockerfile"] = "http://localhost/Dockerfile"
	err = PlatformAdd(name, nil, nil)
	c.Assert(err, check.IsNil)
	defer conn.Platforms().Remove(bson.M{"_id": name})
	err = PlatformRemove(name)
	c.Assert(err, check.IsNil)
	count, err := conn.Platforms().Find(bson.M{"_id": name}).Count()
	c.Assert(err, check.IsNil)
	c.Assert(count, check.Equals, 0)
	err = PlatformRemove("")
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Platform name is required!")
}

func (s *PlatformSuite) TestPlatformWithAppsCantBeRemoved(c *check.C) {
	provisioner := provisiontest.ExtensibleFakeProvisioner{
		FakeProvisioner: provisiontest.NewFakeProvisioner(),
	}
	Provisioner = &provisioner
	defer func() {
		Provisioner = s.provisioner
	}()
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	name := "test_platform_update"
	args := make(map[string]string)
	args["dockerfile"] = "http://localhost/Dockerfile"
	err = PlatformAdd(name, nil, nil)
	c.Assert(err, check.IsNil)
	defer conn.Platforms().Remove(bson.M{"_id": name})
	appName := "test_another_app"
	app := App{
		Name:     appName,
		Platform: name,
	}
	err = conn.Apps().Insert(app)
	c.Assert(err, check.IsNil)
	defer conn.Apps().Remove(bson.M{"_id": appName})
	err = PlatformRemove(name)
	c.Assert(err, check.NotNil)
}

func (s *PlatformSuite) TestPlatformRemoveAlwaysRemoveFromDB(c *check.C) {
	provisioner := provisiontest.ExtensibleFakeProvisioner{
		FakeProvisioner: provisiontest.NewFakeProvisioner(),
	}
	Provisioner = &provisioner
	defer func() {
		Provisioner = s.provisioner
	}()
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	err = PlatformRemove("platform_dont_exists")
	c.Assert(err, check.NotNil)
	name := "test_platform_update"
	args := make(map[string]string)
	args["dockerfile"] = "http://localhost/Dockerfile"
	err = PlatformAdd(name, nil, nil)
	c.Assert(err, check.IsNil)
	provisioner.PlatformRemove(name)
	defer conn.Platforms().Remove(bson.M{"_id": name})
	err = PlatformRemove(name)
	c.Assert(err, check.IsNil)
	count, err := conn.Platforms().Find(bson.M{"_id": name}).Count()
	c.Assert(err, check.IsNil)
	c.Assert(count, check.Equals, 0)
}
