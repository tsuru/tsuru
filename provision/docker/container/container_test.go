// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package container

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/fsouza/go-dockerclient"
	"github.com/tsuru/config"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/app/bind"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/router/routertest"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

func (s *S) TestContainerShortID(c *check.C) {
	container := Container{ID: "abc123"}
	c.Check(container.ShortID(), check.Equals, container.ID)
	container.ID = "abcdef123456"
	c.Check(container.ShortID(), check.Equals, "abcdef1234")
}

func (s *S) TestContainerAvailable(c *check.C) {
	var tests = []struct {
		Input    string
		Expected bool
	}{
		{provision.StatusBuilding.String(), false},
		{provision.StatusCreated.String(), false},
		{provision.StatusError.String(), false},
		{provision.StatusStarted.String(), true},
		{provision.StatusStarting.String(), true},
		{provision.StatusStopped.String(), false},
	}
	var container Container
	for _, t := range tests {
		container.Status = t.Input
		c.Check(container.Available(), check.Equals, t.Expected)
	}
}

func (s *S) TestContainerAddress(c *check.C) {
	container := Container{ID: "id123", HostAddr: "10.10.10.10", HostPort: "49153"}
	address := container.Address()
	expected := "http://10.10.10.10:49153"
	c.Assert(address.String(), check.Equals, expected)
}

func (s *S) TestContainerCreate(c *check.C) {
	s.server.CustomHandler("/images/.*/json", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := docker.Image{
			Config: &docker.Config{
				ExposedPorts: map[docker.Port]struct{}{},
			},
		}
		j, _ := json.Marshal(response)
		w.Write(j)
	}))
	config.Set("host", "my.cool.tsuru.addr:8080")
	defer config.Unset("host")
	app := provisiontest.NewFakeApp("app-name", "brainfuck", 1)
	app.Memory = 15
	app.Swap = 15
	app.CpuShare = 50
	app.SetEnv(bind.EnvVar{Name: "A", Value: "myenva"})
	app.SetEnv(bind.EnvVar{Name: "ABCD", Value: "other env"})
	routertest.FakeRouter.AddBackend(app.GetName())
	defer routertest.FakeRouter.RemoveBackend(app.GetName())
	img := "tsuru/brainfuck:latest"
	s.p.Cluster().PullImage(docker.PullImageOptions{Repository: img}, docker.AuthConfiguration{})
	cont := Container{
		Name:        "myName",
		AppName:     app.GetName(),
		Type:        app.GetPlatform(),
		Status:      "created",
		ProcessName: "myprocess1",
		ExposedPort: "8888/tcp",
	}
	err := cont.Create(&CreateArgs{
		App:         app,
		ImageID:     img,
		Commands:    []string{"docker", "run"},
		Provisioner: s.p,
	})
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(&cont)
	c.Assert(cont.ID, check.Not(check.Equals), "")
	c.Assert(cont.AppName, check.Equals, app.GetName())
	c.Assert(cont.Type, check.Equals, app.GetPlatform())
	u, _ := url.Parse(s.server.URL())
	host, _, _ := net.SplitHostPort(u.Host)
	c.Assert(cont.HostAddr, check.Equals, host)
	dcli, _ := docker.NewClient(s.server.URL())
	container, err := dcli.InspectContainer(cont.ID)
	c.Assert(err, check.IsNil)
	c.Assert(container.Path, check.Equals, "docker")
	c.Assert(container.Args, check.DeepEquals, []string{"run"})
	c.Assert(container.Config.Memory, check.Equals, app.Memory)
	c.Assert(container.Config.MemorySwap, check.Equals, app.Memory+app.Swap)
	c.Assert(container.Config.CPUShares, check.Equals, int64(app.CpuShare))
	sort.Strings(container.Config.Env)
	c.Assert(container.Config.Labels, check.DeepEquals, map[string]string{
		"tsuru.container":    "true",
		"tsuru.router.name":  "fake",
		"tsuru.router.type":  "fakeType",
		"tsuru.app.name":     "app-name",
		"tsuru.app.platform": "brainfuck",
		"tsuru.process.name": "myprocess1",
	})
	c.Assert(container.Config.Env, check.DeepEquals, []string{
		"A=myenva",
		"ABCD=other env",
		"PORT=8888",
		"TSURU_HOST=my.cool.tsuru.addr:8080",
		"TSURU_PROCESSNAME=myprocess1",
		"port=8888",
	})
}

