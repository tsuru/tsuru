// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"bytes"
	"errors"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/builder"
	"github.com/tsuru/tsuru/builder/fake"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/repository/repositorytest"
	appTypes "github.com/tsuru/tsuru/types/app"
	"gopkg.in/check.v1"
)

type PlatformSuite struct {
	builder *fake.FakeBuilder
	conn    *db.Storage
}

var _ = check.Suite(&PlatformSuite{})

func (s *PlatformSuite) SetUpSuite(c *check.C) {
	config.Set("log:disable-syslog", true)
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "platform_tests")
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	s.conn = conn
	s.builder = fake.NewFakeBuilder()
	builder.Register("fake", s.builder)
}

func (s *PlatformSuite) TearDownSuite(c *check.C) {
	defer s.conn.Close()
	s.conn.Apps().Database.DropDatabase()
}

func (s *PlatformSuite) SetUpTest(c *check.C) {
	repositorytest.Reset()
	s.builder.Reset()
	dbtest.ClearAllCollections(s.conn.Apps().Database)
}

func (s *PlatformSuite) TestPlatforms(c *check.C) {
	want := []appTypes.Platform{
		{Name: "dea"},
		{Name: "pecuniae"},
		{Name: "money"},
		{Name: "raise"},
		{Name: "glass"},
	}
	for _, p := range want {
		PlatformService().Insert(p)
	}
	got, err := Platforms(false)
	c.Assert(err, check.IsNil)
	c.Assert(got, check.DeepEquals, want)
}

