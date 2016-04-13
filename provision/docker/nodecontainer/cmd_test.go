// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nodecontainer

import (
	"bytes"
	"net/http"

	"github.com/tsuru/tsuru/cmd"
	"github.com/tsuru/tsuru/cmd/cmdtest"
	"gopkg.in/check.v1"
)

func (s *S) TestNodeContainerInfoRun(c *check.C) {
	var buf bytes.Buffer
	context := cmd.Context{Args: []string{"n1"}, Stdout: &buf}
	body := `{"": {"config": {"image": "img1"}}, "p1": {"config": {"image": "img2"}}}`
	trans := &cmdtest.ConditionalTransport{
		Transport: cmdtest.Transport{Message: body, Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			return req.URL.Path == "/1.0/docker/nodecontainers/n1" && req.Method == "GET"
		},
	}
	manager := cmd.Manager{}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, &manager)
	command := NodeContainerInfo{}
	err := command.Run(&context, client)
	c.Assert(err, check.IsNil)
	c.Assert(buf.String(), check.Equals, `+-------+--------------------------+
| Pool  | Config                   |
+-------+--------------------------+
| <all> | {                        |
|       |   "Name": "",            |
|       |   "PinnedImage": "",     |
|       |   "Config": {            |
|       |     "Cmd": null,         |
|       |     "Image": "img1",     |
|       |     "Entrypoint": null   |
|       |   },                     |
|       |   "HostConfig": {        |
|       |     "RestartPolicy": {}, |
|       |     "LogConfig": {}      |
|       |   }                      |
|       | }                        |
+-------+--------------------------+
| p1    | {                        |
|       |   "Name": "",            |
|       |   "PinnedImage": "",     |
|       |   "Config": {            |
|       |     "Cmd": null,         |
|       |     "Image": "img2",     |
|       |     "Entrypoint": null   |
|       |   },                     |
|       |   "HostConfig": {        |
|       |     "RestartPolicy": {}, |
|       |     "LogConfig": {}      |
|       |   }                      |
|       | }                        |
+-------+--------------------------+
`)
}

func (s *S) TestNodeContainerListRun(c *check.C) {
	var buf bytes.Buffer
	context := cmd.Context{Args: []string{}, Stdout: &buf}
	body := `[
{"name": "big-sibling", "configpools": {"": {"config": {"image": "img1"}}, "p1": {"config": {"image": "img2"}}}},
{"name": "c2", "configpools": {"p2": {"config": {"image": "imgX"}}}}
]`
	trans := &cmdtest.ConditionalTransport{
		Transport: cmdtest.Transport{Message: body, Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			return req.URL.Path == "/1.0/docker/nodecontainers" && req.Method == "GET"
		},
	}
	manager := cmd.Manager{}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, &manager)
	command := NodeContainerList{}
	err := command.Run(&context, client)
	c.Assert(err, check.IsNil)
	c.Assert(buf.String(), check.Equals, `+-------------+--------------+-------+
| Name        | Pool Configs | Image |
+-------------+--------------+-------+
| big-sibling | <all>        | img1  |
|             | p1           | img2  |
+-------------+--------------+-------+
| c2          | p2           | imgX  |
+-------------+--------------+-------+
`)
}

