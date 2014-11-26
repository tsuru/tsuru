// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"errors"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/testing"
	"gopkg.in/mgo.v2/bson"
	"launchpad.net/gocheck"
)

type PlatformSuite struct {
	provisioner *testing.FakeProvisioner
}

var _ = gocheck.Suite(&PlatformSuite{})

func (s *PlatformSuite) SetUpSuite(c *gocheck.C) {
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "platform_tests")
	s.provisioner = testing.NewFakeProvisioner()
	Provisioner = s.provisioner
}

func (s *PlatformSuite) TearDownSuite(c *gocheck.C) {
	conn, err := db.Conn()
	c.Assert(err, gocheck.IsNil)
	testing.ClearAllCollections(conn.Apps().Database)
	conn.Close()
}

func (s *PlatformSuite) TestPlatforms(c *gocheck.C) {
	want := []Platform{
		{Name: "dea"},
		{Name: "pecuniae"},
		{Name: "money"},
		{Name: "raise"},
		{Name: "glass"},
	}
	conn, err := db.Conn()
	c.Assert(err, gocheck.IsNil)
	defer conn.Close()
	for _, p := range want {
		conn.Platforms().Insert(p)
		defer conn.Platforms().Remove(p)
	}
	got, err := Platforms()
	c.Assert(err, gocheck.IsNil)
	c.Assert(got, gocheck.DeepEquals, want)
}

func (s *PlatformSuite) TestPlatformsEmpty(c *gocheck.C) {
	got, err := Platforms()
	c.Assert(err, gocheck.IsNil)
	c.Assert(got, gocheck.HasLen, 0)
}

func (s *PlatformSuite) TestGetPlatform(c *gocheck.C) {
	conn, err := db.Conn()
	c.Assert(err, gocheck.IsNil)
	defer conn.Close()
	p := Platform{Name: "dea"}
	conn.Platforms().Insert(p)
	defer conn.Platforms().Remove(p)
	got, err := getPlatform(p.Name)
	c.Assert(err, gocheck.IsNil)
	c.Assert(*got, gocheck.DeepEquals, p)
	got, err = getPlatform("WAT")
	c.Assert(got, gocheck.IsNil)
	_, ok := err.(InvalidPlatformError)
	c.Assert(ok, gocheck.Equals, true)
}

func (s *PlatformSuite) TestPlatformAdd(c *gocheck.C) {
	provisioner := testing.ExtensibleFakeProvisioner{
		FakeProvisioner: testing.NewFakeProvisioner(),
	}
	Provisioner = &provisioner
	defer func() {
		Provisioner = s.provisioner
	}()
	conn, err := db.Conn()
	c.Assert(err, gocheck.IsNil)
	defer conn.Close()
	name := "test_platform_add"
	args := make(map[string]string)
	args["dockerfile"] = "http://localhost/Dockerfile"
	err = PlatformAdd(name, args, nil)
	defer conn.Platforms().Remove(bson.M{"_id": name})
	c.Assert(err, gocheck.IsNil)
	platform := provisioner.GetPlatform(name)
	c.Assert(platform.Name, gocheck.Equals, name)
	c.Assert(platform.Args, gocheck.DeepEquals, args)
	c.Assert(platform.Version, gocheck.Equals, 1)
}

func (s *PlatformSuite) TestPlatformAddDuplicate(c *gocheck.C) {
	provisioner := testing.ExtensibleFakeProvisioner{
		FakeProvisioner: testing.NewFakeProvisioner(),
	}
	Provisioner = &provisioner
	defer func() {
		Provisioner = s.provisioner
	}()
	conn, err := db.Conn()
	c.Assert(err, gocheck.IsNil)
	defer conn.Close()
	name := "test_platform_add"
	args := make(map[string]string)
	args["dockerfile"] = "http://localhost/Dockerfile"
	err = PlatformAdd(name, args, nil)
	defer conn.Platforms().Remove(bson.M{"_id": name})
	c.Assert(err, gocheck.IsNil)
	provisioner.PlatformRemove(name)
	err = PlatformAdd(name, args, nil)
	_, ok := err.(DuplicatePlatformError)
	c.Assert(ok, gocheck.Equals, true)
}

