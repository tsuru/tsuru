// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dockercommon

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/fsouza/go-dockerclient"
	"github.com/fsouza/go-dockerclient/testing"
	"github.com/tsuru/config"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/app/image"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"gopkg.in/check.v1"
)

type S struct{}

var _ = check.Suite(&S{})

func (s *S) SetUpTest(c *check.C) {
	config.Set("docker:registry", "my.registry")
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	err = dbtest.ClearAllCollections(conn.Apps().Database)
	c.Assert(err, check.IsNil)
}

func (s *S) TestPrepareImageForDeploy(c *check.C) {
	srv, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	defer srv.Stop()
	a := &app.App{Name: "myapp"}
	cli, err := docker.NewClient(srv.URL())
	c.Assert(err, check.IsNil)
	baseImgName := "baseImg"
	err = cli.PullImage(docker.PullImageOptions{Repository: baseImgName}, docker.AuthConfiguration{})
	c.Assert(err, check.IsNil)
	buf := bytes.Buffer{}
	args := PrepareImageArgs{
		Client:      cli,
		App:         a,
		ProcfileRaw: "web: myapp run",
		ImageID:     baseImgName,
		Out:         &buf,
		CustomData: map[string]interface{}{
			"healthcheck": map[string]interface{}{
				"path":   "/",
				"method": "GET",
			},
		},
	}
	newImg, err := PrepareImageForDeploy(args)
	c.Assert(err, check.IsNil)
	c.Assert(newImg, check.Equals, "my.registry/tsuru/app-myapp:v1")
	c.Assert(buf.String(), check.Equals, `---- Inspecting image "baseImg" ----
  ---> Process "web" found with commands: ["myapp run"]
---- Pushing image "my.registry/tsuru/app-myapp:v1" to tsuru ----
Pushing...
Pushed
`)
	imd, err := image.GetImageMetaData(newImg)
	c.Assert(err, check.IsNil)
	c.Assert(imd, check.DeepEquals, image.ImageMetadata{
		Name:      "my.registry/tsuru/app-myapp:v1",
		Processes: map[string][]string{"web": {"myapp run"}},
		CustomData: map[string]interface{}{
			"healthcheck": map[string]interface{}{
				"path":   "/",
				"method": "GET",
			},
		},
		LegacyProcesses: map[string]string{},
	})
}

func (s *S) TestPrepareImageForDeployNoProcfile(c *check.C) {
	srv, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	defer srv.Stop()
	a := &app.App{Name: "myapp"}
	cli, err := docker.NewClient(srv.URL())
	c.Assert(err, check.IsNil)
	baseImgName := "baseImg"
	err = cli.PullImage(docker.PullImageOptions{Repository: baseImgName}, docker.AuthConfiguration{})
	c.Assert(err, check.IsNil)
	srv.CustomHandler(fmt.Sprintf("/images/%s/json", baseImgName), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := docker.Image{
			Config: &docker.Config{
				Entrypoint:   []string{"/bin/sh"},
				Cmd:          []string{"python", "test file.py"},
				ExposedPorts: map[docker.Port]struct{}{"3000/tcp": {}},
			},
		}
		j, _ := json.Marshal(response)
		w.Write(j)
	}))
	buf := bytes.Buffer{}
	args := PrepareImageArgs{
		Client:      cli,
		App:         a,
		ProcfileRaw: "",
		ImageID:     baseImgName,
		Out:         &buf,
	}
	newImg, err := PrepareImageForDeploy(args)
	c.Assert(err, check.IsNil)
	c.Assert(newImg, check.Equals, "my.registry/tsuru/app-myapp:v1")
	c.Assert(buf.String(), check.Equals, `---- Inspecting image "baseImg" ----
  ---> Procfile not found, using entrypoint and cmd
  ---> Process "web" found with commands: ["/bin/sh" "python" "test file.py"]
---- Pushing image "my.registry/tsuru/app-myapp:v1" to tsuru ----
Pushing...
Pushed
`)
	imd, err := image.GetImageMetaData(newImg)
	c.Assert(err, check.IsNil)
	c.Assert(imd, check.DeepEquals, image.ImageMetadata{
		Name:            "my.registry/tsuru/app-myapp:v1",
		Processes:       map[string][]string{"web": {"/bin/sh", "python", "test file.py"}},
		CustomData:      map[string]interface{}{},
		LegacyProcesses: map[string]string{},
		ExposedPort:     "3000/tcp",
	})
}

