// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/tsuru/tsuru/app/image"
	"github.com/tsuru/tsuru/builder"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/permission"
	check "gopkg.in/check.v1"
)

func (s *S) TestBuilderArchiveFile(c *check.C) {
	a, _, rollback := s.defaultReactions(c)
	defer rollback()
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
	imgID, err := s.b.Build(s.p, a, evt, &bopts)
	c.Assert(err, check.IsNil, check.Commentf("%+v", err))
	c.Assert(imgID, check.Equals, "tsuru/app-myapp:v1-builder")
}

func (s *S) TestBuilderArchiveFileWithTag(c *check.C) {
	a, _, rollback := s.defaultReactions(c)
	a.TeamOwner = "admin"
	defer rollback()
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
		Tag:         "mytag",
	}
	imgID, err := s.b.Build(s.p, a, evt, &bopts)
	c.Assert(err, check.IsNil, check.Commentf("%+v", err))
	c.Assert(imgID, check.Equals, s.team.Name+"/app-myapp:mytag")
}

func (s *S) TestBuilderArchiveURL(c *check.C) {
	a, _, rollback := s.defaultReactions(c)
	defer rollback()
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
	imgID, err := s.b.Build(s.p, a, evt, &bopts)
	c.Assert(err, check.NotNil)
	c.Assert(imgID, check.Equals, "")
}

func (s *S) TestBuilderImageID(c *check.C) {
	a, _, rollback := s.defaultReactions(c)
	defer rollback()
	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)
	s.logHook = func(w io.Writer, r *http.Request) {
		container := r.URL.Query().Get("container")
		if container == "myapp-v1-build-yamldata" || container == "myapp-v1-builder-procfile-inspect" {
			w.Write([]byte(""))
			return
		}
		w.Write([]byte(`[{"Config": {"Cmd": ["arg1"], "Entrypoint": ["run", "mycmd"], "ExposedPorts": null}}]`))
	}
	bopts := builder.BuildOpts{
		ImageID: "test/customimage",
	}
	img, err := s.b.Build(s.p, a, evt, &bopts)
	c.Assert(err, check.IsNil, check.Commentf("%+v", err))
	c.Assert(img, check.Equals, "tsuru/app-myapp:v1")
}

func (s *S) TestBuilderImageIDWithProcfile(c *check.C) {
	a, _, rollback := s.defaultReactions(c)
	defer rollback()
	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)
	s.logHook = func(w io.Writer, r *http.Request) {
		container := r.URL.Query().Get("container")
		if container == "myapp-v1-build-procfile-inspect" {
			w.Write([]byte(`web: test.sh`))
			return
		}
		if container == "myapp-v1-build-yamldata" {
			w.Write([]byte(""))
			return
		}
		w.Write([]byte(`[{"Config": {"Cmd": null, "Entrypoint": null, "ExposedPorts": null}}]`))
	}
	bopts := builder.BuildOpts{
		ImageID: "test/customimage",
	}
	img, err := s.b.Build(s.p, a, evt, &bopts)
	c.Assert(err, check.IsNil, check.Commentf("%+v", err))
	c.Assert(img, check.Equals, "tsuru/app-myapp:v1")
	imd, err := image.GetImageMetaData(img)
	c.Assert(err, check.IsNil)
	expectedProcesses := map[string][]string{"web": {"test.sh"}}
	c.Assert(imd.Processes, check.DeepEquals, expectedProcesses)
}

func (s *S) TestBuilderImageIDWithTsuruYaml(c *check.C) {
	a, _, rollback := s.defaultReactions(c)
	defer rollback()
	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)
	s.logHook = func(w io.Writer, r *http.Request) {
		container := r.URL.Query().Get("container")
		if container == "myapp-v1-build-procfile-inspect" {
			w.Write([]byte(`web: my awesome cmd`))
			return
		}
		if container == "myapp-v1-build-yamldata" {
			w.Write([]byte(`healthcheck:
  path: /status
  method: GET
  status: 200
hooks:
  build:
    - ./build1
    - ./build2
  restart:
    before:
      - ./before.sh
    after:
      - ./after.sh`))
			return
		}
		w.Write([]byte(`[{"Config": {"Cmd": null, "Entrypoint": null, "ExposedPorts": null}}]`))
	}
	bopts := builder.BuildOpts{
		ImageID: "test/customimage",
	}
	img, err := s.b.Build(s.p, a, evt, &bopts)
	c.Assert(err, check.IsNil, check.Commentf("%+v", err))
	c.Assert(img, check.Equals, "tsuru/app-myapp:v1")
	imd, err := image.GetImageMetaData(img)
	c.Assert(err, check.IsNil)
	c.Assert(imd.CustomData, check.DeepEquals, map[string]interface{}{
		"healthcheck": map[string]interface{}{
			"path":   "/status",
			"method": "GET",
			"status": 200,
		},
		"hooks": map[string]interface{}{
			"build": []interface{}{"./build1", "./build2"},
			"restart": map[string]interface{}{
				"before": []interface{}{"./before.sh"},
				"after":  []interface{}{"./after.sh"},
			},
		},
	})
}
