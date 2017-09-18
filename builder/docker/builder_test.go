// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"

	"github.com/docker/docker/pkg/stdcopy"
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
	stopCh := s.stopContainers(s.server.URL(), 1)
	defer func() { <-stopCh }()
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
	stopCh := s.stopContainers(s.server.URL(), 1)
	defer func() { <-stopCh }()
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

func (s *S) TestBuilderImageID(c *check.C) {
	opts := provision.AddNodeOptions{Address: s.server.URL()}
	err := s.provisioner.AddNode(opts)
	c.Assert(err, check.IsNil)
	a := &app.App{Name: "myapp", Platform: "whitespace", TeamOwner: s.team.Name}
	err = app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	u, _ := url.Parse(s.server.URL())
	imageName := fmt.Sprintf("%s/%s", u.Host, "customimage")
	config.Set("docker:registry", u.Host)
	defer config.Unset("docker:registry")
	s.server.CustomHandler("/containers/.*/attach", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "cannot hijack connection", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/vnd.docker.raw-stream")
		w.WriteHeader(http.StatusOK)
		conn, _, cErr := hijacker.Hijack()
		if cErr != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		outStream := stdcopy.NewStdWriter(conn, stdcopy.Stdout)
		fmt.Fprintf(outStream, "")
		conn.Close()
	}))
	s.server.CustomHandler(fmt.Sprintf("/images/%s/json", imageName), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := docker.Image{
			Config: &docker.Config{
				Entrypoint: []string{"/bin/sh", "-c", "python test.py"},
			},
		}
		j, _ := json.Marshal(response)
		w.Write(j)
	}))
	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)
	bopts := builder.BuildOpts{
		ImageID: imageName,
	}
	imgID, err := s.b.Build(s.provisioner, a, evt, bopts)
	c.Assert(err, check.IsNil)
	c.Assert(imgID, check.Equals, u.Host+"/tsuru/app-myapp:v1")
	imd, err := image.GetImageMetaData(imgID)
	c.Assert(err, check.IsNil)
	expectedProcesses := map[string][]string{"web": {"/bin/sh", "-c", "python test.py"}}
	c.Assert(imd.Processes, check.DeepEquals, expectedProcesses)
}

func (s *S) TestBuilderImageIDWithExposedPort(c *check.C) {
	stopCh := s.stopContainers(s.server.URL(), 1)
	defer func() { <-stopCh }()
	opts := provision.AddNodeOptions{Address: s.server.URL()}
	err := s.provisioner.AddNode(opts)
	c.Assert(err, check.IsNil)
	a := &app.App{Name: "myapp", Platform: "whitespace", TeamOwner: s.team.Name}
	err = app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	u, _ := url.Parse(s.server.URL())
	imageName := fmt.Sprintf("%s/%s", u.Host, "customimage")
	config.Set("docker:registry", u.Host)
	defer config.Unset("docker:registry")
	s.server.CustomHandler("/containers/.*/attach", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "cannot hijack connection", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/vnd.docker.raw-stream")
		w.WriteHeader(http.StatusOK)
		conn, _, cErr := hijacker.Hijack()
		if cErr != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		outStream := stdcopy.NewStdWriter(conn, stdcopy.Stdout)
		fmt.Fprintf(outStream, "")
		conn.Close()
	}))
	s.server.CustomHandler(fmt.Sprintf("/images/%s/json", imageName), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := docker.Image{
			Config: &docker.Config{
				Entrypoint:   []string{"/bin/sh", "-c", "python test.py"},
				ExposedPorts: map[docker.Port]struct{}{"80/tcp": {}},
			},
		}
		j, _ := json.Marshal(response)
		w.Write(j)
	}))
	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)
	bopts := builder.BuildOpts{
		ImageID: imageName,
	}
	imgID, err := s.b.Build(s.provisioner, a, evt, bopts)
	c.Assert(err, check.IsNil)
	c.Assert(imgID, check.Equals, u.Host+"/tsuru/app-myapp:v1")
	imd, err := image.GetImageMetaData(imgID)
	c.Assert(err, check.IsNil)
	c.Assert(imd.ExposedPort, check.DeepEquals, "80/tcp")
}

func (s *S) TestBuilderImageIDMoreThanOnePortFromImage(c *check.C) {
	opts := provision.AddNodeOptions{Address: s.server.URL()}
	err := s.provisioner.AddNode(opts)
	c.Assert(err, check.IsNil)
	a := &app.App{Name: "myapp", Platform: "whitespace", TeamOwner: s.team.Name}
	err = app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	u, _ := url.Parse(s.server.URL())
	imageName := fmt.Sprintf("%s/%s", u.Host, "customimage")
	config.Set("docker:registry", u.Host)
	defer config.Unset("docker:registry")
	s.server.CustomHandler("/containers/.*/attach", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "cannot hijack connection", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/vnd.docker.raw-stream")
		w.WriteHeader(http.StatusOK)
		conn, _, cErr := hijacker.Hijack()
		if cErr != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		outStream := stdcopy.NewStdWriter(conn, stdcopy.Stdout)
		fmt.Fprintf(outStream, "")
		conn.Close()
	}))
	s.server.CustomHandler(fmt.Sprintf("/images/%s/json", imageName), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := docker.Image{
			Config: &docker.Config{
				Entrypoint:   []string{"/bin/sh", "-c", "python test.py"},
				ExposedPorts: map[docker.Port]struct{}{"3000/tcp": {}, "80/tcp": {}},
			},
		}
		j, _ := json.Marshal(response)
		w.Write(j)
	}))
	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)
	bopts := builder.BuildOpts{
		ImageID: imageName,
	}
	_, err = s.b.Build(s.provisioner, a, evt, bopts)
	c.Assert(err, check.NotNil)
}

