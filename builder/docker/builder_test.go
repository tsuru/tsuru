// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"sync/atomic"

	"github.com/docker/docker/pkg/stdcopy"
	docker "github.com/fsouza/go-dockerclient"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/builder"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
	provTypes "github.com/tsuru/tsuru/types/provision"
	check "gopkg.in/check.v1"
)

func (s *S) TestBuilderArchiveURL(c *check.C) {
	stopCh := s.stopContainers(s.server.URL(), 1)
	defer func() { <-stopCh }()
	opts := provision.AddNodeOptions{Address: s.server.URL()}
	err := s.provisioner.AddNode(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	a := &app.App{Name: "myapp", Platform: "whitespace", TeamOwner: s.team.Name}
	err = app.CreateApp(context.TODO(), a, s.user)
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
	version, err := s.b.Build(context.TODO(), s.provisioner, a, evt, &bopts)
	c.Assert(err, check.IsNil)
	testBuildImage, err := version.BuildImageName()
	c.Assert(err, check.IsNil)
	c.Assert(testBuildImage, check.Equals, s.team.Name+"/app-myapp:v1-builder")
	c.Assert(version.VersionInfo().BuildImage, check.Equals, testBuildImage)
	c.Assert(version.VersionInfo().DeployImage, check.Equals, "")
}

func (s *S) TestBuilderArchiveURLEmptyFile(c *check.C) {
	opts := provision.AddNodeOptions{Address: s.server.URL()}
	err := s.provisioner.AddNode(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	a := &app.App{Name: "myapp", Platform: "whitespace", TeamOwner: s.team.Name}
	err = app.CreateApp(context.TODO(), a, s.user)
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
	version, err := s.b.Build(context.TODO(), s.provisioner, a, evt, &bopts)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, `archive file is empty`)
	c.Assert(version, check.IsNil)
}

func (s *S) TestBuilderArchiveFile(c *check.C) {
	stopCh := s.stopContainers(s.server.URL(), 1)
	defer func() { <-stopCh }()
	opts := provision.AddNodeOptions{Address: s.server.URL()}
	err := s.provisioner.AddNode(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	a := &app.App{Name: "myapp", Platform: "whitespace", TeamOwner: s.team.Name}
	err = app.CreateApp(context.TODO(), a, s.user)
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
	version, err := s.b.Build(context.TODO(), s.provisioner, a, evt, &bopts)
	c.Assert(err, check.IsNil)
	testBuildImage, err := version.BuildImageName()
	c.Assert(err, check.IsNil)
	c.Assert(testBuildImage, check.Equals, s.team.Name+"/app-myapp:v1-builder")
	c.Assert(version.VersionInfo().BuildImage, check.Equals, testBuildImage)
	c.Assert(version.VersionInfo().DeployImage, check.Equals, "")
}

func (s *S) TestBuilderImageID(c *check.C) {
	opts := provision.AddNodeOptions{Address: s.server.URL()}
	err := s.provisioner.AddNode(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	a := &app.App{Name: "myapp", Platform: "whitespace", TeamOwner: s.team.Name}
	err = app.CreateApp(context.TODO(), a, s.user)
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
	s.server.CustomHandler(fmt.Sprintf("/images/%s/json", imageName+":latest"), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := docker.Image{
			Config: &docker.Config{
				Entrypoint: []string{"/bin/sh", "-c", "python test.py"},
			},
		}
		j, _ := json.Marshal(response)
		w.Write(j)
	}))
	var containerDeleteCount int32
	var createCmds []string
	s.server.CustomHandler("/containers/[^/]+$", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			atomic.AddInt32(&containerDeleteCount, 1)
		}
		if r.Method == http.MethodPost {
			data, _ := ioutil.ReadAll(r.Body)
			var parsed struct {
				Cmd []string
			}
			jsonErr := json.Unmarshal(data, &parsed)
			c.Assert(jsonErr, check.IsNil)
			createCmds = append(createCmds, parsed.Cmd...)
			r.Body = ioutil.NopCloser(bytes.NewReader(data))
		}
		s.server.DefaultHandler().ServeHTTP(w, r)
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
	version, err := s.b.Build(context.TODO(), s.provisioner, a, evt, &bopts)
	c.Assert(err, check.IsNil)
	testBaseImage, err := version.BaseImageName()
	c.Assert(err, check.IsNil)
	c.Assert(testBaseImage, check.Equals, u.Host+"/tsuru/app-myapp:v1")
	processes, err := version.Processes()
	c.Assert(err, check.IsNil)
	expectedProcesses := map[string][]string{"web": {"/bin/sh", "-c", "python test.py"}}
	c.Assert(processes, check.DeepEquals, expectedProcesses)
	c.Assert(atomic.LoadInt32(&containerDeleteCount), check.Equals, int32(2))
	c.Assert(createCmds, check.DeepEquals, []string{
		"(cat /home/application/current/Procfile || cat /app/user/Procfile || cat /Procfile || true) 2>/dev/null",
		"(cat /home/application/current/tsuru.yml || cat /app/user/tsuru.yml || cat /tsuru.yml || cat /home/application/current/tsuru.yaml || cat /app/user/tsuru.yaml || cat /tsuru.yaml || cat /home/application/current/app.yml || cat /app/user/app.yml || cat /app.yml || cat /home/application/current/app.yaml || cat /app/user/app.yaml || cat /app.yaml || true) 2>/dev/null",
	})

}

func (s *S) TestBuilderImageIDWithMoreThanOnePort(c *check.C) {
	opts := provision.AddNodeOptions{Address: s.server.URL()}
	err := s.provisioner.AddNode(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	a := &app.App{Name: "myapp", Platform: "whitespace", TeamOwner: s.team.Name}
	err = app.CreateApp(context.TODO(), a, s.user)
	c.Assert(err, check.IsNil)
	u, _ := url.Parse(s.server.URL())
	imageName := fmt.Sprintf("%s/%s", u.Host, "customimage:test")
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
				ExposedPorts: map[docker.Port]struct{}{"3000/tcp": {}, "8080/tcp": {}},
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
	_, err = s.b.Build(context.TODO(), s.provisioner, a, evt, &bopts)
	c.Assert(err, check.NotNil)
}

func (s *S) TestBuilderImageIDWithExposedPort(c *check.C) {
	stopCh := s.stopContainers(s.server.URL(), 2)
	defer func() { <-stopCh }()
	opts := provision.AddNodeOptions{Address: s.server.URL()}
	err := s.provisioner.AddNode(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	a := &app.App{Name: "myapp", Platform: "whitespace", TeamOwner: s.team.Name}
	err = app.CreateApp(context.TODO(), a, s.user)
	c.Assert(err, check.IsNil)
	u, _ := url.Parse(s.server.URL())
	imageName := fmt.Sprintf("%s/%s", u.Host, "customimage:latest")
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
	version, err := s.b.Build(context.TODO(), s.provisioner, a, evt, &bopts)
	c.Assert(err, check.IsNil)
	testBaseImage, err := version.BaseImageName()
	c.Assert(err, check.IsNil)
	c.Assert(testBaseImage, check.Equals, u.Host+"/tsuru/app-myapp:v1")
	c.Assert(version.VersionInfo().ExposedPorts, check.DeepEquals, []string{"80/tcp"})
}

func (s *S) TestBuilderImageIDWithProcfile(c *check.C) {
	opts := provision.AddNodeOptions{Address: s.server.URL()}
	err := s.provisioner.AddNode(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	a := &app.App{Name: "myapp", Platform: "whitespace", TeamOwner: s.team.Name}
	err = app.CreateApp(context.TODO(), a, s.user)
	c.Assert(err, check.IsNil)
	u, _ := url.Parse(s.server.URL())
	imageName := fmt.Sprintf("%s/%s", u.Host, "customimage:latest")
	config.Set("docker:registry", u.Host)
	defer config.Unset("docker:registry")
	var attachCounter int32
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
		if atomic.AddInt32(&attachCounter, 1) == 1 {
			fmt.Fprintf(outStream, "web: test.sh\n")
		} else {
			fmt.Fprintf(outStream, "")
		}
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
	version, err := s.b.Build(context.TODO(), s.provisioner, a, evt, &bopts)
	c.Assert(err, check.IsNil)
	testBaseImage, err := version.BaseImageName()
	c.Assert(err, check.IsNil)
	c.Assert(testBaseImage, check.Equals, u.Host+"/tsuru/app-myapp:v1")
	processes, err := version.Processes()
	c.Assert(err, check.IsNil)
	expectedProcesses := map[string][]string{"web": {"test.sh"}}
	c.Assert(processes, check.DeepEquals, expectedProcesses)
}

func (s *S) TestBuilderImageIDWithEntrypointAndCmd(c *check.C) {
	opts := provision.AddNodeOptions{Address: s.server.URL()}
	err := s.provisioner.AddNode(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	a := &app.App{Name: "myapp", Platform: "whitespace", TeamOwner: s.team.Name}
	err = app.CreateApp(context.TODO(), a, s.user)
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
	version, err := s.b.Build(context.TODO(), s.provisioner, a, evt, &bopts)
	c.Assert(err, check.IsNil)
	testBaseImage, err := version.BaseImageName()
	c.Assert(err, check.IsNil)
	c.Assert(testBaseImage, check.Equals, u.Host+"/tsuru/app-myapp:v1")
	processes, err := version.Processes()
	c.Assert(err, check.IsNil)
	expectedProcesses := map[string][]string{"web": {"/bin/sh", "-c", "python test.py"}}
	c.Assert(processes, check.DeepEquals, expectedProcesses)
}

func (s *S) TestBuilderImageIDWithTsuruYaml(c *check.C) {
	opts := provision.AddNodeOptions{Address: s.server.URL()}
	err := s.provisioner.AddNode(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	a := &app.App{Name: "myapp", Platform: "whitespace", TeamOwner: s.team.Name}
	err = app.CreateApp(context.TODO(), a, s.user)
	c.Assert(err, check.IsNil)
	u, _ := url.Parse(s.server.URL())
	imageName := fmt.Sprintf("%s/%s", u.Host, "customimage:latest")
	config.Set("docker:registry", u.Host)
	defer config.Unset("docker:registry")
	var attachCounter int32
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
		if atomic.AddInt32(&attachCounter, 1) == 2 {
			yamlData := `healthcheck:
  path: /status
  method: GET
  status: 200
  scheme: https
hooks:
  build:
    - ./build1
    - ./build2
  restart:
    before:
      - ./before.sh
    after:
      - ./after.sh`
			fmt.Fprint(outStream, yamlData)
		}
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
	version, err := s.b.Build(context.TODO(), s.provisioner, a, evt, &bopts)
	c.Assert(err, check.IsNil)
	testBaseImage, err := version.BaseImageName()
	c.Assert(err, check.IsNil)
	c.Assert(testBaseImage, check.Equals, u.Host+"/tsuru/app-myapp:v1")
	customdata, err := version.TsuruYamlData()
	c.Assert(err, check.IsNil)
	c.Assert(customdata, check.DeepEquals, provTypes.TsuruYamlData{
		Healthcheck: &provTypes.TsuruYamlHealthcheck{
			Path:   "/status",
			Method: "GET",
			Status: 200,
			Scheme: "https",
		},
		Hooks: &provTypes.TsuruYamlHooks{
			Build: []string{"./build1", "./build2"},
			Restart: provTypes.TsuruYamlRestartHooks{
				Before: []string{"./before.sh"},
				After:  []string{"./after.sh"},
			},
		},
	})
}

func (s *S) TestBuilderImageIDWithHooks(c *check.C) {
	opts := provision.AddNodeOptions{Address: s.server.URL()}
	err := s.provisioner.AddNode(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	a := &app.App{Name: "myapp", Platform: "whitespace", TeamOwner: s.team.Name}
	err = app.CreateApp(context.TODO(), a, s.user)
	c.Assert(err, check.IsNil)
	u, _ := url.Parse(s.server.URL())
	imageName := fmt.Sprintf("%s/%s:v1", u.Host, "customimage")
	config.Set("docker:registry", u.Host)
	defer config.Unset("docker:registry")
	var attachCounter int32
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
		switch atomic.AddInt32(&attachCounter, 1) {
		case 1:
			// cat Procfile call
		case 2:
			// cat tsuru.yaml call
			yamlData := `hooks:
  build:
    - echo "running build hook"`
			fmt.Fprint(outStream, yamlData)
		case 3:
			// Run hook
			fmt.Fprint(outStream, "running build hook\n")
		}
		conn.Close()
	}))
	s.server.CustomHandler("/images/.*/json", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.Assert(r.URL.Path, check.Equals, "/images/"+imageName+"/json")
		response := docker.Image{
			Config: &docker.Config{
				Entrypoint: []string{"/bin/sh", "-c", "python test.py"},
			},
		}
		j, _ := json.Marshal(response)
		w.Write(j)
	}))
	s.server.CustomHandler("/commit", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reg *regexp.Regexp
		reg, err = regexp.Compile("https?://(.*$)")
		c.Assert(err, check.IsNil)
		m := reg.FindStringSubmatch(s.server.URL())
		c.Assert(m, check.HasLen, 2)
		c.Assert(r.URL.Query().Get("repo"), check.Equals, m[1]+"customimage")
		c.Assert(r.URL.Query().Get("tag"), check.Equals, "v1")
		s.server.DefaultHandler().ServeHTTP(w, r)
	}))
	var containerDeleteCount int32
	s.server.CustomHandler("/containers/[^/]+$", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			atomic.AddInt32(&containerDeleteCount, 1)
		}
		s.server.DefaultHandler().ServeHTTP(w, r)
	}))
	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)
	var logBuffer bytes.Buffer
	evt.SetLogWriter(&logBuffer)
	bopts := builder.BuildOpts{
		ImageID: imageName,
	}
	version, err := s.b.Build(context.TODO(), s.provisioner, a, evt, &bopts)
	c.Assert(err, check.IsNil)
	testBaseImage, err := version.BaseImageName()
	c.Assert(err, check.IsNil)
	c.Assert(testBaseImage, check.Equals, u.Host+"/tsuru/app-myapp:v1")
	c.Assert(version.VersionInfo().BuildImage, check.Equals, "")
	c.Assert(version.VersionInfo().DeployImage, check.Equals, testBaseImage)
	c.Assert(logBuffer.String(), check.Matches, `(?s).*---> Running "echo \\"running build hook\\"".+running build hook.*`)
	c.Assert(atomic.LoadInt32(&containerDeleteCount), check.Equals, int32(3))
}