func (s *PlatformSuite) TestPlatformAddWithProvisionerError(c *gocheck.C) {
	provisioner := testing.ExtensibleFakeProvisioner{
		FakeProvisioner: testing.NewFakeProvisioner(),
	}
	provisioner.PrepareFailure("PlatformAdd", errors.New("build error"))
	Provisioner = &provisioner
	defer func() {
		Provisioner = s.provisioner
	}()
	conn, err := db.Conn()
	c.Assert(err, gocheck.IsNil)
	defer conn.Close()
	name := "test_platform_add"
	args := make(map[string]string)
	args["dockerfile"] = "http://localhost/Dockerfile"
	err = PlatformAdd(name, args, nil)
	defer conn.Platforms().Remove(bson.M{"_id": name})
	c.Assert(err, gocheck.NotNil)
	count, err := conn.Platforms().FindId(name).Count()
	c.Assert(err, gocheck.IsNil)
	c.Assert(count, gocheck.Equals, 0)
}

func (s *PlatformSuite) TestPlatformAddNotExtensibleProvisioner(c *gocheck.C) {
	err := PlatformAdd("python", nil, nil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "Provisioner is not extensible")
}

func (s *PlatformSuite) TestPlatformAddWithoutName(c *gocheck.C) {
	provisioner := testing.ExtensibleFakeProvisioner{
		FakeProvisioner: testing.NewFakeProvisioner(),
	}
	Provisioner = &provisioner
	defer func() {
		Provisioner = s.provisioner
	}()
	err := PlatformAdd("", nil, nil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "Platform name is required.")
}

func (s *PlatformSuite) TestPlatformUpdate(c *gocheck.C) {
	provisioner := testing.ExtensibleFakeProvisioner{
		FakeProvisioner: testing.NewFakeProvisioner(),
	}
	Provisioner = &provisioner
	defer func() {
		Provisioner = s.provisioner
	}()
	conn, err := db.Conn()
	c.Assert(err, gocheck.IsNil)
	defer conn.Close()
	name := "test_platform_update"
	args := make(map[string]string)
	args["dockerfile"] = "http://localhost/Dockerfile"
	err = PlatformUpdate(name, args, nil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "Platform doesn't exist.")
	err = PlatformAdd(name, nil, nil)
	c.Assert(err, gocheck.IsNil)
	defer conn.Platforms().Remove(bson.M{"_id": name})
	err = PlatformUpdate(name, args, nil)
	c.Assert(err, gocheck.IsNil)
}

func (s *PlatformSuite) TestPlatformUpdateWithoutName(c *gocheck.C) {
	provisioner := testing.ExtensibleFakeProvisioner{
		FakeProvisioner: testing.NewFakeProvisioner(),
	}
	Provisioner = &provisioner
	defer func() {
		Provisioner = s.provisioner
	}()
	err := PlatformUpdate("", nil, nil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "Platform name is required.")
}

func (s *PlatformSuite) TestPlatformUpdateShouldSetUpdatePlatformFlagOnApps(c *gocheck.C) {
	provisioner := testing.ExtensibleFakeProvisioner{
		FakeProvisioner: testing.NewFakeProvisioner(),
	}
	Provisioner = &provisioner
	defer func() {
		Provisioner = s.provisioner
	}()
	conn, err := db.Conn()
	c.Assert(err, gocheck.IsNil)
	defer conn.Close()
	name := "test_platform_update"
	args := make(map[string]string)
	args["dockerfile"] = "http://localhost/Dockerfile"
	err = PlatformAdd(name, nil, nil)
	c.Assert(err, gocheck.IsNil)
	defer conn.Platforms().Remove(bson.M{"_id": name})
	appName := "test_app"
	app := App{
		Name:     appName,
		Platform: name,
	}
	err = conn.Apps().Insert(app)
	c.Assert(err, gocheck.IsNil)
	defer conn.Apps().Remove(bson.M{"_id": appName})
	err = PlatformUpdate(name, args, nil)
	c.Assert(err, gocheck.IsNil)
	a, err := GetByName(appName)
	c.Assert(err, gocheck.IsNil)
	c.Assert(a.UpdatePlatform, gocheck.Equals, true)
}

