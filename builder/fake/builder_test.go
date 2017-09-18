// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fake

import (
	"io/ioutil"
	"strings"

	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/app/image"
	"github.com/tsuru/tsuru/builder"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/permission"
	check "gopkg.in/check.v1"
)

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

func (s *S) TestBuildImageID(c *check.C) {
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
		ImageID: "myimg",
	}
	imgID, err := s.b.Build(s.provisioner, a, evt, opts)
	c.Assert(err, check.IsNil)
	c.Assert(imgID, check.Equals, "tsuru/app-myapp:v1")
	c.Assert(s.b.IsImageDeploy, check.Equals, true)
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
	_, err = image.AppNewImageName(a.Name)
	c.Assert(err, check.IsNil)
	opts = builder.BuildOpts{
		Rebuild: true,
	}
	imgID, err = s.b.Build(s.provisioner, a, evt, opts)
	c.Assert(err, check.IsNil)
	c.Assert(s.b.IsRebuildDeploy, check.Equals, true)
	c.Assert(imgID, check.Equals, "tsuru/app-myapp:v2-builder")
}