func (s *S) TestBuilderRebuild(c *check.C) {
	stopCh := s.stopContainers(s.server.URL(), 2)
	defer func() { <-stopCh }()
	opts := provision.AddNodeOptions{Address: s.server.URL()}
	err := s.provisioner.AddNode(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	a := &app.App{Name: "myapp", Platform: "whitespace", TeamOwner: s.team.Name}
	err = app.CreateApp(context.TODO(), a, s.user)
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
	version, err := s.b.Build(context.TODO(), s.provisioner, a, evt, &bopts)
	c.Assert(err, check.IsNil)
	testBuildImage, err := version.BuildImageName()
	c.Assert(err, check.IsNil)
	c.Assert(testBuildImage, check.Equals, s.team.Name+"/app-myapp:v1-builder")
	err = version.CommitBaseImage()
	c.Assert(err, check.IsNil)
	err = version.CommitSuccessful()
	c.Assert(err, check.IsNil)

	s.server.CustomHandler("/containers/[^/]+/archive$", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			path := r.URL.Query().Get("path")
			c.Assert(path, check.Equals, "/home/application/archive.tar.gz")
			w.Header().Set("Content-Type", "application/x-tar")
			w.WriteHeader(http.StatusOK)
			return
		}
		s.server.DefaultHandler().ServeHTTP(w, r)
	}))

	bopts = builder.BuildOpts{
		Rebuild: true,
	}
	version, err = s.b.Build(context.TODO(), s.provisioner, a, evt, &bopts)
	c.Assert(err, check.IsNil)
	testBuildImagev2, err := version.BuildImageName()
	c.Assert(err, check.IsNil)
	c.Assert(testBuildImagev2, check.Equals, s.team.Name+"/app-myapp:v2-builder")
}

func (s *S) TestBuilderImageBuilded(c *check.C) {
	opts := provision.AddNodeOptions{Address: s.server.URL()}
	err := s.provisioner.AddNode(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	a := &app.App{Name: "myapp", Platform: "whitespace", TeamOwner: s.team.Name}
	err = app.CreateApp(context.TODO(), a, s.user)
	c.Assert(err, check.IsNil)
	u, _ := url.Parse(s.server.URL())
	imageName := fmt.Sprintf("%s/%s", u.Host, "customimage:latest")
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
				Labels:     map[string]string{"is-tsuru": "true"},
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
	version, err := s.b.Build(context.TODO(), s.provisioner, a, evt, &bopts)
	c.Assert(err, check.IsNil)
	testBasImage, err := version.BaseImageName()
	c.Assert(err, check.IsNil)
	c.Assert(testBasImage, check.Equals, u.Host+"/tsuru/app-myapp:v1")
	c.Assert(version.VersionInfo().BuildImage, check.Equals, "")
	c.Assert(version.VersionInfo().DeployImage, check.Equals, testBasImage)
	c.Assert(bopts.IsTsuruBuilderImage, check.Equals, true)
}
