// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"bytes"
	"errors"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/provision"
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
	conn.Apps().Database.DropDatabase()
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
	got, err := Platforms(false)
	c.Assert(err, check.IsNil)
	c.Assert(got, check.DeepEquals, want)
}

func (s *PlatformSuite) TestPlatformsWithFilterOnlyEnabledPlatforms(c *check.C) {
	input := []Platform{
		{Name: "dea"},
		{Name: "pecuniae", Disabled: true},
		{Name: "money"},
		{Name: "raise", Disabled: true},
		{Name: "glass", Disabled: false},
	}
	expected := []Platform{
		{Name: "dea"},
		{Name: "money"},
		{Name: "glass", Disabled: false},
	}
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	for _, p := range input {
		conn.Platforms().Insert(p)
		defer conn.Platforms().Remove(p)
	}
	got, err := Platforms(true)
	c.Assert(err, check.IsNil)
	c.Assert(got, check.DeepEquals, expected)
}

func (s *PlatformSuite) TestPlatformsEmpty(c *check.C) {
	got, err := Platforms(false)
	c.Assert(err, check.IsNil)
	c.Assert(got, check.HasLen, 0)
}

func (s *PlatformSuite) TestPlatformsEmptyWithQueryTrue(c *check.C) {
	got, err := Platforms(true)
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
	got, err := GetPlatform(p.Name)
	c.Assert(err, check.IsNil)
	c.Assert(*got, check.DeepEquals, p)
	got, err = GetPlatform("WAT")
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
	err = PlatformAdd(provision.PlatformOptions{Name: name, Args: args})
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
	err = PlatformAdd(provision.PlatformOptions{Name: name, Args: args})
	defer conn.Platforms().Remove(bson.M{"_id": name})
	c.Assert(err, check.IsNil)
	provisioner.PlatformRemove(name)
	err = PlatformAdd(provision.PlatformOptions{Name: name, Args: args})
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
	err = PlatformAdd(provision.PlatformOptions{Name: name, Args: args})
	defer conn.Platforms().Remove(bson.M{"_id": name})
	c.Assert(err, check.NotNil)
	count, err := conn.Platforms().FindId(name).Count()
	c.Assert(err, check.IsNil)
	c.Assert(count, check.Equals, 0)
}

func (s *PlatformSuite) TestPlatformAddNotExtensibleProvisioner(c *check.C) {
	err := PlatformAdd(provision.PlatformOptions{Name: "python"})
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
	err := PlatformAdd(provision.PlatformOptions{Name: ""})
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
	args["disabled"] = ""
	err = PlatformUpdate(provision.PlatformOptions{Name: name, Args: args})
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Platform doesn't exist.")
	err = PlatformAdd(provision.PlatformOptions{Name: name})
	c.Assert(err, check.IsNil)
	defer conn.Platforms().Remove(bson.M{"_id": name})
	err = PlatformUpdate(provision.PlatformOptions{Name: name, Args: args})
	c.Assert(err, check.IsNil)
}

func (s *PlatformSuite) TestPlatformUpdateDisableTrueWithDockerfile(c *check.C) {
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
	args["disabled"] = "true"
	err = PlatformAdd(provision.PlatformOptions{Name: name})
	c.Assert(err, check.IsNil)
	defer conn.Platforms().Remove(bson.M{"_id": name})
	appName := "test_app_1"
	app := App{
		Name:     appName,
		Platform: name,
	}
	err = conn.Apps().Insert(app)
	c.Assert(err, check.IsNil)
	defer conn.Apps().Remove(bson.M{"_id": appName})
	err = PlatformUpdate(provision.PlatformOptions{Name: name, Args: args})
	c.Assert(err, check.IsNil)
	a, err := GetByName(appName)
	c.Assert(err, check.IsNil)
	c.Assert(a.UpdatePlatform, check.Equals, true)
	platf, err := GetPlatform(name)
	c.Assert(err, check.IsNil)
	c.Assert(platf.Disabled, check.Equals, true)
}

func (s *PlatformSuite) TestPlatformUpdateDisabletrueFileIn(c *check.C) {
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
	args["disabled"] = "true"
	err = PlatformAdd(provision.PlatformOptions{Name: name})
	c.Assert(err, check.IsNil)
	defer conn.Platforms().Remove(bson.M{"_id": name})
	appName := "test_app_2"
	app := App{
		Name:     appName,
		Platform: name,
	}
	err = conn.Apps().Insert(app)
	c.Assert(err, check.IsNil)
	defer conn.Apps().Remove(bson.M{"name": appName})
	err = PlatformUpdate(provision.PlatformOptions{Name: name, Args: args, Input: bytes.NewReader(nil)})
	c.Assert(err, check.IsNil)
	platf, err := GetPlatform(name)
	c.Assert(err, check.IsNil)
	c.Assert(platf.Disabled, check.Equals, true)
	a, err := GetByName(appName)
	c.Assert(err, check.IsNil)
	c.Assert(a.UpdatePlatform, check.Equals, true)
}

