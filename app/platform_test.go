// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"context"

	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/builder"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/storagev2"
	registrytest "github.com/tsuru/tsuru/registry/testing"
	servicemock "github.com/tsuru/tsuru/servicemanager/mock"
	appTypes "github.com/tsuru/tsuru/types/app"
	check "gopkg.in/check.v1"
)

type PlatformSuite struct {
	builder     *builder.MockBuilder
	conn        *db.Storage
	mockService servicemock.MockService
}

var _ = check.Suite(&PlatformSuite{})

func (s *PlatformSuite) SetUpSuite(c *check.C) {
	config.Set("log:disable-syslog", true)
	config.Set("database:url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("database:name", "platform_tests")
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	s.conn = conn
}

func (s *PlatformSuite) TearDownSuite(c *check.C) {
	defer s.conn.Close()
	storagev2.ClearAllCollections(nil)
}

func (s *PlatformSuite) SetUpTest(c *check.C) {
	s.builder = &builder.MockBuilder{}
	builder.Register("fake", s.builder)
	builder.DefaultBuilder = "fake"
	storagev2.ClearAllCollections(nil)
	servicemock.SetMockService(&s.mockService)
	s.mockService.PlatformImage.OnNewVersion = func(name string) (int, error) {
		return 1, nil
	}
	s.mockService.PlatformImage.OnAppendImages = func(name string, versions int, images []string) error {
		return nil
	}
}

func (s *PlatformSuite) TestPlatformCreate(c *check.C) {
	name := "test-platform-add"
	ps := &platformService{
		storage: &appTypes.MockPlatformStorage{
			OnInsert: func(p appTypes.Platform) error {
				c.Assert(p.Name, check.Equals, name)
				return nil
			},
		},
	}
	err := ps.Create(context.TODO(), appTypes.PlatformOptions{Name: name})
	c.Assert(err, check.IsNil)
}

func (s *PlatformSuite) TestPlatformCreateValidatesPlatformName(c *check.C) {
	ps := &platformService{
		storage: &appTypes.MockPlatformStorage{
			OnInsert: func(_ appTypes.Platform) error {
				return nil
			},
		},
	}
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
		{"myapp-41-characters-ppmyappmyappmyappmyap", appTypes.ErrInvalidPlatformName},
		{"myapp-40-characters-ppmyappmyappmyappmya", nil},
	}
	for _, t := range tt {
		err := ps.Create(context.TODO(), appTypes.PlatformOptions{Name: t.name})
		c.Check(err, check.DeepEquals, t.expectedErr)
	}
}

func (s *PlatformSuite) TestPlatformCreateWithStorageError(c *check.C) {
	ps := &platformService{
		storage: &appTypes.MockPlatformStorage{
			OnInsert: func(_ appTypes.Platform) error {
				return appTypes.ErrDuplicatePlatform
			},
		},
	}
	name := "test-platform-add"
	err := ps.Create(context.TODO(), appTypes.PlatformOptions{Name: name})
	c.Assert(err, check.Equals, appTypes.ErrDuplicatePlatform)
}

func (s *PlatformSuite) TestPlatformCreateWithProvisionerError(c *check.C) {
	s.builder.OnPlatformBuild = func(appTypes.PlatformOptions) ([]string, error) {
		return nil, errors.New("something wrong happened")
	}
	name := "test-platform-add"
	ps := &platformService{
		storage: &appTypes.MockPlatformStorage{
			OnInsert: func(p appTypes.Platform) error {
				c.Assert(p.Name, check.Equals, name)
				return nil
			},
			OnDelete: func(p appTypes.Platform) error {
				c.Assert(p.Name, check.Equals, name)
				return nil
			},
		},
	}
	args := make(map[string]string)
	args["dockerfile"] = "http://localhost/Dockerfile"
	opts := appTypes.PlatformOptions{Name: name, Args: args}
	err := ps.Create(context.TODO(), opts)
	c.Assert(err, check.NotNil)
}

func (s *PlatformSuite) TestPlatformList(c *check.C) {
	enabledPlatforms := []appTypes.Platform{
		{Name: "pecuniae"},
		{Name: "raise", Disabled: false},
		{Name: "glass"},
	}
	disabledPlatforms := []appTypes.Platform{
		{Name: "dea", Disabled: true},
		{Name: "money", Disabled: true},
	}
	ps := &platformService{
		storage: &appTypes.MockPlatformStorage{
			OnFindAll: func() ([]appTypes.Platform, error) {
				return append(enabledPlatforms, disabledPlatforms...), nil
			},
			OnFindEnabled: func() ([]appTypes.Platform, error) {
				return enabledPlatforms, nil
			},
		},
	}

	plats, err := ps.List(context.TODO(), false)
	c.Assert(err, check.IsNil)
	c.Assert(plats, check.HasLen, 5)

	plats, err = ps.List(context.TODO(), true)
	c.Assert(err, check.IsNil)
	c.Assert(plats, check.HasLen, 3)
}