func (s *S) TestContainerCreateAllocatesPort(c *check.C) {
	s.server.CustomHandler("/images/.*/json", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := docker.Image{
			Config: &docker.Config{
				ExposedPorts: map[docker.Port]struct{}{"3000/tcp": {}},
			},
		}
		j, _ := json.Marshal(response)
		w.Write(j)
	}))
	config.Set("host", "my.cool.tsuru.addr:8080")
	defer config.Unset("host")
	app := provisiontest.NewFakeApp("app-name", "brainfuck", 1)
	app.Memory = 15
	app.Swap = 15
	app.CpuShare = 50
	routertest.FakeRouter.AddBackend(app.GetName())
	defer routertest.FakeRouter.RemoveBackend(app.GetName())
	img := "tsuru/brainfuck:latest"
	s.p.Cluster().PullImage(docker.PullImageOptions{Repository: img}, docker.AuthConfiguration{})
	cont := Container{
		Name:        "myName",
		AppName:     app.GetName(),
		Type:        app.GetPlatform(),
		Status:      "created",
		ProcessName: "myprocess1",
		ExposedPort: "3000/tcp",
	}
	err := cont.Create(&CreateArgs{
		App:         app,
		Commands:    []string{"docker", "run"},
		Provisioner: s.p,
		ImageID:     img,
	})
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(&cont)
	dcli, _ := docker.NewClient(s.server.URL())
	container, err := dcli.InspectContainer(cont.ID)
	c.Assert(err, check.IsNil)
	c.Assert(container.Config.ExposedPorts, check.DeepEquals, map[docker.Port]struct{}{"3000/tcp": {}})
}

func (s *S) TestContainerCreateSecurityOptions(c *check.C) {
	s.server.CustomHandler("/images/.*/json", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := docker.Image{
			Config: &docker.Config{
				ExposedPorts: map[docker.Port]struct{}{},
			},
		}
		j, _ := json.Marshal(response)
		w.Write(j)
	}))
	config.Set("docker:security-opts", []string{"label:type:svirt_apache", "ptrace peer=@unsecure"})
	defer config.Unset("docker:security-opts")
	app := provisiontest.NewFakeApp("app-name", "brainfuck", 1)
	app.Memory = 15
	app.Swap = 15
	app.CpuShare = 50
	routertest.FakeRouter.AddBackend(app.GetName())
	defer routertest.FakeRouter.RemoveBackend(app.GetName())
	img := "tsuru/brainfuck:latest"
	s.p.Cluster().PullImage(docker.PullImageOptions{Repository: img}, docker.AuthConfiguration{})
	cont := Container{
		Name:    "myName",
		AppName: app.GetName(),
		Type:    app.GetPlatform(),
		Status:  "created",
	}
	err := cont.Create(&CreateArgs{
		App:         app,
		ImageID:     img,
		Commands:    []string{"docker", "run"},
		Provisioner: s.p,
	})
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(&cont)
	dcli, _ := docker.NewClient(s.server.URL())
	container, err := dcli.InspectContainer(cont.ID)
	c.Assert(err, check.IsNil)
	c.Assert(container.Config.SecurityOpts, check.DeepEquals, []string{"label:type:svirt_apache", "ptrace peer=@unsecure"})
}

func (s *S) TestContainerCreateDoesNotAlocatesPortForDeploy(c *check.C) {
	s.server.CustomHandler("/images/.*/json", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := docker.Image{
			Config: &docker.Config{
				ExposedPorts: map[docker.Port]struct{}{},
			},
		}
		j, _ := json.Marshal(response)
		w.Write(j)
	}))
	app := provisiontest.NewFakeApp("app-name", "brainfuck", 1)
	app.Memory = 15
	routertest.FakeRouter.AddBackend(app.GetName())
	defer routertest.FakeRouter.RemoveBackend(app.GetName())
	img := "tsuru/brainfuck:latest"
	s.p.Cluster().PullImage(docker.PullImageOptions{Repository: img}, docker.AuthConfiguration{})
	cont := Container{
		Name:    "myName",
		AppName: app.GetName(),
		Type:    app.GetPlatform(),
		Status:  "created",
	}
	err := cont.Create(&CreateArgs{
		Deploy:      true,
		App:         app,
		ImageID:     img,
		Commands:    []string{"docker", "run"},
		Provisioner: s.p,
	})
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(&cont)
	info, err := cont.NetworkInfo(s.p)
	c.Assert(err, check.IsNil)
	c.Assert(info.HTTPHostPort, check.Equals, "")
}