func (s *PlatformSuite) TestPlatformUpdateDisabletrueWithoutDockerfile(c *check.C) {
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
	args["dockerfile"] = ""
	args["disabled"] = "true"
	err = PlatformAdd(provision.PlatformOptions{Name: name})
	c.Assert(err, check.IsNil)
	defer conn.Platforms().Remove(bson.M{"_id": name})
	appName := "test_app_2"
	app := App{
		Name:     appName,
		Platform: name,
	}
	err = conn.Apps().Insert(app)
	c.Assert(err, check.IsNil)
	defer conn.Apps().Remove(bson.M{"name": appName})
	err = PlatformUpdate(provision.PlatformOptions{Name: name, Args: args})
	c.Assert(err, check.IsNil)
	platf, err := GetPlatform(name)
	c.Assert(err, check.IsNil)
	c.Assert(platf.Disabled, check.Equals, true)
	a, err := GetByName(appName)
	c.Assert(err, check.IsNil)
	c.Assert(a.UpdatePlatform, check.Equals, false)
}

func (s *PlatformSuite) TestPlatformUpdateDisableFalseWithDockerfile(c *check.C) {
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
	args["disabled"] = "false"
	err = PlatformAdd(provision.PlatformOptions{Name: name})
	c.Assert(err, check.IsNil)
	defer conn.Platforms().Remove(bson.M{"_id": name})
	appName := "test_app_3"
	app := App{
		Name:     appName,
		Platform: name,
	}
	err = conn.Apps().Insert(app)
	c.Assert(err, check.IsNil)
	defer conn.Apps().Remove(bson.M{"_id": appName})
	err = PlatformUpdate(provision.PlatformOptions{Name: name, Args: args})
	c.Assert(err, check.IsNil)
	a, err := GetByName(appName)
	c.Assert(err, check.IsNil)
	c.Assert(a.UpdatePlatform, check.Equals, true)
	platf, err := GetPlatform(name)
	c.Assert(err, check.IsNil)
	c.Assert(platf.Disabled, check.Equals, false)
}

func (s *PlatformSuite) TestPlatformUpdateDisableFalseWithoutDockerfile(c *check.C) {
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
	args["dockerfile"] = ""
	args["disabled"] = "false"
	err = PlatformAdd(provision.PlatformOptions{Name: name})
	c.Assert(err, check.IsNil)
	defer conn.Platforms().Remove(bson.M{"_id": name})
	appName := "test_app_4"
	app := App{
		Name:     appName,
		Platform: name,
	}
	err = conn.Apps().Insert(app)
	c.Assert(err, check.IsNil)
	defer conn.Apps().Remove(bson.M{"_id": appName})
	err = PlatformUpdate(provision.PlatformOptions{Name: name, Args: args})
	c.Assert(err, check.IsNil)
	platf, err := GetPlatform(name)
	c.Assert(err, check.IsNil)
	c.Assert(platf.Disabled, check.Equals, false)
}

func (s *PlatformSuite) TestPlatformUpdateWithoutName(c *check.C) {
	provisioner := provisiontest.ExtensibleFakeProvisioner{
		FakeProvisioner: provisiontest.NewFakeProvisioner(),
	}
	Provisioner = &provisioner
	defer func() {
		Provisioner = s.provisioner
	}()
	err := PlatformUpdate(provision.PlatformOptions{Name: ""})
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
	err = PlatformAdd(provision.PlatformOptions{Name: name})
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
	err = PlatformUpdate(provision.PlatformOptions{Name: name, Args: args})
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
	err = PlatformAdd(provision.PlatformOptions{Name: name})
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
	err = PlatformAdd(provision.PlatformOptions{Name: name})
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
	err = PlatformAdd(provision.PlatformOptions{Name: name})
	c.Assert(err, check.IsNil)
	provisioner.PlatformRemove(name)
	defer conn.Platforms().Remove(bson.M{"_id": name})
	err = PlatformRemove(name)
	c.Assert(err, check.IsNil)
	count, err := conn.Platforms().Find(bson.M{"_id": name}).Count()
	c.Assert(err, check.IsNil)
	c.Assert(count, check.Equals, 0)
}