func (s *PlatformSuite) TestPlatformFindByName(c *check.C) {
	ps := &platformService{
		storage: &appTypes.MockPlatformStorage{
			OnFindByName: func(name string) (*appTypes.Platform, error) {
				if name == "java" {
					return &appTypes.Platform{Name: "java"}, nil
				}
				return nil, appTypes.ErrPlatformNotFound
			},
		},
	}

	p, err := ps.FindByName(context.TODO(), "java")
	c.Assert(err, check.IsNil)
	c.Assert(p.Name, check.Equals, "java")

	p, err = ps.FindByName(context.TODO(), "other")
	c.Assert(err, check.Equals, appTypes.ErrInvalidPlatform)
	c.Assert(p, check.IsNil)
}

func (s *PlatformSuite) TestPlatformUpdate(c *check.C) {
	name := "test-platform-update"
	ps := &platformService{
		storage: &appTypes.MockPlatformStorage{
			OnFindByName: func(n string) (*appTypes.Platform, error) {
				if n == name {
					return &appTypes.Platform{Name: name}, nil
				}
				return nil, appTypes.ErrPlatformNotFound
			},
			OnUpdate: func(p appTypes.Platform) error {
				if p.Name == name {
					c.Assert(p.Disabled, check.Equals, false)
					return nil
				}
				return appTypes.ErrPlatformNotFound
			},
		},
	}
	args := make(map[string]string)
	args["disabled"] = ""

	s.builder.OnPlatformBuild = func(o appTypes.PlatformOptions) ([]string, error) {
		c.Assert(o.Data, check.NotNil)
		return nil, nil
	}
	err := ps.Update(context.TODO(), appTypes.PlatformOptions{Name: name, Args: args, Data: []byte("FROM tsuru/test")})
	c.Assert(err, check.IsNil)

	err = ps.Update(context.TODO(), appTypes.PlatformOptions{Name: "other", Args: args})
	c.Assert(err, check.Equals, appTypes.ErrInvalidPlatform)
}

func (s *PlatformSuite) TestPlatformUpdateDisableTrueWithDockerfile(c *check.C) {
	name := "test-platform-update"
	ps := &platformService{
		storage: &appTypes.MockPlatformStorage{
			OnFindByName: func(n string) (*appTypes.Platform, error) {
				if n == name {
					return &appTypes.Platform{Name: name}, nil
				}
				return nil, appTypes.ErrPlatformNotFound
			},
			OnUpdate: func(p appTypes.Platform) error {
				if p.Name == name {
					c.Assert(p.Disabled, check.Equals, true)
					return nil
				}
				return appTypes.ErrPlatformNotFound
			},
		},
	}
	args := make(map[string]string)
	args["disabled"] = "true"
	appName := "test-app-1"
	app := App{
		Name:     appName,
		Platform: name,
	}
	err := s.conn.Apps().Insert(app)
	c.Assert(err, check.IsNil)

	err = ps.Update(context.TODO(), appTypes.PlatformOptions{Name: name, Args: args, Data: []byte("FROM tsuru/test")})
	c.Assert(err, check.IsNil)
	a, err := GetByName(context.TODO(), appName)
	c.Assert(err, check.IsNil)
	c.Assert(a.UpdatePlatform, check.Equals, true)
}

func (s *PlatformSuite) TestPlatformUpdateDisableTrueFileIn(c *check.C) {
	name := "test-platform-update"
	ps := &platformService{
		storage: &appTypes.MockPlatformStorage{
			OnFindByName: func(n string) (*appTypes.Platform, error) {
				if n == name {
					return &appTypes.Platform{Name: name}, nil
				}
				return nil, appTypes.ErrPlatformNotFound
			},
			OnUpdate: func(p appTypes.Platform) error {
				if p.Name == name {
					c.Assert(p.Disabled, check.Equals, true)
					return nil
				}
				return appTypes.ErrPlatformNotFound
			},
		},
	}
	args := make(map[string]string)
	args["disabled"] = "true"
	appName := "test-app-2"
	app := App{
		Name:     appName,
		Platform: name,
	}
	err := s.conn.Apps().Insert(app)
	c.Assert(err, check.IsNil)

	err = ps.Update(context.TODO(), appTypes.PlatformOptions{Name: name, Args: args, Data: []byte("FROM tsuru/test")})
	c.Assert(err, check.IsNil)
	a, err := GetByName(context.TODO(), appName)
	c.Assert(err, check.IsNil)
	c.Assert(a.UpdatePlatform, check.Equals, true)
}