func (s *S) TestContainerCreateDoesNotSetEnvs(c *check.C) {
	s.server.CustomHandler("/images/.*/json", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := docker.Image{
			Config: &docker.Config{
				ExposedPorts: map[docker.Port]struct{}{},
			},
		}
		j, _ := json.Marshal(response)
		w.Write(j)
	}))
	config.Set("host", "my.cool.tsuru.addr:8080")
	defer config.Unset("host")
	app := provisiontest.NewFakeApp("app-name", "brainfuck", 1)
	app.SetEnv(bind.EnvVar{Name: "A", Value: "myenva"})
	app.SetEnv(bind.EnvVar{Name: "ABCD", Value: "other env"})
	routertest.FakeRouter.AddBackend(app.GetName())
	defer routertest.FakeRouter.RemoveBackend(app.GetName())
	img := "tsuru/brainfuck:latest"
	s.p.Cluster().PullImage(docker.PullImageOptions{Repository: img}, docker.AuthConfiguration{})
	cont := Container{
		Name:    "myName",
		AppName: app.GetName(),
		Type:    app.GetPlatform(),
		Status:  "created",
	}
	err := cont.Create(&CreateArgs{
		Deploy:      true,
		App:         app,
		ImageID:     img,
		Commands:    []string{"docker", "run"},
		Provisioner: s.p,
	})
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(&cont)
	dcli, err := docker.NewClient(s.server.URL())
	c.Assert(err, check.IsNil)
	container, err := dcli.InspectContainer(cont.ID)
	c.Assert(err, check.IsNil)
	sort.Strings(container.Config.Env)
	c.Assert(container.Config.Env, check.DeepEquals, []string{
		"PORT=",
		"TSURU_HOST=my.cool.tsuru.addr:8080",
		"port=",
	})
}

func (s *S) TestContainerCreateUndefinedUser(c *check.C) {
	s.server.CustomHandler("/images/.*/json", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := docker.Image{
			Config: &docker.Config{
				ExposedPorts: map[docker.Port]struct{}{},
			},
		}
		j, _ := json.Marshal(response)
		w.Write(j)
	}))
	oldUser, _ := config.Get("docker:user")
	defer config.Set("docker:user", oldUser)
	config.Unset("docker:user")
	img := "tsuru/python:latest"
	s.p.Cluster().PullImage(docker.PullImageOptions{Repository: img}, docker.AuthConfiguration{})
	app := provisiontest.NewFakeApp("app-name", "python", 1)
	routertest.FakeRouter.AddBackend(app.GetName())
	defer routertest.FakeRouter.RemoveBackend(app.GetName())
	cont := Container{
		Name:    "myName",
		AppName: app.GetName(),
		Type:    app.GetPlatform(),
		Status:  "created",
	}
	err := cont.Create(&CreateArgs{
		App:         app,
		ImageID:     img,
		Commands:    []string{"docker", "run"},
		Provisioner: s.p,
	})
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(&cont)
	dcli, _ := docker.NewClient(s.server.URL())
	container, err := dcli.InspectContainer(cont.ID)
	c.Assert(err, check.IsNil)
	c.Assert(container.Config.User, check.Equals, "")
}

func (s *S) TestContainerCreateOverwriteEntrypoint(c *check.C) {
	s.server.CustomHandler("/images/.*/json", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := docker.Image{
			Config: &docker.Config{
				ExposedPorts: map[docker.Port]struct{}{},
			},
		}
		j, _ := json.Marshal(response)
		w.Write(j)
	}))
	config.Set("host", "my.cool.tsuru.addr:8080")
	defer config.Unset("host")
	app := provisiontest.NewFakeApp("app-name", "brainfuck", 1)
	img := "tsuru/brainfuck:latest"
	s.p.Cluster().PullImage(docker.PullImageOptions{Repository: img}, docker.AuthConfiguration{})
	cont := Container{
		Name:    "myName",
		AppName: app.GetName(),
		Type:    app.GetPlatform(),
		Status:  "created",
	}
	err := cont.Create(&CreateArgs{
		Deploy:      true,
		App:         app,
		ImageID:     img,
		Commands:    []string{"docker", "run"},
		Provisioner: s.p,
	})
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(&cont)
	dcli, err := docker.NewClient(s.server.URL())
	c.Assert(err, check.IsNil)
	container, err := dcli.InspectContainer(cont.ID)
	c.Assert(err, check.IsNil)
	c.Assert(container.Config.Entrypoint, check.DeepEquals, []string{})
}