func (s *S) TestNodeContainerAddRun(c *check.C) {
	var buf bytes.Buffer
	context := cmd.Context{Args: []string{"n1"}, Stdout: &buf}
	trans := &cmdtest.ConditionalTransport{
		Transport: cmdtest.Transport{Message: "", Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			err := req.ParseForm()
			c.Assert(err, check.IsNil)
			c.Assert(req.FormValue("name"), check.Equals, "n1")
			c.Assert(req.FormValue("config.image"), check.Equals, "img2")
			c.Assert(req.FormValue("hostconfig.binds.0"), check.Equals, "/a:/b")
			c.Assert(req.FormValue("hostconfig.binds.1"), check.Equals, "/c:/d")
			c.Assert(req.FormValue("hostconfig.logconfig.config.a"), check.Equals, "b")
			c.Assert(req.Form["config.exposedports.8080/tcp"], check.DeepEquals, []string{""})
			c.Assert(req.Form["hostconfig.portbindings.8080/tcp.0.hostport"], check.DeepEquals, []string{"80"})
			c.Assert(req.Form["hostconfig.portbindings.8080/tcp.0.hostip"], check.DeepEquals, []string{""})
			return req.URL.Path == "/1.0/docker/nodecontainers" && req.Method == "POST"
		},
	}
	manager := cmd.Manager{}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, &manager)
	command := NodeContainerAdd{}
	command.Flags().Parse(true, []string{"--image", "img1", "-p", "80:8080", "-v", "/a:/b", "-v", "/c:/d", "-r", "Config.image=img2", "--log-opt", "a=b"})
	err := command.Run(&context, client)
	c.Assert(err, check.IsNil)
	c.Assert(buf.String(), check.Equals, "Node container successfully added.\n")
}

func (s *S) TestNodeContainerUpdateRun(c *check.C) {
	var buf bytes.Buffer
	context := cmd.Context{Args: []string{"n1"}, Stdout: &buf}
	trans := &cmdtest.ConditionalTransport{
		Transport: cmdtest.Transport{Message: "", Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			err := req.ParseForm()
			c.Assert(err, check.IsNil)
			c.Assert(req.FormValue("config.image"), check.Equals, "img2")
			c.Assert(req.FormValue("hostconfig.binds.0"), check.Equals, "/a:/b")
			c.Assert(req.FormValue("hostconfig.binds.1"), check.Equals, "/c:/d")
			c.Assert(req.FormValue("hostconfig.logconfig.config.a"), check.Equals, "b")
			c.Assert(req.Form["config.exposedports.8080/tcp"], check.DeepEquals, []string{""})
			c.Assert(req.Form["hostconfig.portbindings.8080/tcp.0.hostport"], check.DeepEquals, []string{"80"})
			c.Assert(req.Form["hostconfig.portbindings.8080/tcp.0.hostip"], check.DeepEquals, []string{""})
			return req.URL.Path == "/1.0/docker/nodecontainers/n1" && req.Method == "POST"
		},
	}
	manager := cmd.Manager{}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, &manager)
	command := NodeContainerUpdate{}
	command.Flags().Parse(true, []string{"--image", "img1", "-p", "80:8080", "-v", "/a:/b", "-v", "/c:/d", "-r", "Config.image=img2", "--log-opt", "a=b"})
	err := command.Run(&context, client)
	c.Assert(err, check.IsNil)
	c.Assert(buf.String(), check.Equals, "Node container successfully updated.\n")
}

func (s *S) TestNodeContainerDeleteRun(c *check.C) {
	var buf bytes.Buffer
	context := cmd.Context{Args: []string{"n1"}, Stdout: &buf}
	trans := &cmdtest.ConditionalTransport{
		Transport: cmdtest.Transport{Message: "", Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			return req.URL.Path == "/1.0/docker/nodecontainers/n1" && req.Method == "DELETE"
		},
	}
	manager := cmd.Manager{}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, &manager)
	command := NodeContainerDelete{}
	command.Flags().Parse(true, []string{"-y"})
	err := command.Run(&context, client)
	c.Assert(err, check.IsNil)
	c.Assert(buf.String(), check.Equals, "Node container successfully deleted.\n")
}

func (s *S) TestNodeContainerUpgradeRun(c *check.C) {
	var buf bytes.Buffer
	context := cmd.Context{Args: []string{"n1"}, Stdout: &buf}
	trans := &cmdtest.ConditionalTransport{
		Transport: cmdtest.Transport{Message: "", Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			return req.URL.Path == "/1.0/docker/nodecontainers/n1/upgrade" && req.Method == "POST"
		},
	}
	manager := cmd.Manager{}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, &manager)
	command := NodeContainerUpgrade{}
	command.Flags().Parse(true, []string{"-y"})
	err := command.Run(&context, client)
	c.Assert(err, check.IsNil)
	c.Assert(buf.String(), check.Equals, "")
}