func (s *PlatformSuite) TestPlatformUpdateDisableTrueWithoutDockerfile(c *check.C) {
	name := "test-platform-update"
	ps := &platformService{
		storage: &appTypes.MockPlatformStorage{
			OnFindByName: func(n string) (*appTypes.Platform, error) {
				if n == name {
					return &appTypes.Platform{Name: name}, nil
				}
				return nil, appTypes.ErrPlatformNotFound
			},
			OnUpdate: func(p appTypes.Platform) error {
				if p.Name == name {
					c.Assert(p.Disabled, check.Equals, true)
					return nil
				}
				return appTypes.ErrPlatformNotFound
			},
		},
	}
	args := make(map[string]string)
	args["dockerfile"] = ""
	args["disabled"] = "true"
	appName := "test-app2"
	app := App{
		Name:     appName,
		Platform: name,
	}
	err := s.conn.Apps().Insert(app)
	c.Assert(err, check.IsNil)

	err = ps.Update(context.TODO(), appTypes.PlatformOptions{Name: name, Args: args})
	c.Assert(err, check.IsNil)
	a, err := GetByName(context.TODO(), appName)
	c.Assert(err, check.IsNil)
	c.Assert(a.UpdatePlatform, check.Equals, false)
}

func (s *PlatformSuite) TestPlatformUpdateDisableFalseWithDockerfile(c *check.C) {
	name := "test-platform-update"
	ps := &platformService{
		storage: &appTypes.MockPlatformStorage{
			OnFindByName: func(n string) (*appTypes.Platform, error) {
				if n == name {
					return &appTypes.Platform{Name: name}, nil
				}
				return nil, appTypes.ErrPlatformNotFound
			},
			OnUpdate: func(p appTypes.Platform) error {
				if p.Name == name {
					c.Assert(p.Disabled, check.Equals, false)
					return nil
				}
				return appTypes.ErrPlatformNotFound
			},
		},
	}
	args := make(map[string]string)
	args["disabled"] = "false"
	appName := "test-app3"
	app := App{
		Name:     appName,
		Platform: name,
	}
	err := s.conn.Apps().Insert(app)
	c.Assert(err, check.IsNil)

	err = ps.Update(context.TODO(), appTypes.PlatformOptions{Name: name, Args: args, Data: []byte("FROM tsuru/test")})
	c.Assert(err, check.IsNil)
	a, err := GetByName(context.TODO(), appName)
	c.Assert(err, check.IsNil)
	c.Assert(a.UpdatePlatform, check.Equals, true)
}

func (s *PlatformSuite) TestPlatformUpdateDisableFalseWithoutDockerfile(c *check.C) {
	name := "test-platform-update"
	ps := &platformService{
		storage: &appTypes.MockPlatformStorage{
			OnFindByName: func(n string) (*appTypes.Platform, error) {
				if n == name {
					return &appTypes.Platform{Name: name}, nil
				}
				return nil, appTypes.ErrPlatformNotFound
			},
			OnUpdate: func(p appTypes.Platform) error {
				if p.Name == name {
					c.Assert(p.Disabled, check.Equals, false)
					return nil
				}
				return appTypes.ErrPlatformNotFound
			},
		},
	}
	args := make(map[string]string)
	args["disabled"] = "false"
	appName := "test-app4"
	app := App{
		Name:     appName,
		Platform: name,
	}
	err := s.conn.Apps().Insert(app)
	c.Assert(err, check.IsNil)

	err = ps.Update(context.TODO(), appTypes.PlatformOptions{Name: name, Args: args})
	c.Assert(err, check.IsNil)
	a, err := GetByName(context.TODO(), appName)
	c.Assert(err, check.IsNil)
	c.Assert(a.UpdatePlatform, check.Equals, false)
}

func (s *PlatformSuite) TestPlatformUpdate_WithoutDisabledNorDockerfile(c *check.C) {
	ps := &platformService{
		storage: &appTypes.MockPlatformStorage{
			OnFindByName: func(n string) (*appTypes.Platform, error) {
				c.Check(n, check.Equals, "my-plat")
				return &appTypes.Platform{Name: "my-plat"}, nil
			},
		},
	}

	err := ps.Update(context.TODO(), appTypes.PlatformOptions{Name: "my-plat"})
	c.Assert(err, check.ErrorMatches, "either disabled or dockerfile must be provided")
}

func (s *PlatformSuite) TestPlatformUpdateWithoutName(c *check.C) {
	ps := &platformService{}
	err := ps.Update(context.TODO(), appTypes.PlatformOptions{Name: ""})
	c.Assert(err, check.Equals, appTypes.ErrPlatformNameMissing)
}