func (s *S) TestContainerNetworkInfo(c *check.C) {
	inspectOut := `{
	"NetworkSettings": {
		"IpAddress": "10.10.10.10",
		"IpPrefixLen": 8,
		"Gateway": "10.65.41.1",
		"Ports": {}
	}
}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/containers/") {
			w.Write([]byte(inspectOut))
		}
	}))
	defer server.Close()
	var storage cluster.MapStorage
	storage.StoreContainer("c-01", server.URL)
	p, err := newFakeDockerProvisioner(server.URL)
	c.Assert(err, check.IsNil)
	p.cluster, err = cluster.New(nil, &storage,
		cluster.Node{Address: server.URL},
	)
	c.Assert(err, check.IsNil)
	container := Container{ID: "c-01"}
	info, err := container.NetworkInfo(p)
	c.Assert(err, check.IsNil)
	c.Assert(info.IP, check.Equals, "10.10.10.10")
	c.Assert(info.HTTPHostPort, check.Equals, "")
}

func (s *S) TestContainerSetStatus(c *check.C) {
	update := time.Date(1989, 2, 2, 14, 59, 32, 0, time.UTC).In(time.UTC)
	container := Container{ID: "something-300", LastStatusUpdate: update}
	coll := s.p.Collection()
	defer coll.Close()
	coll.Insert(container)
	defer coll.Remove(bson.M{"id": container.ID})
	container.SetStatus(s.p, "what?!", true)
	var c2 Container
	err := coll.Find(bson.M{"id": container.ID}).One(&c2)
	c.Assert(err, check.IsNil)
	c.Assert(c2.Status, check.Equals, "what?!")
	lastUpdate := c2.LastStatusUpdate.In(time.UTC).Format(time.RFC822)
	c.Assert(lastUpdate, check.Not(check.DeepEquals), update.Format(time.RFC822))
	c.Assert(c2.LastSuccessStatusUpdate.IsZero(), check.Equals, true)
}

func (s *S) TestContainerSetStatusStarted(c *check.C) {
	container := Container{ID: "telnet"}
	coll := s.p.Collection()
	defer coll.Close()
	err := coll.Insert(container)
	c.Assert(err, check.IsNil)
	defer coll.Remove(bson.M{"id": container.ID})
	err = container.SetStatus(s.p, provision.StatusStarted, true)
	c.Assert(err, check.IsNil)
	var c2 Container
	err = coll.Find(bson.M{"id": container.ID}).One(&c2)
	c.Assert(err, check.IsNil)
	c.Assert(c2.Status, check.Equals, provision.StatusStarted.String())
	c.Assert(c2.LastSuccessStatusUpdate.IsZero(), check.Equals, false)
	c2.LastSuccessStatusUpdate = time.Time{}
	err = coll.Update(bson.M{"id": c2.ID}, c2)
	c.Assert(err, check.IsNil)
	err = c2.SetStatus(s.p, provision.StatusStarting, true)
	c.Assert(err, check.IsNil)
	err = coll.Find(bson.M{"id": container.ID}).One(&c2)
	c.Assert(err, check.IsNil)
	c.Assert(c2.LastSuccessStatusUpdate.IsZero(), check.Equals, false)
}

func (s *S) TestContainerSetStatusBuilding(c *check.C) {
	c1 := Container{
		ID:     "something-300",
		Status: provision.StatusBuilding.String(),
	}
	coll := s.p.Collection()
	defer coll.Close()
	coll.Insert(c1)
	defer coll.Remove(bson.M{"id": c1.ID})
	err := c1.SetStatus(s.p, provision.StatusStarted, true)
	c.Assert(err, check.Equals, mgo.ErrNotFound)
	var c2 Container
	err = coll.Find(bson.M{"id": c1.ID}).One(&c2)
	c.Assert(err, check.IsNil)
	c.Assert(c2.Status, check.Equals, provision.StatusBuilding.String())
	c.Assert(c2.LastStatusUpdate.IsZero(), check.Equals, true)
	c.Assert(c2.LastSuccessStatusUpdate.IsZero(), check.Equals, true)
}

func (s *S) TestContainerSetStatusNoUpdate(c *check.C) {
	c1 := Container{
		ID:     "something-300",
		Status: provision.StatusBuilding.String(),
	}
	coll := s.p.Collection()
	defer coll.Close()
	coll.Insert(c1)
	defer coll.Remove(bson.M{"id": c1.ID})
	err := c1.SetStatus(s.p, provision.StatusStarted, false)
	c.Assert(err, check.IsNil)
}

func (s *S) TestContainerSetImage(c *check.C) {
	container := Container{ID: "something-300"}
	coll := s.p.Collection()
	defer coll.Close()
	coll.Insert(container)
	defer coll.Remove(bson.M{"id": container.ID})
	err := container.SetImage(s.p, "newimage")
	c.Assert(err, check.IsNil)
	var c2 Container
	err = coll.Find(bson.M{"id": container.ID}).One(&c2)
	c.Assert(err, check.IsNil)
	c.Assert(c2.Image, check.Equals, "newimage")
}

func (s *S) TestContainerRemove(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	err = conn.Apps().Insert(app.App{Name: "test-app"})
	c.Assert(err, check.IsNil)
	container, err := s.newContainer(newContainerOpts{AppName: "test-app"}, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(container)
	err = container.Remove(s.p)
	c.Assert(err, check.IsNil)
	coll := s.p.Collection()
	defer coll.Close()
	err = coll.Find(bson.M{"id": container.ID}).One(&container)
	c.Assert(err, check.Equals, mgo.ErrNotFound)
	client, _ := docker.NewClient(s.server.URL())
	_, err = client.InspectContainer(container.ID)
	c.Assert(err, check.NotNil)
	_, ok := err.(*docker.NoSuchContainer)
	c.Assert(ok, check.Equals, true)
}

func (s *S) TestContainerRemoveIgnoreErrors(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	err = conn.Apps().Insert(app.App{Name: "test-app"})
	c.Assert(err, check.IsNil)
	container, err := s.newContainer(newContainerOpts{AppName: "test-app"}, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(container)
	client, _ := docker.NewClient(s.server.URL())
	err = client.RemoveContainer(docker.RemoveContainerOptions{ID: container.ID})
	c.Assert(err, check.IsNil)
	err = container.Remove(s.p)
	c.Assert(err, check.IsNil)
	coll := s.p.Collection()
	defer coll.Close()
	err = coll.Find(bson.M{"id": container.ID}).One(&container)
	c.Assert(err, check.Equals, mgo.ErrNotFound)
}

func (s *S) TestContainerRemoveStopsContainer(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	a := app.App{Name: "test-app"}
	err = conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	container, err := s.newContainer(newContainerOpts{AppName: a.Name}, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(container)
	err = container.Start(&StartArgs{Provisioner: s.p, App: &a})
	c.Assert(err, check.IsNil)
	err = container.Remove(s.p)
	c.Assert(err, check.IsNil)
	coll := s.p.Collection()
	defer coll.Close()
	err = coll.Find(bson.M{"id": container.ID}).One(&container)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "not found")
	client, _ := docker.NewClient(s.server.URL())
	_, err = client.InspectContainer(container.ID)
	c.Assert(err, check.NotNil)
	_, ok := err.(*docker.NoSuchContainer)
	c.Assert(ok, check.Equals, true)
}

func (s *S) TestContainerShell(c *check.C) {
	var urls struct {
		items []url.URL
		sync.Mutex
	}
	s.server.SetHook(func(r *http.Request) {
		urls.Lock()
		urls.items = append(urls.items, *r.URL)
		urls.Unlock()
	})
	defer s.server.SetHook(nil)
	s.server.PrepareExec("*", func() {
		time.Sleep(500e6)
	})
	container, err := s.newContainer(newContainerOpts{}, nil)
	c.Assert(err, check.IsNil)
	var stdout, stderr bytes.Buffer
	stdin := bytes.NewBufferString("")
	err = container.Shell(s.p, stdin, &stdout, &stderr, Pty{Width: 140, Height: 38, Term: "xterm"})
	c.Assert(err, check.IsNil)
	c.Assert(strings.Contains(stdout.String(), ""), check.Equals, true)
	urls.Lock()
	resizeURL := urls.items[len(urls.items)-2]
	urls.Unlock()
	execResizeRegexp := regexp.MustCompile(`^.*/exec/(.*)/resize$`)
	matches := execResizeRegexp.FindStringSubmatch(resizeURL.Path)
	c.Assert(matches, check.HasLen, 2)
	c.Assert(resizeURL.Query().Get("w"), check.Equals, "140")
	c.Assert(resizeURL.Query().Get("h"), check.Equals, "38")
	client, _ := docker.NewClient(s.server.URL())
	exec, err := client.InspectExec(matches[1])
	c.Assert(err, check.IsNil)
	cmd := append([]string{exec.ProcessConfig.EntryPoint}, exec.ProcessConfig.Arguments...)
	c.Assert(cmd, check.DeepEquals, []string{"/usr/bin/env", "TERM=xterm", "bash", "-l"})
}

func (s *S) TestContainerExec(c *check.C) {
	container, err := s.newContainer(newContainerOpts{}, nil)
	c.Assert(err, check.IsNil)
	var stdout, stderr bytes.Buffer
	err = container.Exec(s.p, &stdout, &stderr, "ls", "-lh")
	c.Assert(err, check.IsNil)
}

func (s *S) TestContainerExecErrorCode(c *check.C) {
	s.server.CustomHandler("/exec/.*/json", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ID":"id","ExitCode":9}`))
	}))
	container, err := s.newContainer(newContainerOpts{}, nil)
	c.Assert(err, check.IsNil)
	var stdout, stderr bytes.Buffer
	err = container.Exec(s.p, &stdout, &stderr, "ls", "-lh")
	c.Assert(err, check.DeepEquals, &execErr{code: 9})
}

