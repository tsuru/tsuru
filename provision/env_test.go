// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package provision_test

import (
	"context"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app/version"
	"github.com/tsuru/tsuru/db/storagev2"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/provisiontest"
	servicemock "github.com/tsuru/tsuru/servicemanager/mock"
	_ "github.com/tsuru/tsuru/storage/mongodb"
	appTypes "github.com/tsuru/tsuru/types/app"
	bindTypes "github.com/tsuru/tsuru/types/bind"
	check "gopkg.in/check.v1"
)

type S struct {
	mockService servicemock.MockService
}

var _ = check.Suite(&S{})

func (s *S) SetUpTest(c *check.C) {
	config.Set("database:url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("database:name", "provision_tests_2_s")
	err := storagev2.ClearAllCollections(nil)
	c.Assert(err, check.IsNil)

	servicemock.SetMockService(&s.mockService)
}

func (s *S) TestWebProcessDefaultPort(c *check.C) {
	port := provision.WebProcessDefaultPort()
	c.Assert(port, check.Equals, "8888")
}

func (s *S) TestWebProcessDefaultPortWithConfig(c *check.C) {
	config.Set("docker:run-cmd:port", "9191")
	defer config.Unset("docker:run-cmd:port")
	port := provision.WebProcessDefaultPort()
	c.Assert(port, check.Equals, "9191")
}

func (s *S) TestEnvsForApp(c *check.C) {
	a := provisiontest.NewFakeApp("myapp", "crystal", 1)
	a.Env = map[string]bindTypes.EnvVar{
		"e1": {Name: "e1", Value: "v1"},
	}
	envs := provision.EnvsForAppAndVersion(a, "p1", nil)
	c.Assert(envs, check.DeepEquals, []bindTypes.EnvVar{
		{Name: "TSURU_APPDIR", Value: "/home/application/current", ManagedBy: "tsuru", Public: true},
		{Name: "TSURU_APPNAME", Value: "myapp", ManagedBy: "tsuru", Public: true},
		{Name: "TSURU_SERVICES", Value: "{}", ManagedBy: "tsuru"},
		{Name: "e1", Value: "v1"},
		{Name: "TSURU_PROCESSNAME", Value: "p1", Public: true},
		{Name: "TSURU_HOST", Value: "", Public: true},
		{Name: "port", Value: "8888", Public: true},
		{Name: "PORT", Value: "8888", Public: true},
	})
}

func (s *S) TestEnvsForAppWithVersion(c *check.C) {
	a := provisiontest.NewFakeApp("myapp", "crystal", 1)
	a.Env = map[string]bindTypes.EnvVar{
		"e1": {Name: "e1", Value: "v1"},
	}

	svc, err := version.AppVersionService()
	c.Assert(err, check.IsNil)
	version, err := svc.NewAppVersion(context.TODO(), appTypes.NewVersionArgs{App: a})
	c.Assert(err, check.IsNil)

	envs := provision.EnvsForAppAndVersion(a, "p1", version)
	c.Assert(envs, check.DeepEquals, []bindTypes.EnvVar{
		{Name: "TSURU_APPDIR", Value: "/home/application/current", ManagedBy: "tsuru", Public: true},
		{Name: "TSURU_APPNAME", Value: "myapp", ManagedBy: "tsuru", Public: true},
		{Name: "TSURU_SERVICES", Value: "{}", ManagedBy: "tsuru"},
		{Name: "e1", Value: "v1"},
		{Name: "TSURU_PROCESSNAME", Value: "p1", Public: true},
		{Name: "TSURU_APPVERSION", Value: "1", Public: true},
		{Name: "TSURU_HOST", Value: "", Public: true},
		{Name: "port", Value: "8888", Public: true},
		{Name: "PORT", Value: "8888", Public: true},
	})

}

func (s *S) TestEnvsForAppCustomConfig(c *check.C) {
	config.Set("host", "cloud.tsuru.io")
	config.Set("docker:run-cmd:port", "8989")
	defer config.Unset("host")
	defer config.Unset("docker:run-cmd:port")
	a := provisiontest.NewFakeApp("myapp", "crystal", 1)
	a.Env = map[string]bindTypes.EnvVar{
		"e1": {Name: "e1", Value: "v1"},
	}
	envs := provision.EnvsForAppAndVersion(a, "p1", nil)
	c.Assert(envs, check.DeepEquals, []bindTypes.EnvVar{
		{Name: "TSURU_APPDIR", Value: "/home/application/current", ManagedBy: "tsuru", Public: true},
		{Name: "TSURU_APPNAME", Value: "myapp", ManagedBy: "tsuru", Public: true},
		{Name: "TSURU_SERVICES", Value: "{}", ManagedBy: "tsuru"},
		{Name: "e1", Value: "v1"},
		{Name: "TSURU_PROCESSNAME", Value: "p1", Public: true},
		{Name: "TSURU_HOST", Value: "cloud.tsuru.io", Public: true},
		{Name: "port", Value: "8989", Public: true},
		{Name: "PORT", Value: "8989", Public: true},
	})
}
