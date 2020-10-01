// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package container

import (
	"bytes"
	"context"
	"encoding/json"
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

	docker "github.com/fsouza/go-dockerclient"
	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/app/bind"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/docker/types"
	"github.com/tsuru/tsuru/provision/dockercommon"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/router/routertest"
	check "gopkg.in/check.v1"
)

func (s *S) TestContainerShortID(c *check.C) {
	container := Container{Container: types.Container{ID: "abc123"}}
	c.Check(container.ShortID(), check.Equals, container.ID)
	container.ID = "abcdef12345678"
	c.Check(container.ShortID(), check.Equals, "abcdef123456")
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
	container := Container{Container: types.Container{ID: "id123", HostAddr: "10.10.10.10", HostPort: "49153"}}
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
	routertest.FakeRouter.AddBackend(context.TODO(), app)
	defer routertest.FakeRouter.RemoveBackend(context.TODO(), app.GetName())
	img := "tsuru/brainfuck:latest"
	s.cli.PullImage(docker.PullImageOptions{Repository: img}, docker.AuthConfiguration{})
	cont := Container{Container: types.Container{
		Name:        "myName",
		AppName:     app.GetName(),
		Type:        app.GetPlatform(),
		Status:      "created",
		ProcessName: "myprocess1",
		ExposedPort: "8888/tcp",
	}}
	err := cont.Create(&CreateArgs{
		App:      app,
		ImageID:  img,
		Commands: []string{"docker", "run"},
		Client:   s.cli,
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
	expectedLabels, err := provision.ProcessLabels(context.TODO(), provision.ProcessLabelsOpts{
		App:         app,
		Process:     "myprocess1",
		Provisioner: "docker",
	})
	c.Assert(err, check.IsNil)
	c.Assert(container.Config.Labels, check.DeepEquals, expectedLabels.ToLabels())
	c.Assert(container.Config.Env, check.DeepEquals, []string{
		"A=myenva",
		"ABCD=other env",
		"PORT=8888",
		"TSURU_HOST=my.cool.tsuru.addr:8080",
		"TSURU_PROCESSNAME=myprocess1",
		"port=8888",
	})
	c.Assert(container.State.Running, check.Equals, false)
	expectedLogOptions := map[string]string{
		"syslog-address": "udp://localhost:1514",
	}
	expectedPortBindings := map[docker.Port][]docker.PortBinding{
		"8888/tcp": {{HostIP: "", HostPort: ""}},
	}
	c.Assert(container.HostConfig.RestartPolicy.Name, check.Equals, "always")
	c.Assert(container.HostConfig.LogConfig.Type, check.Equals, "syslog")
	c.Assert(container.HostConfig.LogConfig.Config, check.DeepEquals, expectedLogOptions)
	c.Assert(container.HostConfig.PortBindings, check.DeepEquals, expectedPortBindings)
	c.Assert(container.HostConfig.Memory, check.Equals, int64(15))
	c.Assert(container.HostConfig.MemorySwap, check.Equals, int64(30))
	c.Assert(container.HostConfig.CPUShares, check.Equals, int64(50))
	c.Assert(container.HostConfig.OomScoreAdj, check.Equals, 0)
	c.Assert(cont.Status, check.Equals, "created")
}

func (s *S) TestContainerCreateCustomLog(c *check.C) {
	client, err := docker.NewClient(s.server.URL())
	c.Assert(err, check.IsNil)
	app := provisiontest.NewFakeApp("myapp", "python", 1)
	app.Pool = "mypool"
	img := "tsuru/brainfuck:latest"
	s.cli.PullImage(docker.PullImageOptions{Repository: img}, docker.AuthConfiguration{})
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
	for i, testData := range testCases {
		c.Logf("test %d", i)
		cont := Container{Container: types.Container{
			Name:        fmt.Sprintf("myName-%d", i),
			AppName:     app.GetName(),
			Type:        app.GetPlatform(),
			Status:      "created",
			ProcessName: "myprocess1",
			ExposedPort: "8888/tcp",
		}}
		conf := DockerLogConfig{DockerLogConfig: types.DockerLogConfig{Driver: testData.name, LogOpts: testData.opts}}
		err = conf.Save(app.Pool)
		c.Assert(err, check.IsNil)
		err := cont.Create(&CreateArgs{
			App:      app,
			ImageID:  img,
			Commands: []string{"docker", "run"},
			Client:   s.cli,
		})
		c.Assert(err, check.IsNil)
		dockerContainer, err := client.InspectContainer(cont.ID)
		c.Assert(err, check.IsNil)
		c.Assert(dockerContainer.HostConfig.LogConfig.Type, check.Equals, testData.name)
		c.Assert(dockerContainer.HostConfig.LogConfig.Config, check.DeepEquals, testData.opts)
	}
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
	routertest.FakeRouter.AddBackend(context.TODO(), app)
	defer routertest.FakeRouter.RemoveBackend(context.TODO(), app.GetName())
	img := "tsuru/brainfuck:latest"
	s.cli.PullImage(docker.PullImageOptions{Repository: img}, docker.AuthConfiguration{})
	cont := Container{Container: types.Container{
		Name:        "myName",
		AppName:     app.GetName(),
		Type:        app.GetPlatform(),
		Status:      "created",
		ProcessName: "myprocess1",
		ExposedPort: "3000/tcp",
	}}
	err := cont.Create(&CreateArgs{
		App:      app,
		Commands: []string{"docker", "run"},
		Client:   s.cli,
		ImageID:  img,
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
	routertest.FakeRouter.AddBackend(context.TODO(), app)
	defer routertest.FakeRouter.RemoveBackend(context.TODO(), app.GetName())
	img := "tsuru/brainfuck:latest"
	s.cli.PullImage(docker.PullImageOptions{Repository: img}, docker.AuthConfiguration{})
	cont := Container{Container: types.Container{
		Name:    "myName",
		AppName: app.GetName(),
		Type:    app.GetPlatform(),
		Status:  "created",
	}}
	err := cont.Create(&CreateArgs{
		App:      app,
		ImageID:  img,
		Commands: []string{"docker", "run"},
		Client:   s.cli,
	})
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(&cont)
	dcli, _ := docker.NewClient(s.server.URL())
	container, err := dcli.InspectContainer(cont.ID)
	c.Assert(err, check.IsNil)
	c.Assert(container.Config.SecurityOpts, check.DeepEquals, []string{"label:type:svirt_apache", "ptrace peer=@unsecure"})
}

func (s *S) TestContainerCreateForDeploy(c *check.C) {
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
	app.Swap = 15
	app.CpuShare = 50
	routertest.FakeRouter.AddBackend(context.TODO(), app)
	defer routertest.FakeRouter.RemoveBackend(context.TODO(), app.GetName())
	img := "tsuru/brainfuck:latest"
	s.cli.PullImage(docker.PullImageOptions{Repository: img}, docker.AuthConfiguration{})
	cont := Container{Container: types.Container{
		Name:    "myName",
		AppName: app.GetName(),
		Type:    app.GetPlatform(),
		Status:  "created",
	}}
	err := cont.Create(&CreateArgs{
		Deploy:   true,
		App:      app,
		ImageID:  img,
		Commands: []string{"docker", "run"},
		Client:   s.cli,
	})
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(&cont)
	info, err := cont.NetworkInfo(s.cli)
	c.Assert(err, check.IsNil)
	c.Assert(info.HTTPHostPort, check.Equals, "")
	client, err := docker.NewClient(s.server.URL())
	c.Assert(err, check.IsNil)
	dockerContainer, err := client.InspectContainer(cont.ID)
	c.Assert(err, check.IsNil)
	c.Assert(dockerContainer.HostConfig.RestartPolicy.Name, check.Equals, "")
	c.Assert(dockerContainer.HostConfig.LogConfig.Type, check.Equals, "json-file")
	c.Assert(dockerContainer.HostConfig.Memory, check.Equals, int64(0))
	c.Assert(dockerContainer.HostConfig.MemorySwap, check.Equals, int64(0))
	c.Assert(dockerContainer.HostConfig.CPUShares, check.Equals, int64(50))
	c.Assert(dockerContainer.HostConfig.OomScoreAdj, check.Equals, 1000)
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
	routertest.FakeRouter.AddBackend(context.TODO(), app)
	defer routertest.FakeRouter.RemoveBackend(context.TODO(), app.GetName())
	img := "tsuru/brainfuck:latest"
	s.cli.PullImage(docker.PullImageOptions{Repository: img}, docker.AuthConfiguration{})
	cont := Container{Container: types.Container{
		Name:    "myName",
		AppName: app.GetName(),
		Type:    app.GetPlatform(),
		Status:  "created",
	}}
	err := cont.Create(&CreateArgs{
		Deploy:   true,
		App:      app,
		ImageID:  img,
		Commands: []string{"docker", "run"},
		Client:   s.cli,
	})
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(&cont)
	dcli, err := docker.NewClient(s.server.URL())
	c.Assert(err, check.IsNil)
	container, err := dcli.InspectContainer(cont.ID)
	c.Assert(err, check.IsNil)
	sort.Strings(container.Config.Env)
	c.Assert(container.Config.Env, check.DeepEquals, []string{
		"TSURU_HOST=my.cool.tsuru.addr:8080",
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
	s.cli.PullImage(docker.PullImageOptions{Repository: img}, docker.AuthConfiguration{})
	app := provisiontest.NewFakeApp("app-name", "python", 1)
	routertest.FakeRouter.AddBackend(context.TODO(), app)
	defer routertest.FakeRouter.RemoveBackend(context.TODO(), app.GetName())
	cont := Container{Container: types.Container{
		Name:    "myName",
		AppName: app.GetName(),
		Type:    app.GetPlatform(),
		Status:  "created",
	}}
	err := cont.Create(&CreateArgs{
		App:      app,
		ImageID:  img,
		Commands: []string{"docker", "run"},
		Client:   s.cli,
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
	s.cli.PullImage(docker.PullImageOptions{Repository: img}, docker.AuthConfiguration{})
	cont := Container{Container: types.Container{
		Name:    "myName",
		AppName: app.GetName(),
		Type:    app.GetPlatform(),
		Status:  "created",
	}}
	err := cont.Create(&CreateArgs{
		Deploy:   true,
		App:      app,
		ImageID:  img,
		Commands: []string{"docker", "run"},
		Client:   s.cli,
	})
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(&cont)
	dcli, err := docker.NewClient(s.server.URL())
	c.Assert(err, check.IsNil)
	container, err := dcli.InspectContainer(cont.ID)
	c.Assert(err, check.IsNil)
	c.Assert(container.Config.Entrypoint, check.DeepEquals, []string{})
}

func (s *S) TestContainerCreatePidLimit(c *check.C) {
	s.server.CustomHandler("/images/.*/json", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := docker.Image{
			Config: &docker.Config{
				ExposedPorts: map[docker.Port]struct{}{},
			},
		}
		j, _ := json.Marshal(response)
		w.Write(j)
	}))
	config.Set("docker:pids-limit", 10)
	defer config.Unset("docker:pids-limit")
	app := provisiontest.NewFakeApp("app-name", "brainfuck", 1)
	routertest.FakeRouter.AddBackend(context.TODO(), app)
	defer routertest.FakeRouter.RemoveBackend(context.TODO(), app.GetName())
	img := "tsuru/brainfuck:latest"
	s.cli.PullImage(docker.PullImageOptions{Repository: img}, docker.AuthConfiguration{})
	cont := Container{Container: types.Container{
		Name:    "myName",
		AppName: app.GetName(),
		Type:    app.GetPlatform(),
		Status:  "created",
	}}
	err := cont.Create(&CreateArgs{
		App:      app,
		ImageID:  img,
		Commands: []string{"docker", "run"},
		Client:   s.cli,
	})
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(&cont)
	dcli, _ := docker.NewClient(s.server.URL())
	container, err := dcli.InspectContainer(cont.ID)
	c.Assert(err, check.IsNil)
	c.Assert(container.HostConfig.PidsLimit, check.Equals, int64(10))
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
	cliRaw, err := docker.NewClient(server.URL)
	c.Assert(err, check.IsNil)
	cli := &dockercommon.PullAndCreateClient{Client: cliRaw}
	container := Container{Container: types.Container{ID: "c-01"}}
	info, err := container.NetworkInfo(cli)
	c.Assert(err, check.IsNil)
	c.Assert(info.IP, check.Equals, "10.10.10.10")
	c.Assert(info.HTTPHostPort, check.Equals, "")
}

func (s *S) TestContainerSetStatus(c *check.C) {
	update := time.Date(1989, 2, 2, 14, 59, 32, 0, time.UTC).In(time.UTC)
	container := Container{Container: types.Container{ID: "something-300", LastStatusUpdate: update}}
	err := container.SetStatus(s.cli, "what?!", true)
	c.Assert(err, check.IsNil)
	c.Assert(container.Status, check.Equals, "what?!")
	c.Assert(container.StatusBeforeError, check.Equals, "what?!")
	lastUpdate := container.LastStatusUpdate.In(time.UTC).Format(time.RFC822)
	c.Assert(lastUpdate, check.Not(check.DeepEquals), update.Format(time.RFC822))
	c.Assert(container.LastSuccessStatusUpdate.IsZero(), check.Equals, true)
}

func (s *S) TestContainerSetStatusStarted(c *check.C) {
	container := Container{Container: types.Container{ID: "telnet"}}
	err := container.SetStatus(s.cli, provision.StatusStarted, true)
	c.Assert(err, check.IsNil)
	c.Assert(container.Status, check.Equals, provision.StatusStarted.String())
	c.Assert(container.StatusBeforeError, check.Equals, provision.StatusStarted.String())
	c.Assert(container.LastSuccessStatusUpdate.IsZero(), check.Equals, false)
	container.LastSuccessStatusUpdate = time.Time{}
	err = container.SetStatus(s.cli, provision.StatusStarting, true)
	c.Assert(err, check.IsNil)
	c.Assert(container.LastSuccessStatusUpdate.IsZero(), check.Equals, false)
}

func (s *S) TestContainerSetStatusStopped(c *check.C) {
	container := Container{Container: types.Container{ID: "telnet"}}
	err := container.SetStatus(s.cli, provision.StatusStopped, true)
	c.Assert(err, check.IsNil)
	c.Assert(container.Status, check.Equals, provision.StatusStopped.String())
	c.Assert(container.StatusBeforeError, check.Equals, provision.StatusStopped.String())
	c.Assert(container.LastSuccessStatusUpdate.IsZero(), check.Equals, false)
}

func (s *S) TestContainerSetStatusError(c *check.C) {
	container := Container{Container: types.Container{ID: "telnet"}}
	err := container.SetStatus(s.cli, provision.StatusError, true)
	c.Assert(err, check.IsNil)
	c.Assert(container.Status, check.Equals, provision.StatusError.String())
	c.Assert(container.StatusBeforeError, check.Equals, "")
	c.Assert(container.LastSuccessStatusUpdate.IsZero(), check.Equals, true)
	err = container.SetStatus(s.cli, provision.StatusStarted, true)
	c.Assert(err, check.IsNil)
	c.Assert(container.Status, check.Equals, provision.StatusStarted.String())
	c.Assert(container.StatusBeforeError, check.Equals, provision.StatusStarted.String())
	err = container.SetStatus(s.cli, provision.StatusError, true)
	c.Assert(err, check.IsNil)
	c.Assert(container.Status, check.Equals, provision.StatusError.String())
	c.Assert(container.StatusBeforeError, check.Equals, provision.StatusStarted.String())
}

func (s *S) TestContainerSetStatusNoUpdate(c *check.C) {
	c1 := Container{Container: types.Container{
		ID:     "something-300",
		Status: provision.StatusBuilding.String(),
	}}
	err := c1.SetStatus(s.cli, provision.StatusStarted, false)
	c.Assert(err, check.IsNil)
	c.Assert(c1.Status, check.Equals, provision.StatusStarted.String())
	c.Assert(c1.StatusBeforeError, check.Equals, provision.StatusStarted.String())
}

func (s *S) TestContainerExpectedStatus(c *check.C) {
	c1 := Container{Container: types.Container{ID: "something-300"}}
	c.Assert(c1.ExpectedStatus(), check.Equals, provision.Status(""))
	err := c1.SetStatus(s.cli, provision.StatusStarted, true)
	c.Assert(err, check.IsNil)
	c.Assert(c1.ExpectedStatus(), check.Equals, provision.StatusStarted)
	err = c1.SetStatus(s.cli, provision.StatusError, true)
	c.Assert(err, check.IsNil)
	c.Assert(c1.ExpectedStatus(), check.Equals, provision.StatusStarted)
	err = c1.SetStatus(s.cli, provision.StatusStopped, true)
	c.Assert(err, check.IsNil)
	c.Assert(c1.ExpectedStatus(), check.Equals, provision.StatusStopped)
	err = c1.SetStatus(s.cli, provision.StatusError, true)
	c.Assert(err, check.IsNil)
	c.Assert(c1.ExpectedStatus(), check.Equals, provision.StatusStopped)
}

func (s *S) TestContainerSetImage(c *check.C) {
	container := Container{Container: types.Container{ID: "something-300"}}
	err := container.SetImage(s.cli, "newimage")
	c.Assert(err, check.IsNil)
	c.Assert(container.Image, check.Equals, "newimage")
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
	err = container.Remove(s.cli, s.limiter)
	c.Assert(err, check.IsNil)
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
	err = container.Remove(s.cli, s.limiter)
	c.Assert(err, check.IsNil)
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
	err = container.Start(&StartArgs{Client: s.cli, Limiter: s.limiter, App: &a})
	c.Assert(err, check.IsNil)
	err = container.Remove(s.cli, s.limiter)
	c.Assert(err, check.IsNil)
	client, _ := docker.NewClient(s.server.URL())
	_, err = client.InspectContainer(container.ID)
	c.Assert(err, check.NotNil)
	_, ok := err.(*docker.NoSuchContainer)
	c.Assert(ok, check.Equals, true)
}

func (s *S) TestContainerExecWithStdin(c *check.C) {
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
	err = container.Exec(s.cli, stdin, &stdout, &stderr, Pty{Width: 140, Height: 38, Term: "xterm"}, "cmd1", "arg1")
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
	c.Assert(cmd, check.DeepEquals, []string{"cmd1", "arg1"})
}

func (s *S) TestContainerExec(c *check.C) {
	container, err := s.newContainer(newContainerOpts{}, nil)
	c.Assert(err, check.IsNil)
	var stdout, stderr bytes.Buffer
	err = container.Exec(s.cli, nil, &stdout, &stderr, Pty{}, "ls", "-lh")
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
	err = container.Exec(s.cli, nil, &stdout, &stderr, Pty{}, "ls", "-lh")
	c.Assert(err, check.DeepEquals, &execErr{code: 9})
}

func (s *S) TestContainerCommit(c *check.C) {
	cont, err := s.newContainer(newContainerOpts{AppName: "myapp"}, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(cont)
	cont.BuildingImage = "tsuru/app-myapp:v1"
	var buf bytes.Buffer
	imageID, err := cont.Commit(s.cli, s.limiter, &buf, false)
	c.Assert(err, check.IsNil)
	repoNamespace, _ := config.GetString("docker:repository-namespace")
	repository := repoNamespace + "/app-" + cont.AppName + ":v1"
	c.Assert(imageID, check.Equals, repository)
}

var pushPathRegex = regexp.MustCompile(`/images/(.*)/push`)

func (s *S) TestContainerCommitWithRegistryBuild(c *check.C) {
	config.Set("docker:registry-max-try", 1)
	config.Set("docker:registry", "localhost:3030")
	defer config.Unset("docker:registry")
	var pushes []string
	s.server.SetHook(func(r *http.Request) {
		parts := pushPathRegex.FindStringSubmatch(r.URL.Path)
		if len(parts) == 2 {
			pushes = append(pushes, parts[1]+":"+r.URL.Query().Get("tag"))
		}
	})
	cont, err := s.newContainer(newContainerOpts{AppName: "myapp"}, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(cont)
	cont.BuildingImage = "localhost:3030/tsuru/app-myapp:v1"
	var buf bytes.Buffer
	imageID, err := cont.Commit(s.cli, s.limiter, &buf, false)
	c.Assert(err, check.IsNil)
	repoNamespace, _ := config.GetString("docker:repository-namespace")
	repository := "localhost:3030/" + repoNamespace + "/app-" + cont.AppName + ":v1"
	c.Assert(imageID, check.Equals, repository)
	c.Assert(pushes, check.DeepEquals, []string{
		"localhost:3030/" + repoNamespace + "/app-" + cont.AppName + ":v1",
	})
}

func (s *S) TestContainerCommitWithRegistryDeploy(c *check.C) {
	config.Set("docker:registry-max-try", 1)
	config.Set("docker:registry", "localhost:3030")
	defer config.Unset("docker:registry")
	var pushes []string
	s.server.SetHook(func(r *http.Request) {
		parts := pushPathRegex.FindStringSubmatch(r.URL.Path)
		if len(parts) == 2 {
			pushes = append(pushes, parts[1]+":"+r.URL.Query().Get("tag"))
		}
	})
	cont, err := s.newContainer(newContainerOpts{AppName: "myapp"}, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(cont)
	cont.BuildingImage = "localhost:3030/tsuru/app-myapp:v1"
	var buf bytes.Buffer
	imageID, err := cont.Commit(s.cli, s.limiter, &buf, true)
	c.Assert(err, check.IsNil)
	repoNamespace, _ := config.GetString("docker:repository-namespace")
	repository := "localhost:3030/" + repoNamespace + "/app-" + cont.AppName + ":v1"
	c.Assert(imageID, check.Equals, repository)
	c.Assert(pushes, check.DeepEquals, []string{
		"localhost:3030/" + repoNamespace + "/app-" + cont.AppName + ":v1",
		"localhost:3030/" + repoNamespace + "/app-" + cont.AppName + ":latest",
	})
}

func (s *S) TestContainerCommitErrorInPush(c *check.C) {
	failures := []string{"first failure", "second failure", "third failure"}
	s.server.CustomHandler("/images/.*/push", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(failures) > 0 {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(failures[0]))
			failures = failures[1:]
			return
		}
		s.server.DefaultHandler().ServeHTTP(w, r)
	}))
	config.Set("docker:registry", "localhost:3030")
	defer config.Unset("docker:registry")
	cont, err := s.newContainer(newContainerOpts{}, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(cont)
	cont.BuildingImage = cont.Image
	var buf bytes.Buffer
	_, err = cont.Commit(s.cli, s.limiter, &buf, false)
	c.Assert(err, check.ErrorMatches, ".*third failure$")
}

func (s *S) TestContainerCommitRetryPush(c *check.C) {
	failures := []string{"first failure", "second failure"}
	var pushes []string
	s.server.CustomHandler("/images/.*/push", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := pushPathRegex.FindStringSubmatch(r.URL.Path)
		if len(parts) == 2 {
			pushes = append(pushes, parts[1]+":"+r.URL.Query().Get("tag"))
		}
		if len(failures) > 0 {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(failures[0]))
			failures = failures[1:]
			return
		}
		s.server.DefaultHandler().ServeHTTP(w, r)
	}))
	config.Set("docker:registry-max-try", -1)
	config.Set("docker:registry", "localhost:3030")
	defer config.Unset("docker:registry")
	cont, err := s.newContainer(newContainerOpts{}, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(cont)
	cont.BuildingImage = cont.Image
	var buf bytes.Buffer
	_, err = cont.Commit(s.cli, s.limiter, &buf, false)
	c.Assert(err, check.IsNil)
	expectedPush := "tsuru/python:latest"
	c.Assert(pushes, check.DeepEquals, []string{expectedPush, expectedPush, expectedPush})
}

func (s *S) TestContainerStop(c *check.C) {
	cont, err := s.newContainer(newContainerOpts{}, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(cont)
	client, err := docker.NewClient(s.server.URL())
	c.Assert(err, check.IsNil)
	err = client.StartContainer(cont.ID, nil)
	c.Assert(err, check.IsNil)
	err = cont.Stop(s.cli, s.limiter)
	c.Assert(err, check.IsNil)
	dockerContainer, err := s.cli.InspectContainer(cont.ID)
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
		Client:  s.cli,
		Limiter: s.limiter,
		App:     app,
	})
	c.Assert(err, check.IsNil)
	err = cont.Sleep(s.cli, s.limiter)
	c.Assert(err, check.IsNil)
	dockerContainer, err := s.cli.InspectContainer(cont.ID)
	c.Assert(err, check.IsNil)
	c.Assert(dockerContainer.State.Running, check.Equals, false)
	c.Assert(cont.Status, check.Equals, provision.StatusAsleep.String())
}

func (s *S) TestContainerSleepNotStarted(c *check.C) {
	cont, err := s.newContainer(newContainerOpts{}, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(cont)
	err = cont.Sleep(s.cli, s.limiter)
	c.Assert(err, check.NotNil)
}

func (s *S) TestContainerStopReturnsNilWhenContainerAlreadyMarkedAsStopped(c *check.C) {
	cont, err := s.newContainer(newContainerOpts{}, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(cont)
	cont.SetStatus(s.cli, provision.StatusStopped, true)
	err = cont.Stop(s.cli, s.limiter)
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
	err = cont.Start(&StartArgs{
		Client:  s.cli,
		Limiter: s.limiter,
		App:     app,
	})
	c.Assert(err, check.IsNil)
	dockerContainer, err = client.InspectContainer(cont.ID)
	c.Assert(err, check.IsNil)
	c.Assert(dockerContainer.State.Running, check.Equals, true)
	c.Assert(cont.Status, check.Equals, "starting")
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
	err = cont.Start(&StartArgs{Client: s.cli, Limiter: s.limiter, App: app, Deploy: true})
	c.Assert(err, check.IsNil)
	c.Assert(cont.Status, check.Equals, "building")
	dockerContainer, err := client.InspectContainer(cont.ID)
	c.Assert(err, check.IsNil)
	c.Assert(dockerContainer.State.Running, check.Equals, true)
}

func (s *S) TestContainerStartStartedUnits(c *check.C) {
	app := provisiontest.NewFakeApp("myapp", "python", 1)
	cont, err := s.newContainer(newContainerOpts{}, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(cont)
	args := StartArgs{Client: s.cli, Limiter: s.limiter, App: app}
	err = cont.Start(&args)
	c.Assert(err, check.IsNil)
	err = cont.Start(&args)
	c.Assert(err, check.NotNil)
}

func (s *S) TestContainerLogsAlreadyStopped(c *check.C) {
	app := provisiontest.NewFakeApp("myapp", "python", 1)
	cont, err := s.newContainer(newContainerOpts{}, nil)
	c.Assert(err, check.IsNil)
	args := StartArgs{Client: s.cli, Limiter: s.limiter, App: app}
	err = cont.Start(&args)
	c.Assert(err, check.IsNil)
	err = cont.Stop(s.cli, s.limiter)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(cont)
	var buff bytes.Buffer
	status, err := cont.Logs(s.cli, &buff)
	c.Assert(err, check.IsNil)
	c.Assert(status, check.Equals, 0)
	c.Assert(buff.String(), check.Not(check.Equals), "")
}

func (s *S) TestContainerAsUnit(c *check.C) {
	app := provisiontest.NewFakeApp("myapp", "python", 1)
	expected := provision.Unit{
		ID:          "c-id",
		Name:        "c-name",
		AppName:     "myapp",
		Type:        "python",
		IP:          "192.168.50.4",
		Status:      provision.StatusBuilding,
		ProcessName: "web",
		Address:     &url.URL{Scheme: "http", Host: "192.168.50.4:8080"},
		Routable:    true,
	}
	container := Container{Container: types.Container{
		ID:          "c-id",
		Name:        "c-name",
		HostAddr:    "192.168.50.4",
		HostPort:    "8080",
		ProcessName: "web",
	}}
	got := container.AsUnit(app)
	c.Assert(got, check.DeepEquals, expected)
	container.Type = "ruby"
	container.Status = provision.StatusStarted.String()
	got = container.AsUnit(app)
	expected.Status = provision.StatusStarted
	expected.Type = "ruby"
	c.Assert(got, check.DeepEquals, expected)
}

func (s *S) TestSafeAttachWaitContainerStopped(c *check.C) {
	app := provisiontest.NewFakeApp("myapp", "python", 1)
	cont, err := s.newContainer(newContainerOpts{}, nil)
	c.Assert(err, check.IsNil)
	args := StartArgs{Client: s.cli, Limiter: s.limiter, App: app}
	err = cont.Start(&args)
	c.Assert(err, check.IsNil)
	err = cont.Stop(s.cli, s.limiter)
	c.Assert(err, check.IsNil)
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
	status, err := SafeAttachWaitContainer(s.cli, opts)
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
	status, err := SafeAttachWaitContainer(s.cli, opts)
	c.Assert(err, check.IsNil)
	c.Assert(status, check.Equals, 0)
	c.Assert(buf.String(), check.Equals, "")
}

func (s *S) TestContainerValidAddr(c *check.C) {
	c.Assert((&Container{}).ValidAddr(), check.Equals, false)
	c.Assert((&Container{Container: types.Container{HostAddr: "1.1.1.1"}}).ValidAddr(), check.Equals, false)
	c.Assert((&Container{Container: types.Container{HostAddr: "1.1.1.1", HostPort: "0"}}).ValidAddr(), check.Equals, false)
	c.Assert((&Container{Container: types.Container{HostAddr: "1.1.1.1", HostPort: "123"}}).ValidAddr(), check.Equals, true)
}

func (s *S) TestRunPipelineWithRetry(c *check.C) {
	var params []interface{}
	var calls int
	var testAction = &action.Action{
		Forward: func(ctx action.FWContext) (action.Result, error) {
			params = ctx.Params
			calls++
			return nil, nil
		},
	}
	pipe := action.NewPipeline(testAction)
	expectedArgs := "test"
	err := RunPipelineWithRetry(context.TODO(), pipe, expectedArgs)
	c.Assert(err, check.IsNil)
	c.Assert(calls, check.Equals, 1)
	c.Assert(params, check.DeepEquals, []interface{}{
		expectedArgs,
	})
}

func (s *S) TestRunPipelineWithRetryUnknownError(c *check.C) {
	var calls int
	var testAction = &action.Action{
		Forward: func(ctx action.FWContext) (action.Result, error) {
			calls++
			return nil, errors.New("my err")
		},
	}
	pipe := action.NewPipeline(testAction)
	err := RunPipelineWithRetry(context.TODO(), pipe, nil)
	c.Assert(err, check.ErrorMatches, "my err")
	c.Assert(calls, check.Equals, 1)
}

func (s *S) TestRunPipelineWithRetryStartError(c *check.C) {
	var calls int
	var testAction = &action.Action{
		Forward: func(ctx action.FWContext) (action.Result, error) {
			calls++
			return nil, &StartError{Base: errors.New("my err")}
		},
	}
	pipe := action.NewPipeline(testAction)
	err := RunPipelineWithRetry(context.TODO(), pipe, nil)
	c.Assert(err, check.ErrorMatches, `(?s)multiple errors reported \(5\):.*my err.*`)
	c.Assert(calls, check.Equals, maxStartRetries+1)
}

func (s *S) TestRunPipelineWithRetryStartErrorWithSuccess(c *check.C) {
	var calls int
	var testAction = &action.Action{
		Forward: func(ctx action.FWContext) (action.Result, error) {
			calls++
			if calls < 3 {
				return nil, &StartError{Base: errors.New("my err")}
			}
			return nil, nil
		},
	}
	pipe := action.NewPipeline(testAction)
	err := RunPipelineWithRetry(context.TODO(), pipe, nil)
	c.Assert(err, check.IsNil)
	c.Assert(calls, check.Equals, 3)
}
