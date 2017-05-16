// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fake

import (
	"io/ioutil"
	"strings"
	"testing"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/auth/native"
	"github.com/tsuru/tsuru/builder"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/quota"
	"github.com/tsuru/tsuru/router/routertest"
	check "gopkg.in/check.v1"
)

type S struct {
	b           *FakeBuilder
	conn        *db.Storage
	user        *auth.User
	team        *auth.Team
	token       auth.Token
	provisioner *provisiontest.FakeProvisioner
}

var _ = check.Suite(&S{})

func Test(t *testing.T) {
	check.TestingT(t)
}

func (s *S) SetUpSuite(c *check.C) {
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "fake_builder_tests_s")
	config.Set("routers:fake:type", "fake")
	config.Set("routers:fake:default", true)
	var err error
	s.conn, err = db.Conn()
	c.Assert(err, check.IsNil)
	err = dbtest.ClearAllCollections(s.conn.Apps().Database)
	c.Assert(err, check.IsNil)
	s.provisioner = provisiontest.ProvisionerInstance
	provision.DefaultProvisioner = "fake"
}

func (s *S) SetUpTest(c *check.C) {
	routertest.FakeRouter.Reset()
	s.provisioner.Reset()
	err := dbtest.ClearAllCollections(s.conn.Apps().Database)
	c.Assert(err, check.IsNil)
	err = provision.AddPool(provision.AddPoolOptions{
		Name:        "thepool",
		Default:     true,
		Provisioner: "fake",
	})
	c.Assert(err, check.IsNil)
	p := app.Plan{
		Name:     "default",
		Default:  true,
		CpuShare: 100,
	}
	err = p.Save()
	c.Assert(err, check.IsNil)
	s.b = &FakeBuilder{}
	s.user = &auth.User{Email: "whiskeyjack@genabackis.com", Password: "123456", Quota: quota.Unlimited}
	nativeScheme := auth.ManagedScheme(native.NativeScheme{})
	app.AuthScheme = nativeScheme
	_, err = nativeScheme.Create(s.user)
	c.Assert(err, check.IsNil)
	s.team = &auth.Team{Name: "admin"}
	c.Assert(err, check.IsNil)
	err = s.conn.Teams().Insert(s.team)
	c.Assert(err, check.IsNil)
	s.token, err = nativeScheme.Login(map[string]string{"email": s.user.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
}

func (s *S) TestBuildArchiveURL(c *check.C) {
	a := &app.App{Name: "myapp", Platform: "whitespace", TeamOwner: s.team.Name}
	err := app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	evt, err := event.New(&event.Opts{
		Target:   event.Target{Type: "app", Value: a.GetName()},
		Kind:     permission.PermAppDeploy,
		RawOwner: event.Owner{Type: event.OwnerTypeUser, Name: "me@me.com"},
		Allowed:  event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	opts := builder.BuildOpts{
		ArchiveURL: "http://test.com/myfile.tgz",
	}
	imgID, err := s.b.Build(s.provisioner, a, evt, opts)
	c.Assert(err, check.IsNil)
	c.Assert(imgID, check.Equals, "tsuru/app-myapp:v1-builder")
	c.Assert(s.b.IsArchiveURLDeploy, check.Equals, true)
}

func (s *S) TestBuildArchiveUpload(c *check.C) {
	a := &app.App{Name: "myapp", Platform: "whitespace", TeamOwner: s.team.Name}
	err := app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	evt, err := event.New(&event.Opts{
		Target:   event.Target{Type: "app", Value: a.GetName()},
		Kind:     permission.PermAppDeploy,
		RawOwner: event.Owner{Type: event.OwnerTypeUser, Name: "me@me.com"},
		Allowed:  event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	buf := strings.NewReader("my upload data")
	opts := builder.BuildOpts{
		ArchiveFile: ioutil.NopCloser(buf),
		ArchiveSize: int64(buf.Len()),
	}
	imgID, err := s.b.Build(s.provisioner, a, evt, opts)
	c.Assert(err, check.IsNil)
	c.Assert(imgID, check.Equals, "tsuru/app-myapp:v1-builder")
	c.Assert(s.b.IsArchiveFileDeploy, check.Equals, true)
}

func (s *S) TestBuilderRebuild(c *check.C) {
	a := &app.App{Name: "myapp", Platform: "whitespace", TeamOwner: s.team.Name}
	err := app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	evt, err := event.New(&event.Opts{
		Target:   event.Target{Type: "app", Value: a.GetName()},
		Kind:     permission.PermAppDeploy,
		RawOwner: event.Owner{Type: event.OwnerTypeUser, Name: "me@me.com"},
		Allowed:  event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	buf := strings.NewReader("my upload data")
	opts := builder.BuildOpts{
		ArchiveFile: ioutil.NopCloser(buf),
		ArchiveSize: int64(buf.Len()),
	}
	imgID, err := s.b.Build(s.provisioner, a, evt, opts)
	c.Assert(err, check.IsNil)
	c.Assert(imgID, check.Equals, "tsuru/app-myapp:v1-builder")
	c.Assert(s.b.IsArchiveFileDeploy, check.Equals, true)
	opts = builder.BuildOpts{
		Rebuild: true,
	}
	imgID, err = s.b.Build(s.provisioner, a, evt, opts)
	c.Assert(err, check.IsNil)
	c.Assert(s.b.IsRebuildDeploy, check.Equals, true)
	c.Assert(imgID, check.Equals, "tsuru/app-myapp:v2-builder")
}