func (s *PlatformSuite) TestPlatformRemove(c *gocheck.C) {
	provisioner := testing.ExtensibleFakeProvisioner{
		FakeProvisioner: testing.NewFakeProvisioner(),
	}
	Provisioner = &provisioner
	defer func() {
		Provisioner = s.provisioner
	}()
	conn, err := db.Conn()
	c.Assert(err, gocheck.IsNil)
	defer conn.Close()
	err = PlatformRemove("platform_dont_exists")
	c.Assert(err, gocheck.NotNil)
	name := "test_platform_update"
	args := make(map[string]string)
	args["dockerfile"] = "http://localhost/Dockerfile"
	err = PlatformAdd(name, nil, nil)
	c.Assert(err, gocheck.IsNil)
	defer conn.Platforms().Remove(bson.M{"_id": name})
	err = PlatformRemove(name)
	c.Assert(err, gocheck.IsNil)
	count, err := conn.Platforms().Find(bson.M{"_id": name}).Count()
	c.Assert(err, gocheck.IsNil)
	c.Assert(count, gocheck.Equals, 0)
	err = PlatformRemove("")
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "Platform name is required!")
}

func (s *PlatformSuite) TestPlatformWithAppsCantBeRemoved(c *gocheck.C) {
	provisioner := testing.ExtensibleFakeProvisioner{
		FakeProvisioner: testing.NewFakeProvisioner(),
	}
	Provisioner = &provisioner
	defer func() {
		Provisioner = s.provisioner
	}()
	conn, err := db.Conn()
	c.Assert(err, gocheck.IsNil)
	defer conn.Close()
	name := "test_platform_update"
	args := make(map[string]string)
	args["dockerfile"] = "http://localhost/Dockerfile"
	err = PlatformAdd(name, nil, nil)
	c.Assert(err, gocheck.IsNil)
	defer conn.Platforms().Remove(bson.M{"_id": name})
	appName := "test_another_app"
	app := App{
		Name:     appName,
		Platform: name,
	}
	err = conn.Apps().Insert(app)
	c.Assert(err, gocheck.IsNil)
	defer conn.Apps().Remove(bson.M{"_id": appName})
	err = PlatformRemove(name)
	c.Assert(err, gocheck.NotNil)
}

func (s *PlatformSuite) TestPlatformRemoveAlwaysRemoveFromDB(c *gocheck.C) {
	provisioner := testing.ExtensibleFakeProvisioner{
		FakeProvisioner: testing.NewFakeProvisioner(),
	}
	Provisioner = &provisioner
	defer func() {
		Provisioner = s.provisioner
	}()
	conn, err := db.Conn()
	c.Assert(err, gocheck.IsNil)
	defer conn.Close()
	err = PlatformRemove("platform_dont_exists")
	c.Assert(err, gocheck.NotNil)
	name := "test_platform_update"
	args := make(map[string]string)
	args["dockerfile"] = "http://localhost/Dockerfile"
	err = PlatformAdd(name, nil, nil)
	c.Assert(err, gocheck.IsNil)
	provisioner.PlatformRemove(name)
	defer conn.Platforms().Remove(bson.M{"_id": name})
	err = PlatformRemove(name)
	c.Assert(err, gocheck.IsNil)
	count, err := conn.Platforms().Find(bson.M{"_id": name}).Count()
	c.Assert(err, gocheck.IsNil)
	c.Assert(count, gocheck.Equals, 0)
}