func (s *S) TestPrepareImageForDeployMoreThanOnePortFromImage(c *check.C) {
	srv, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	defer srv.Stop()
	a := &app.App{Name: "myapp"}
	cli, err := docker.NewClient(srv.URL())
	c.Assert(err, check.IsNil)
	baseImgName := "baseImg"
	err = cli.PullImage(docker.PullImageOptions{Repository: baseImgName}, docker.AuthConfiguration{})
	c.Assert(err, check.IsNil)
	srv.CustomHandler(fmt.Sprintf("/images/%s/json", baseImgName), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := docker.Image{
			Config: &docker.Config{
				Entrypoint:   []string{"/bin/sh"},
				Cmd:          []string{"python", "test file.py"},
				ExposedPorts: map[docker.Port]struct{}{"3000/tcp": {}, "8080/tcp": {}},
			},
		}
		j, _ := json.Marshal(response)
		w.Write(j)
	}))
	buf := bytes.Buffer{}
	args := PrepareImageArgs{
		Client:      cli,
		App:         a,
		ProcfileRaw: "web: myapp run",
		ImageID:     baseImgName,
		Out:         &buf,
	}
	_, err = PrepareImageForDeploy(args)
	c.Assert(err, check.NotNil)
}

func (s *S) TestWaitDocker(c *check.C) {
	server, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	defer server.Stop()
	client, err := docker.NewClient(server.URL())
	c.Assert(err, check.IsNil)
	err = WaitDocker(client)
	c.Assert(err, check.IsNil)
	server.CustomHandler("/_ping", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	err = WaitDocker(client)
	c.Assert(err, check.NotNil)
	config.Set("docker:api-timeout", 1)
	defer config.Unset("docker:api-timeout")
	client, err = docker.NewClient("http://169.254.169.254:2375/")
	c.Assert(err, check.IsNil)
	err = WaitDocker(client)
	c.Assert(err, check.NotNil)
	expectedMsg := `Docker API at "http://169.254.169.254:2375/" didn't respond after 1 seconds`
	c.Assert(err.Error(), check.Equals, expectedMsg)
}

func (s *S) TestPushImage(c *check.C) {
	var requests []*http.Request
	server, err := testing.NewServer("127.0.0.1:0", nil, func(r *http.Request) {
		requests = append(requests, r)
	})
	c.Assert(err, check.IsNil)
	defer server.Stop()
	client, err := docker.NewClient(server.URL())
	c.Assert(err, check.IsNil)
	config.Set("docker:registry", "localhost:3030")
	defer config.Unset("docker:registry")
	err = client.PullImage(docker.PullImageOptions{Repository: "localhost:3030/base/img"}, docker.AuthConfiguration{})
	c.Assert(err, check.IsNil)
	err = PushImage(client, "localhost:3030/base/img", "", docker.AuthConfiguration{})
	c.Assert(err, check.IsNil)
	c.Assert(requests, check.HasLen, 2)
	c.Assert(requests[0].URL.Path, check.Equals, "/images/create")
	c.Assert(requests[1].URL.Path, check.Equals, "/images/localhost:3030/base/img/push")
	c.Assert(requests[1].URL.RawQuery, check.Equals, "")
	err = client.PullImage(docker.PullImageOptions{Repository: "localhost:3030/base/img:v2"}, docker.AuthConfiguration{})
	c.Assert(err, check.IsNil)
	err = PushImage(client, "localhost:3030/base/img", "v2", docker.AuthConfiguration{})
	c.Assert(err, check.IsNil)
	c.Assert(requests, check.HasLen, 4)
	c.Assert(requests[2].URL.Path, check.Equals, "/images/create")
	c.Assert(requests[3].URL.Path, check.Equals, "/images/localhost:3030/base/img/push")
	c.Assert(requests[3].URL.RawQuery, check.Equals, "tag=v2")
}

func (s *S) TestPushImageAuth(c *check.C) {
	var requests []*http.Request
	server, err := testing.NewServer("127.0.0.1:0", nil, func(r *http.Request) {
		requests = append(requests, r)
	})
	c.Assert(err, check.IsNil)
	defer server.Stop()
	client, err := docker.NewClient(server.URL())
	c.Assert(err, check.IsNil)
	config.Set("docker:registry", "localhost:3030")
	config.Set("docker:registry-auth:email", "me@company.com")
	config.Set("docker:registry-auth:username", "myuser")
	config.Set("docker:registry-auth:password", "mypassword")
	defer config.Unset("docker:registry")
	err = client.PullImage(docker.PullImageOptions{Repository: "localhost:3030/base/img"}, docker.AuthConfiguration{})
	c.Assert(err, check.IsNil)
	err = PushImage(client, "localhost:3030/base/img", "", docker.AuthConfiguration{})
	c.Assert(err, check.IsNil)
	c.Assert(requests, check.HasLen, 2)
	c.Assert(requests[0].URL.Path, check.Equals, "/images/create")
	c.Assert(requests[1].URL.Path, check.Equals, "/images/localhost:3030/base/img/push")
	c.Assert(requests[1].URL.RawQuery, check.Equals, "")
	auth := requests[1].Header.Get("X-Registry-Auth")
	var providedAuth docker.AuthConfiguration
	data, err := base64.StdEncoding.DecodeString(auth)
	c.Assert(err, check.IsNil)
	err = json.Unmarshal(data, &providedAuth)
	c.Assert(err, check.IsNil)
	c.Assert(providedAuth.ServerAddress, check.Equals, "localhost:3030")
	c.Assert(providedAuth.Email, check.Equals, "me@company.com")
	c.Assert(providedAuth.Username, check.Equals, "myuser")
	c.Assert(providedAuth.Password, check.Equals, "mypassword")
}

func (s *S) TestPushImageNoRegistry(c *check.C) {
	config.Unset("docker:registry")
	var request *http.Request
	server, err := testing.NewServer("127.0.0.1:0", nil, func(r *http.Request) {
		request = r
	})
	c.Assert(err, check.IsNil)
	defer server.Stop()
	client, err := docker.NewClient(server.URL())
	c.Assert(err, check.IsNil)
	err = PushImage(client, "localhost:3030/base", "", docker.AuthConfiguration{})
	c.Assert(err, check.IsNil)
	c.Assert(request, check.IsNil)
}

func (s *S) TestGetNodeByHost(c *check.C) {
	nodes := []cluster.Node{{
		Address: "http://h1:80",
	}, {
		Address: "http://h2:90",
	}, {
		Address: "http://h3",
	}, {
		Address: "h4",
	}, {
		Address: "h5:30123",
	}}
	myCluster, err := cluster.New(nil, &cluster.MapStorage{}, "", nodes...)
	c.Assert(err, check.IsNil)
	tests := [][]string{
		{"h1", nodes[0].Address},
		{"h2", nodes[1].Address},
		{"h3", nodes[2].Address},
		{"h4", nodes[3].Address},
		{"h5", nodes[4].Address},
	}
	for _, t := range tests {
		var n cluster.Node
		n, err = GetNodeByHost(myCluster, t[0])
		c.Assert(err, check.IsNil)
		c.Assert(n.Address, check.DeepEquals, t[1])
	}
	_, err = GetNodeByHost(myCluster, "h6")
	c.Assert(err, check.ErrorMatches, `node with host "h6" not found`)
}