func (s *S) TestBuilderImageIDMWithProcfile(c *check.C) {
	opts := provision.AddNodeOptions{Address: s.server.URL()}
	err := s.provisioner.AddNode(opts)
	c.Assert(err, check.IsNil)
	a := &app.App{Name: "myapp", Platform: "whitespace", TeamOwner: s.team.Name}
	err = app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	u, _ := url.Parse(s.server.URL())
	imageName := fmt.Sprintf("%s/%s", u.Host, "customimage")
	config.Set("docker:registry", u.Host)
	defer config.Unset("docker:registry")
	s.server.CustomHandler("/containers/.*/attach", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "cannot hijack connection", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/vnd.docker.raw-stream")
		w.WriteHeader(http.StatusOK)
		conn, _, cErr := hijacker.Hijack()
		if cErr != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		outStream := stdcopy.NewStdWriter(conn, stdcopy.Stdout)
		fmt.Fprintf(outStream, "web: test.sh\n")
		conn.Close()
	}))
	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: "app", Value: a.Name},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	bopts := builder.BuildOpts{
		ImageID: imageName,
	}
	imgID, err := s.b.Build(s.provisioner, a, evt, bopts)
	c.Assert(err, check.IsNil)
	c.Assert(imgID, check.Equals, u.Host+"/tsuru/app-myapp:v1")
	imd, err := image.GetImageMetaData(imgID)
	c.Assert(err, check.IsNil)
	expectedProcesses := map[string][]string{"web": {"test.sh"}}
	c.Assert(imd.Processes, check.DeepEquals, expectedProcesses)
}

func (s *S) TestBuilderImageIDMWithEntrypointAndCmd(c *check.C) {
	opts := provision.AddNodeOptions{Address: s.server.URL()}
	err := s.provisioner.AddNode(opts)
	c.Assert(err, check.IsNil)
	a := &app.App{Name: "myapp", Platform: "whitespace", TeamOwner: s.team.Name}
	err = app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	u, _ := url.Parse(s.server.URL())
	imageName := fmt.Sprintf("%s/%s", u.Host, "customimage")
	config.Set("docker:registry", u.Host)
	defer config.Unset("docker:registry")
	s.server.CustomHandler("/containers/.*/attach", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "cannot hijack connection", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/vnd.docker.raw-stream")
		w.WriteHeader(http.StatusOK)
		conn, _, cErr := hijacker.Hijack()
		if cErr != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		outStream := stdcopy.NewStdWriter(conn, stdcopy.Stdout)
		fmt.Fprintf(outStream, "")
		conn.Close()
	}))
	s.server.CustomHandler("/images/.*/json", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := docker.Image{
			Config: &docker.Config{
				Entrypoint: []string{"/bin/sh", "-c"},
				Cmd:        []string{"python test.py"},
			},
		}
		j, _ := json.Marshal(response)
		w.Write(j)
	}))
	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)
	bopts := builder.BuildOpts{
		ImageID: imageName,
	}
	imgID, err := s.b.Build(s.provisioner, a, evt, bopts)
	c.Assert(err, check.IsNil)
	c.Assert(imgID, check.Equals, u.Host+"/tsuru/app-myapp:v1")
	imd, err := image.GetImageMetaData(imgID)
	c.Assert(err, check.IsNil)
	expectedProcesses := map[string][]string{"web": {"/bin/sh", "-c", "python test.py"}}
	c.Assert(imd.Processes, check.DeepEquals, expectedProcesses)
}

func (s *S) TestBuilderRebuild(c *check.C) {
	stopCh := s.stopContainers(s.server.URL(), 2)
	defer func() { <-stopCh }()
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
	stopCh := s.stopContainers(s.server.URL(), 2)
	defer func() { <-stopCh }()
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
	fakeHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "fake image")
	})
	fakeServer := httptest.NewServer(fakeHandler)
	defer fakeServer.Close()
	buildOpts := builder.BuildOpts{
		ArchiveURL: fakeServer.URL,
	}
	_, err = s.b.Build(s.provisioner, a, evt, buildOpts)
	c.Assert(err, check.IsNil)
	dclient, err := docker.NewClient(s.server.URL())
	c.Assert(err, check.IsNil)
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
	c.Assert(imgs, check.HasLen, 3)
	c.Assert(imgs[1].RepoTags, check.HasLen, 1)
	c.Assert(imgs[2].RepoTags, check.HasLen, 1)
	got = []string{imgs[1].RepoTags[0], imgs[2].RepoTags[0]}
	sort.Strings(got)
	expected = []string{"tsuru/app-myapp:v2-builder", "tsuru/python:latest"}
	c.Assert(got, check.DeepEquals, expected)
}