func (s *S) TestContainerCommit(c *check.C) {
	cont, err := s.newContainer(newContainerOpts{AppName: "myapp"}, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(cont)
	cont.BuildingImage = "tsuru/app-myapp:v1"
	var buf bytes.Buffer
	imageId, err := cont.Commit(s.p, &buf)
	c.Assert(err, check.IsNil)
	repoNamespace, _ := config.GetString("docker:repository-namespace")
	repository := repoNamespace + "/app-" + cont.AppName + ":v1"
	c.Assert(imageId, check.Equals, repository)
}

func (s *S) TestContainerCommitWithRegistry(c *check.C) {
	config.Set("docker:registry-max-try", 1)
	config.Set("docker:registry", "localhost:3030")
	defer config.Unset("docker:registry")
	cont, err := s.newContainer(newContainerOpts{AppName: "myapp"}, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(cont)
	cont.BuildingImage = "localhost:3030/tsuru/app-myapp:v1"
	var buf bytes.Buffer
	imageId, err := cont.Commit(s.p, &buf)
	c.Assert(err, check.IsNil)
	repoNamespace, _ := config.GetString("docker:repository-namespace")
	repository := "localhost:3030/" + repoNamespace + "/app-" + cont.AppName + ":v1"
	c.Assert(imageId, check.Equals, repository)
	expectedPush := push{
		name: "localhost:3030/" + repoNamespace + "/app-" + cont.AppName,
		tag:  "v1",
	}
	c.Assert(s.p.pushes, check.DeepEquals, []push{expectedPush})
}

func (s *S) TestContainerCommitErrorInPush(c *check.C) {
	s.p.failPush(errors.New("first failure"), errors.New("second failure"), errors.New("third failure"))
	config.Set("docker:registry", "localhost:3030")
	defer config.Unset("docker:registry")
	cont, err := s.newContainer(newContainerOpts{}, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(cont)
	cont.BuildingImage = cont.Image
	var buf bytes.Buffer
	_, err = cont.Commit(s.p, &buf)
	c.Assert(err, check.ErrorMatches, ".*third failure$")
}

func (s *S) TestContainerCommitRetryPush(c *check.C) {
	s.p.failPush(errors.New("first failure"), errors.New("second failure"))
	config.Set("docker:registry-max-try", -1)
	config.Set("docker:registry", "localhost:3030")
	defer config.Unset("docker:registry")
	cont, err := s.newContainer(newContainerOpts{}, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(cont)
	cont.BuildingImage = cont.Image
	var buf bytes.Buffer
	_, err = cont.Commit(s.p, &buf)
	c.Assert(err, check.IsNil)
	expectedPush := push{name: "tsuru/python", tag: "latest"}
	c.Assert(s.p.pushes, check.DeepEquals, []push{expectedPush, expectedPush, expectedPush})
}

func (s *S) TestContainerStop(c *check.C) {
	cont, err := s.newContainer(newContainerOpts{}, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(cont)
	client, err := docker.NewClient(s.server.URL())
	c.Assert(err, check.IsNil)
	err = client.StartContainer(cont.ID, nil)
	c.Assert(err, check.IsNil)
	err = cont.Stop(s.p)
	c.Assert(err, check.IsNil)
	dockerContainer, err := s.p.Cluster().InspectContainer(cont.ID)
	c.Assert(err, check.IsNil)
	c.Assert(dockerContainer.State.Running, check.Equals, false)
	c.Assert(cont.Status, check.Equals, provision.StatusStopped.String())
}

func (s *S) TestContainerSleep(c *check.C) {
	cont, err := s.newContainer(newContainerOpts{}, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(cont)
	app := provisiontest.NewFakeApp("myapp", "python", 1)
	err = cont.Start(&StartArgs{
		Provisioner: s.p,
		App:         app,
	})
	c.Assert(err, check.IsNil)
	err = cont.Sleep(s.p)
	c.Assert(err, check.IsNil)
	dockerContainer, err := s.p.Cluster().InspectContainer(cont.ID)
	c.Assert(err, check.IsNil)
	c.Assert(dockerContainer.State.Running, check.Equals, false)
	c.Assert(cont.Status, check.Equals, provision.StatusAsleep.String())
}

func (s *S) TestContainerSleepNotStarted(c *check.C) {
	cont, err := s.newContainer(newContainerOpts{}, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(cont)
	err = cont.Sleep(s.p)
	c.Assert(err, check.NotNil)
}

func (s *S) TestContainerStopReturnsNilWhenContainerAlreadyMarkedAsStopped(c *check.C) {
	cont, err := s.newContainer(newContainerOpts{}, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(cont)
	cont.SetStatus(s.p, provision.StatusStopped, true)
	err = cont.Stop(s.p)
	c.Assert(err, check.IsNil)
}

func (s *S) TestContainerStart(c *check.C) {
	cont, err := s.newContainer(newContainerOpts{}, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(cont)
	client, err := docker.NewClient(s.server.URL())
	c.Assert(err, check.IsNil)
	contPath := fmt.Sprintf("/containers/%s/start", cont.ID)
	defer s.server.CustomHandler(contPath, s.server.DefaultHandler())
	dockerContainer, err := client.InspectContainer(cont.ID)
	c.Assert(err, check.IsNil)
	c.Assert(dockerContainer.State.Running, check.Equals, false)
	app := provisiontest.NewFakeApp("myapp", "python", 1)
	app.Memory = 15
	app.Swap = 15
	app.CpuShare = 10
	err = cont.Start(&StartArgs{
		Provisioner: s.p,
		App:         app,
	})
	c.Assert(err, check.IsNil)
	dockerContainer, err = client.InspectContainer(cont.ID)
	c.Assert(err, check.IsNil)
	c.Assert(dockerContainer.State.Running, check.Equals, true)
	expectedLogOptions := map[string]string{
		"syslog-address": "udp://localhost:1514",
	}
	expectedPortBindings := map[docker.Port][]docker.PortBinding{
		"8888/tcp": {{HostIP: "", HostPort: ""}},
	}
	c.Assert(dockerContainer.HostConfig.RestartPolicy.Name, check.Equals, "always")
	c.Assert(dockerContainer.HostConfig.LogConfig.Type, check.Equals, "syslog")
	c.Assert(dockerContainer.HostConfig.LogConfig.Config, check.DeepEquals, expectedLogOptions)
	c.Assert(dockerContainer.HostConfig.PortBindings, check.DeepEquals, expectedPortBindings)
	c.Assert(dockerContainer.HostConfig.Memory, check.Equals, int64(15))
	c.Assert(dockerContainer.HostConfig.MemorySwap, check.Equals, int64(30))
	c.Assert(dockerContainer.HostConfig.CPUShares, check.Equals, int64(10))
	c.Assert(cont.Status, check.Equals, "starting")
}

func (s *S) TestContainerStartCustomLog(c *check.C) {
	client, err := docker.NewClient(s.server.URL())
	c.Assert(err, check.IsNil)
	app := provisiontest.NewFakeApp("myapp", "python", 1)
	app.Pool = "mypool"
	testCases := []struct {
		name string
		opts map[string]string
	}{
		{"fluentd", map[string]string{
			"fluentd-address": "localhost:24224",
		}},
		{"syslog", map[string]string{
			"syslog-address": "udp://localhost:1514",
		}},
		{"fluentd", map[string]string{
			"fluentd-address": "somewhere:24224",
			"tag":             "x",
		}},
	}
	for _, testData := range testCases {
		cont, err := s.newContainer(newContainerOpts{}, nil)
		c.Assert(err, check.IsNil)
		conf := DockerLogConfig{Driver: testData.name, LogOpts: testData.opts}
		err = conf.Save(app.Pool)
		c.Assert(err, check.IsNil)
		err = cont.Start(&StartArgs{Provisioner: s.p, App: app})
		c.Assert(err, check.IsNil)
		dockerContainer, err := client.InspectContainer(cont.ID)
		c.Assert(err, check.IsNil)
		c.Assert(dockerContainer.HostConfig.LogConfig.Type, check.Equals, testData.name)
		c.Assert(dockerContainer.HostConfig.LogConfig.Config, check.DeepEquals, testData.opts)
	}
}

func (s *S) TestContainerStartDeployContainer(c *check.C) {
	cont, err := s.newContainer(newContainerOpts{}, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(cont)
	client, err := docker.NewClient(s.server.URL())
	c.Assert(err, check.IsNil)
	contPath := fmt.Sprintf("/containers/%s/start", cont.ID)
	defer s.server.CustomHandler(contPath, s.server.DefaultHandler())
	app := provisiontest.NewFakeApp("myapp", "python", 1)
	app.Memory = 15
	app.Swap = 15
	app.CpuShare = 50
	err = cont.Start(&StartArgs{Provisioner: s.p, App: app, Deploy: true})
	c.Assert(err, check.IsNil)
	c.Assert(cont.Status, check.Equals, "building")
	dockerContainer, err := client.InspectContainer(cont.ID)
	c.Assert(err, check.IsNil)
	c.Assert(dockerContainer.HostConfig.RestartPolicy.Name, check.Equals, "")
	c.Assert(dockerContainer.HostConfig.LogConfig.Type, check.Equals, "")
	c.Assert(dockerContainer.HostConfig.Memory, check.Equals, int64(15))
	c.Assert(dockerContainer.HostConfig.MemorySwap, check.Equals, int64(30))
	c.Assert(dockerContainer.HostConfig.CPUShares, check.Equals, int64(50))
}

func (s *S) TestContainerStartStartedUnits(c *check.C) {
	app := provisiontest.NewFakeApp("myapp", "python", 1)
	cont, err := s.newContainer(newContainerOpts{}, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(cont)
	args := StartArgs{Provisioner: s.p, App: app}
	err = cont.Start(&args)
	c.Assert(err, check.IsNil)
	err = cont.Start(&args)
	c.Assert(err, check.NotNil)
}

func (s *S) TestContainerStartTsuruAllocator(c *check.C) {
	config.Set("docker:port-allocator", "tsuru")
	defer config.Unset("docker:port-allocator")
	cont, err := s.newContainer(newContainerOpts{}, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(cont)
	client, err := docker.NewClient(s.server.URL())
	c.Assert(err, check.IsNil)
	contPath := fmt.Sprintf("/containers/%s/start", cont.ID)
	defer s.server.CustomHandler(contPath, s.server.DefaultHandler())
	dockerContainer, err := client.InspectContainer(cont.ID)
	c.Assert(err, check.IsNil)
	c.Assert(dockerContainer.State.Running, check.Equals, false)
	app := provisiontest.NewFakeApp("myapp", "python", 1)
	app.Memory = 15
	app.Swap = 15
	app.CpuShare = 10
	err = cont.Start(&StartArgs{
		Provisioner: s.p,
		App:         app,
	})
	c.Assert(err, check.IsNil)
	dockerContainer, err = client.InspectContainer(cont.ID)
	c.Assert(err, check.IsNil)
	c.Assert(dockerContainer.State.Running, check.Equals, true)
	expectedLogOptions := map[string]string{
		"syslog-address": "udp://localhost:1514",
	}
	expectedPortBindings := map[docker.Port][]docker.PortBinding{
		"8888/tcp": {{HostIP: "", HostPort: fmt.Sprintf("%d", portRangeStart)}},
	}
	c.Assert(dockerContainer.HostConfig.RestartPolicy.Name, check.Equals, "always")
	c.Assert(dockerContainer.HostConfig.LogConfig.Type, check.Equals, "syslog")
	c.Assert(dockerContainer.HostConfig.LogConfig.Config, check.DeepEquals, expectedLogOptions)
	c.Assert(dockerContainer.HostConfig.PortBindings, check.DeepEquals, expectedPortBindings)
	c.Assert(dockerContainer.HostConfig.Memory, check.Equals, int64(15))
	c.Assert(dockerContainer.HostConfig.MemorySwap, check.Equals, int64(30))
	c.Assert(dockerContainer.HostConfig.CPUShares, check.Equals, int64(10))
	c.Assert(cont.Status, check.Equals, "starting")
}

func (s *S) TestContainerLogs(c *check.C) {
	cont, err := s.newContainer(newContainerOpts{}, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(cont)
	var buff bytes.Buffer
	status, err := cont.Logs(s.p, &buff)
	c.Assert(err, check.IsNil)
	c.Assert(status, check.Equals, 0)
	c.Assert(buff.String(), check.Not(check.Equals), "")
}

func (s *S) TestContainerAsUnit(c *check.C) {
	app := provisiontest.NewFakeApp("myapp", "python", 1)
	expected := provision.Unit{
		ID:          "c-id",
		AppName:     "myapp",
		Type:        "python",
		Ip:          "192.168.50.4",
		Status:      provision.StatusBuilding,
		ProcessName: "web",
		Address:     &url.URL{Scheme: "http", Host: "192.168.50.4:8080"},
	}
	container := Container{
		ID:          "c-id",
		HostAddr:    "192.168.50.4",
		HostPort:    "8080",
		ProcessName: "web",
	}
	got := container.AsUnit(app)
	c.Assert(got, check.DeepEquals, expected)
	container.Type = "ruby"
	container.Status = provision.StatusStarted.String()
	got = container.AsUnit(app)
	expected.Status = provision.StatusStarted
	expected.Type = "ruby"
	c.Assert(got, check.DeepEquals, expected)
}

func (s *S) TestSafeAttachWaitContainer(c *check.C) {
	cont, err := s.newContainer(newContainerOpts{}, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(cont)
	var buf bytes.Buffer
	opts := docker.AttachToContainerOptions{
		Container:    cont.ID,
		Logs:         true,
		Stdout:       true,
		Stderr:       true,
		OutputStream: &buf,
		ErrorStream:  &buf,
		Stream:       true,
	}
	status, err := SafeAttachWaitContainer(s.p, opts)
	c.Assert(err, check.IsNil)
	c.Assert(status, check.Equals, 0)
	c.Assert(buf.String(), check.Not(check.Equals), "")
}

func (s *S) TestSafeAttachWaitContainerAttachBlock(c *check.C) {
	oldWait := safeAttachInspectTimeout
	safeAttachInspectTimeout = 500 * time.Millisecond
	defer func() {
		safeAttachInspectTimeout = oldWait
	}()
	cont, err := s.newContainer(newContainerOpts{}, nil)
	c.Assert(err, check.IsNil)
	block := make(chan bool)
	s.server.CustomHandler(fmt.Sprintf("/containers/%s/attach", cont.ID), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-block
	}))
	defer s.removeTestContainer(cont)
	var buf bytes.Buffer
	opts := docker.AttachToContainerOptions{
		Container:    cont.ID,
		Logs:         true,
		Stdout:       true,
		Stderr:       true,
		OutputStream: &buf,
		ErrorStream:  &buf,
		Stream:       true,
	}
	status, err := SafeAttachWaitContainer(s.p, opts)
	c.Assert(err, check.IsNil)
	c.Assert(status, check.Equals, 0)
	c.Assert(buf.String(), check.Equals, "")
}

func (s *S) TestContainerValidAddr(c *check.C) {
	c.Assert((&Container{}).ValidAddr(), check.Equals, false)
	c.Assert((&Container{HostAddr: "1.1.1.1"}).ValidAddr(), check.Equals, false)
	c.Assert((&Container{HostAddr: "1.1.1.1", HostPort: "0"}).ValidAddr(), check.Equals, false)
	c.Assert((&Container{HostAddr: "1.1.1.1", HostPort: "123"}).ValidAddr(), check.Equals, true)
}