func (s *PlatformSuite) TestPlatformUpdateShouldSetUpdatePlatformFlagOnApps(c *check.C) {
	name := "test-platform-update"
	ps := &platformService{
		storage: &appTypes.MockPlatformStorage{
			OnFindByName: func(n string) (*appTypes.Platform, error) {
				if n == name {
					return &appTypes.Platform{Name: name}, nil
				}
				return nil, appTypes.ErrPlatformNotFound
			},
			OnUpdate: func(p appTypes.Platform) error {
				if p.Name == name {
					c.Assert(p.Disabled, check.Equals, false)
					return nil
				}
				return appTypes.ErrPlatformNotFound
			},
		},
	}
	args := make(map[string]string)
	args["dockerfile"] = "http://localhost/Dockerfile"
	appName := "test-app"
	app := App{
		Name:     appName,
		Platform: name,
	}
	err := s.conn.Apps().Insert(app)
	c.Assert(err, check.IsNil)

	err = ps.Update(context.TODO(), appTypes.PlatformOptions{Name: name, Args: args, Data: []byte("FROM tsuru/test")})
	c.Assert(err, check.IsNil)
	a, err := GetByName(context.TODO(), appName)
	c.Assert(err, check.IsNil)
	c.Assert(a.UpdatePlatform, check.Equals, true)
}

func (s *PlatformSuite) TestPlatformRemove(c *check.C) {
	name := "test-platform-remove"
	ps := &platformService{
		storage: &appTypes.MockPlatformStorage{
			OnDelete: func(p appTypes.Platform) error {
				if p.Name == name {
					return nil
				}
				return appTypes.ErrPlatformNotFound
			},
		},
	}

	registry, err := registrytest.NewServer("127.0.0.1:0")
	c.Assert(err, check.IsNil)
	defer registry.Stop()
	config.Set("registry", "docker")
	defer config.Unset("registry")
	config.Set("docker:registry", registry.Addr())
	defer config.Unset("docker:registry")
	registry.AddRepo(registrytest.Repository{Name: "tsuru/" + name, Tags: map[string]string{"v1": "abcdefg"}})
	c.Assert(registry.Repos, check.HasLen, 1)
	c.Assert(registry.Repos[0].Tags, check.HasLen, 1)

	s.mockService.PlatformImage.OnDeleteImages = func(name string) error {
		return nil
	}
	s.mockService.PlatformImage.OnListImagesOrDefault = func(name string) ([]string, error) {
		return []string{"tsuru/" + name + ":v1"}, nil
	}
	err = ps.Remove(context.TODO(), "platform-doesnt-exist")
	c.Assert(err, check.NotNil)
	c.Assert(err, check.Equals, appTypes.ErrPlatformNotFound)

	err = ps.Remove(context.TODO(), name)
	c.Assert(err, check.IsNil)
	c.Assert(registry.Repos, check.HasLen, 1)
	c.Assert(registry.Repos[0].Tags, check.HasLen, 0)

	err = ps.Remove(context.TODO(), "")
	c.Assert(err, check.Equals, appTypes.ErrPlatformNameMissing)
}

func (s *PlatformSuite) TestPlatformWithAppsCantBeRemoved(c *check.C) {
	name := "test-platform-remove"
	ps := &platformService{
		storage: &appTypes.MockPlatformStorage{
			OnDelete: func(p appTypes.Platform) error {
				if p.Name == name {
					return nil
				}
				return appTypes.ErrPlatformNotFound
			},
		},
	}
	appName := "test-another-app"
	app := App{
		Name:     appName,
		Platform: name,
	}
	err := s.conn.Apps().Insert(app)
	c.Assert(err, check.IsNil)

	err = ps.Remove(context.TODO(), name)
	c.Assert(err, check.NotNil)
}

func (s *PlatformSuite) TestPlatformRollback(c *check.C) {
	name := "test-platform-rollback"
	ps := &platformService{
		storage: &appTypes.MockPlatformStorage{
			OnFindByName: func(n string) (*appTypes.Platform, error) {
				if n == name {
					return &appTypes.Platform{Name: name}, nil
				}
				return nil, appTypes.ErrPlatformNotFound
			},
		},
	}
	s.builder.OnPlatformBuild = func(o appTypes.PlatformOptions) ([]string, error) {
		c.Assert(o.RollbackVersion, check.Equals, 1)
		return nil, nil
	}
	err := ps.Rollback(context.TODO(), appTypes.PlatformOptions{Name: name, RollbackVersion: 1})
	c.Assert(err, check.IsNil)
}