func (s *PlatformSuite) TestPlatformsWithFilterOnlyEnabledPlatforms(c *check.C) {
	input := []appTypes.Platform{
		{Name: "dea"},
		{Name: "pecuniae", Disabled: true},
		{Name: "money"},
		{Name: "raise", Disabled: true},
		{Name: "glass", Disabled: false},
	}
	expected := []appTypes.Platform{
		{Name: "dea"},
		{Name: "money"},
		{Name: "glass", Disabled: false},
	}
	for _, p := range input {
		PlatformService().Insert(p)
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
	p := appTypes.Platform{Name: "dea"}
	PlatformService().Insert(p)
	got, err := GetPlatform(p.Name)
	c.Assert(err, check.IsNil)
	c.Assert(*got, check.DeepEquals, p)
	got, err = GetPlatform("WAT")
	c.Assert(got, check.IsNil)
	c.Assert(err, check.Equals, appTypes.ErrInvalidPlatform)
}

func (s *PlatformSuite) TestPlatformAdd(c *check.C) {
	name := "test-platform-add"
	args := make(map[string]string)
	args["dockerfile"] = "http://localhost/Dockerfile"
	err := PlatformAdd(builder.PlatformOptions{Name: name, Args: args})
	c.Assert(err, check.IsNil)
	platform, err := GetPlatform(name)
	c.Assert(err, check.IsNil)
	c.Assert(platform.Name, check.Equals, name)
}

func (s *PlatformSuite) TestPlatformAddValidatesPlatformName(c *check.C) {
	tt := []struct {
		name        string
		expectedErr error
	}{
		{"platform", nil},
		{"Platform", appTypes.ErrInvalidPlatformName},
		{"", appTypes.ErrPlatformNameMissing},
		{"plat_form", appTypes.ErrInvalidPlatformName},
		{"123platform", appTypes.ErrInvalidPlatformName},
		{"plat-form", nil},
		{"myappmyappmyappmyappmyappmyappmyappmyappmyappmyappmyappmyappmyapp", appTypes.ErrInvalidPlatformName},
		{"myappmyappmyappmyappmyappmyappmyappmyappmyappmyappmyappmyappmyap", appTypes.ErrInvalidPlatformName},
		{"myappmyappmyappmyappmyappmyappmyappmyappmyappmyappmyappmyappmya", nil},
	}
	for _, t := range tt {
		err := PlatformAdd(builder.PlatformOptions{Name: t.name})
		c.Assert(err, check.DeepEquals, t.expectedErr)
	}
}

func (s *PlatformSuite) TestPlatformAddDuplicate(c *check.C) {
	name := "test-platform-add"
	args := make(map[string]string)
	args["dockerfile"] = "http://localhost/Dockerfile"
	err := PlatformAdd(builder.PlatformOptions{Name: name, Args: args})
	c.Assert(err, check.IsNil)
	err = PlatformAdd(builder.PlatformOptions{Name: name, Args: args})
	c.Assert(err, check.Equals, appTypes.ErrDuplicatePlatform)
}

func (s *PlatformSuite) TestPlatformAddWithProvisionerError(c *check.C) {
	name := "test-platform-add"
	args := make(map[string]string)
	args["dockerfile"] = "http://localhost/Dockerfile"
	opts := builder.PlatformOptions{Name: name, Args: args}
	s.builder.PrepareFailure("PlatformAdd", errors.New("something wrong happened"))
	err := PlatformAdd(opts)
	c.Assert(err, check.NotNil)
	p, err := PlatformService().FindByName(name)
	c.Assert(err, check.Equals, appTypes.ErrPlatformNotFound)
	c.Assert(p, check.IsNil)
}

func (s *PlatformSuite) TestPlatformAddWithoutName(c *check.C) {
	err := PlatformAdd(builder.PlatformOptions{Name: ""})
	c.Assert(err, check.Equals, appTypes.ErrPlatformNameMissing)
}

func (s *PlatformSuite) TestPlatformUpdate(c *check.C) {
	name := "test-platform-update"
	args := make(map[string]string)
	args["dockerfile"] = "http://localhost/Dockerfile"
	args["disabled"] = ""
	err := PlatformUpdate(builder.PlatformOptions{Name: name, Args: args})
	c.Assert(err, check.Equals, appTypes.ErrPlatformNotFound)
	err = PlatformAdd(builder.PlatformOptions{Name: name})
	c.Assert(err, check.IsNil)
	err = PlatformUpdate(builder.PlatformOptions{Name: name, Args: args})
	c.Assert(err, check.IsNil)
}

func (s *PlatformSuite) TestPlatformUpdateDisableTrueWithDockerfile(c *check.C) {
	name := "test-platform-update"
	args := make(map[string]string)
	args["dockerfile"] = "http://localhost/Dockerfile"
	args["disabled"] = "true"
	err := PlatformAdd(builder.PlatformOptions{Name: name})
	c.Assert(err, check.IsNil)
	appName := "test-app-1"
	app := App{
		Name:     appName,
		Platform: name,
	}
	err = s.conn.Apps().Insert(app)
	c.Assert(err, check.IsNil)
	err = PlatformUpdate(builder.PlatformOptions{Name: name, Args: args})
	c.Assert(err, check.IsNil)
	a, err := GetByName(appName)
	c.Assert(err, check.IsNil)
	c.Assert(a.UpdatePlatform, check.Equals, true)
	platf, err := GetPlatform(name)
	c.Assert(err, check.IsNil)
	c.Assert(platf.Disabled, check.Equals, true)
}

func (s *PlatformSuite) TestPlatformUpdateDisableTrueFileIn(c *check.C) {
	name := "test-platform-update"
	args := make(map[string]string)
	args["disabled"] = "true"
	err := PlatformAdd(builder.PlatformOptions{Name: name})
	c.Assert(err, check.IsNil)
	appName := "test-app-2"
	app := App{
		Name:     appName,
		Platform: name,
	}
	err = s.conn.Apps().Insert(app)
	c.Assert(err, check.IsNil)
	err = PlatformUpdate(builder.PlatformOptions{Name: name, Args: args, Input: bytes.NewReader(nil)})
	c.Assert(err, check.IsNil)
	platf, err := GetPlatform(name)
	c.Assert(err, check.IsNil)
	c.Assert(platf.Disabled, check.Equals, true)
	a, err := GetByName(appName)
	c.Assert(err, check.IsNil)
	c.Assert(a.UpdatePlatform, check.Equals, true)
}

func (s *PlatformSuite) TestPlatformUpdateDisableTrueWithoutDockerfile(c *check.C) {
	name := "test-platform-update"
	args := make(map[string]string)
	args["dockerfile"] = ""
	args["disabled"] = "true"
	err := PlatformAdd(builder.PlatformOptions{Name: name})
	c.Assert(err, check.IsNil)
	appName := "test-app-2"
	app := App{
		Name:     appName,
		Platform: name,
	}
	err = s.conn.Apps().Insert(app)
	c.Assert(err, check.IsNil)
	err = PlatformUpdate(builder.PlatformOptions{Name: name, Args: args})
	c.Assert(err, check.IsNil)
	platf, err := GetPlatform(name)
	c.Assert(err, check.IsNil)
	c.Assert(platf.Disabled, check.Equals, true)
	a, err := GetByName(appName)
	c.Assert(err, check.IsNil)
	c.Assert(a.UpdatePlatform, check.Equals, false)
}

func (s *PlatformSuite) TestPlatformUpdateDisableFalseWithDockerfile(c *check.C) {
	name := "test-platform-update"
	args := make(map[string]string)
	args["dockerfile"] = "http://localhost/Dockerfile"
	args["disabled"] = "false"
	err := PlatformAdd(builder.PlatformOptions{Name: name})
	c.Assert(err, check.IsNil)
	appName := "test-app-3"
	app := App{
		Name:     appName,
		Platform: name,
	}
	err = s.conn.Apps().Insert(app)
	c.Assert(err, check.IsNil)
	err = PlatformUpdate(builder.PlatformOptions{Name: name, Args: args})
	c.Assert(err, check.IsNil)
	a, err := GetByName(appName)
	c.Assert(err, check.IsNil)
	c.Assert(a.UpdatePlatform, check.Equals, true)
	platf, err := GetPlatform(name)
	c.Assert(err, check.IsNil)
	c.Assert(platf.Disabled, check.Equals, false)
}

func (s *PlatformSuite) TestPlatformUpdateDisableFalseWithoutDockerfile(c *check.C) {
	name := "test-platform-update"
	args := make(map[string]string)
	args["dockerfile"] = ""
	args["disabled"] = "false"
	err := PlatformAdd(builder.PlatformOptions{Name: name})
	c.Assert(err, check.IsNil)
	appName := "test-app-4"
	app := App{
		Name:     appName,
		Platform: name,
	}
	err = s.conn.Apps().Insert(app)
	c.Assert(err, check.IsNil)
	err = PlatformUpdate(builder.PlatformOptions{Name: name, Args: args})
	c.Assert(err, check.IsNil)
	platf, err := GetPlatform(name)
	c.Assert(err, check.IsNil)
	c.Assert(platf.Disabled, check.Equals, false)
}

func (s *PlatformSuite) TestPlatformUpdateWithoutName(c *check.C) {
	err := PlatformUpdate(builder.PlatformOptions{Name: ""})
	c.Assert(err, check.Equals, appTypes.ErrPlatformNameMissing)
}

func (s *PlatformSuite) TestPlatformUpdateShouldSetUpdatePlatformFlagOnApps(c *check.C) {
	name := "test-platform-update"
	args := make(map[string]string)
	args["dockerfile"] = "http://localhost/Dockerfile"
	err := PlatformAdd(builder.PlatformOptions{Name: name})
	c.Assert(err, check.IsNil)
	appName := "test-app"
	app := App{
		Name:     appName,
		Platform: name,
	}
	err = s.conn.Apps().Insert(app)
	c.Assert(err, check.IsNil)
	err = PlatformUpdate(builder.PlatformOptions{Name: name, Args: args})
	c.Assert(err, check.IsNil)
	a, err := GetByName(appName)
	c.Assert(err, check.IsNil)
	c.Assert(a.UpdatePlatform, check.Equals, true)
}

func (s *PlatformSuite) TestPlatformRemove(c *check.C) {
	err := PlatformRemove("platform-dont-exists")
	c.Assert(err, check.NotNil)
	c.Assert(err, check.Equals, appTypes.ErrPlatformNotFound)
	name := "test-platform-update"
	args := make(map[string]string)
	args["dockerfile"] = "http://localhost/Dockerfile"
	err = PlatformAdd(builder.PlatformOptions{Name: name})
	c.Assert(err, check.IsNil)
	err = PlatformRemove(name)
	c.Assert(err, check.IsNil)
	p, err := PlatformService().FindByName(name)
	c.Assert(err, check.Equals, appTypes.ErrPlatformNotFound)
	c.Assert(p, check.IsNil)
	err = PlatformRemove("")
	c.Assert(err, check.Equals, appTypes.ErrPlatformNameMissing)
}

func (s *PlatformSuite) TestPlatformWithAppsCantBeRemoved(c *check.C) {
	name := "test-platform-update"
	args := make(map[string]string)
	args["dockerfile"] = "http://localhost/Dockerfile"
	err := PlatformAdd(builder.PlatformOptions{Name: name})
	c.Assert(err, check.IsNil)
	appName := "test-another-app"
	app := App{
		Name:     appName,
		Platform: name,
	}
	err = s.conn.Apps().Insert(app)
	c.Assert(err, check.IsNil)
	err = PlatformRemove(name)
	c.Assert(err, check.NotNil)
}

func (s *PlatformSuite) TestPlatformRemoveAlwaysRemoveFromDB(c *check.C) {
	err := PlatformRemove("platform-dont-exists")
	c.Assert(err, check.NotNil)
	name := "test-platform-update"
	args := make(map[string]string)
	args["dockerfile"] = "http://localhost/Dockerfile"
	err = PlatformAdd(builder.PlatformOptions{Name: name})
	c.Assert(err, check.IsNil)
	err = PlatformRemove(name)
	c.Assert(err, check.IsNil)
	p, err := PlatformService().FindByName(name)
	c.Assert(err, check.Equals, appTypes.ErrPlatformNotFound)
	c.Assert(p, check.IsNil)
}
