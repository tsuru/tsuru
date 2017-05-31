// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/app/image"
	"github.com/tsuru/tsuru/builder"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
	check "gopkg.in/check.v1"
)

func (s *S) TestBuilderArchiveURL(c *check.C) {
	opts := provision.AddNodeOptions{Address: s.server.URL()}
	err := s.provisioner.AddNode(opts)
	c.Assert(err, check.IsNil)
	a := &app.App{Name: "myapp", Platform: "whitespace", TeamOwner: s.team.Name}
	err = app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("my archive data"))
	}))
	defer ts.Close()
	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)
	bopts := builder.BuildOpts{
		ArchiveURL: ts.URL + "/myfile.tgz",
	}
	imgID, err := s.b.Build(s.provisioner, a, evt, bopts)
	c.Assert(err, check.IsNil)
	c.Assert(imgID, check.Equals, "tsuru/app-myapp:v1-builder")
}

func (s *S) TestBuilderArchiveURLEmptyFile(c *check.C) {
	opts := provision.AddNodeOptions{Address: s.server.URL()}
	err := s.provisioner.AddNode(opts)
	c.Assert(err, check.IsNil)
	a := &app.App{Name: "myapp", Platform: "whitespace", TeamOwner: s.team.Name}
	err = app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)
	bopts := builder.BuildOpts{
		ArchiveURL: ts.URL + "/myfile.tgz",
	}
	imgID, err := s.b.Build(s.provisioner, a, evt, bopts)
	c.Assert(err, check.NotNil)
	c.Assert(imgID, check.Equals, "")
}

func (s *S) TestBuilderArchiveFile(c *check.C) {
	opts := provision.AddNodeOptions{Address: s.server.URL()}
	err := s.provisioner.AddNode(opts)
	c.Assert(err, check.IsNil)
	a := &app.App{Name: "myapp", Platform: "whitespace", TeamOwner: s.team.Name}
	err = app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)
	buf := strings.NewReader("my upload data")
	bopts := builder.BuildOpts{
		ArchiveFile: ioutil.NopCloser(buf),
		ArchiveSize: int64(buf.Len()),
	}
	imgID, err := s.b.Build(s.provisioner, a, evt, bopts)
	c.Assert(err, check.IsNil)
	c.Assert(imgID, check.Equals, "tsuru/app-myapp:v1-builder")
}

func (s *S) TestBuilderRebuild(c *check.C) {
	opts := provision.AddNodeOptions{Address: s.server.URL()}
	err := s.provisioner.AddNode(opts)
	c.Assert(err, check.IsNil)
	a := &app.App{Name: "myapp", Platform: "whitespace", TeamOwner: s.team.Name}
	err = app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)
	buf := strings.NewReader("my upload data")
	bopts := builder.BuildOpts{
		ArchiveFile: ioutil.NopCloser(buf),
		ArchiveSize: int64(buf.Len()),
	}
	imgID, err := s.b.Build(s.provisioner, a, evt, bopts)
	c.Assert(err, check.IsNil)
	c.Assert(imgID, check.Equals, "tsuru/app-myapp:v1-builder")
	_, err = image.AppNewImageName(a.Name)
	c.Assert(err, check.IsNil)
	bopts = builder.BuildOpts{
		Rebuild: true,
	}
	imgID, err = s.b.Build(s.provisioner, a, evt, bopts)
	c.Assert(err, check.IsNil)
	c.Assert(imgID, check.Equals, "tsuru/app-myapp:v2-builder")
}

func (s *S) TestBuilderErasesOldImages(c *check.C) {
	opts := provision.AddNodeOptions{Address: s.server.URL()}
	err := s.provisioner.AddNode(opts)
	c.Assert(err, check.IsNil)
	config.Set("docker:image-history-size", 1)
	defer config.Unset("docker:image-history-size")
	a := &app.App{Name: "myapp", Platform: "python", TeamOwner: s.team.Name}
	err = app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: "app", Value: a.Name},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	buildOpts := builder.BuildOpts{
		ArchiveURL: "http://mystorage.com/archive.tar.gz",
	}
	_, err = s.b.Build(s.provisioner, a, evt, buildOpts)
	c.Assert(err, check.IsNil)
	dclient, err := docker.NewClient(s.server.URL())
	imgs, err := dclient.ListImages(docker.ListImagesOptions{All: true})
	c.Assert(err, check.IsNil)
	c.Assert(imgs, check.HasLen, 2)
	c.Assert(imgs[0].RepoTags, check.HasLen, 1)
	c.Assert(imgs[1].RepoTags, check.HasLen, 1)
	expected := []string{"tsuru/app-myapp:v1-builder", "tsuru/python:latest"}
	got := []string{imgs[0].RepoTags[0], imgs[1].RepoTags[0]}
	sort.Strings(got)
	c.Assert(got, check.DeepEquals, expected)
	_, err = image.AppNewImageName(a.Name)
	c.Assert(err, check.IsNil)
	_, err = s.b.Build(s.provisioner, a, evt, buildOpts)
	c.Assert(err, check.IsNil)
	imgs, err = dclient.ListImages(docker.ListImagesOptions{All: true})
	c.Assert(err, check.IsNil)
	c.Assert(imgs, check.HasLen, 2)
	c.Assert(imgs[0].RepoTags, check.HasLen, 1)
	c.Assert(imgs[1].RepoTags, check.HasLen, 1)
	got = []string{imgs[0].RepoTags[0], imgs[1].RepoTags[0]}
	sort.Strings(got)
	expected = []string{"tsuru/app-myapp:v2-builder", "tsuru/python:latest"}
	c.Assert(got, check.DeepEquals, expected)
}
