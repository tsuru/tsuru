// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"

	"github.com/tsuru/tsuru/cmd"
	"github.com/tsuru/tsuru/cmd/cmdtest"
	"launchpad.net/gocheck"
)

func (s *S) TestMoveContainersInfo(c *gocheck.C) {
	expected := &cmd.Info{
		Name:    "containers-move",
		Usage:   "containers-move <from host> <to host>",
		Desc:    "Move all containers from one host to another.\nThis command is especially useful for host maintenance.",
		MinArgs: 2,
	}
	c.Assert((&moveContainersCmd{}).Info(), gocheck.DeepEquals, expected)
}

func (s *S) TestMoveContainersRun(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
		Args:   []string{"from", "to"},
	}
	msg, _ := json.Marshal(progressLog{Message: "progress msg"})
	result := string(msg)
	trans := &cmdtest.ConditionalTransport{
		Transport: cmdtest.Transport{Message: result, Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			defer req.Body.Close()
			body, err := ioutil.ReadAll(req.Body)
			c.Assert(err, gocheck.IsNil)
			expected := map[string]string{
				"from": "from",
				"to":   "to",
			}
			result := map[string]string{}
			err = json.Unmarshal(body, &result)
			c.Assert(expected, gocheck.DeepEquals, result)
			return req.URL.Path == "/docker/containers/move" && req.Method == "POST"
		},
	}
	manager := cmd.NewManager("admin", "0.1", "admin-ver", &stdout, &stderr, nil, nil)
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, manager)
	cmd := moveContainersCmd{}
	err := cmd.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	expected := "progress msg\n"
	c.Assert(stdout.String(), gocheck.Equals, expected)
}

func (s *S) TestMoveContainerInfo(c *gocheck.C) {
	expected := &cmd.Info{
		Name:    "container-move",
		Usage:   "container-move <container id> <to host>",
		Desc:    "Move specified container to another host.",
		MinArgs: 2,
	}
	c.Assert((&moveContainerCmd{}).Info(), gocheck.DeepEquals, expected)
}

func (s *S) TestMoveContainerRun(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
		Args:   []string{"contId", "toHost"},
	}
	msg, _ := json.Marshal(progressLog{Message: "progress msg"})
	result := string(msg)
	trans := &cmdtest.ConditionalTransport{
		Transport: cmdtest.Transport{Message: result, Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			defer req.Body.Close()
			body, err := ioutil.ReadAll(req.Body)
			c.Assert(err, gocheck.IsNil)
			expected := map[string]string{
				"to": "toHost",
			}
			result := map[string]string{}
			err = json.Unmarshal(body, &result)
			c.Assert(expected, gocheck.DeepEquals, result)
			return req.URL.Path == "/docker/container/contId/move" && req.Method == "POST"
		},
	}
	manager := cmd.NewManager("admin", "0.1", "admin-ver", &stdout, &stderr, nil, nil)
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, manager)
	cmd := moveContainerCmd{}
	err := cmd.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	expected := "progress msg\n"
	c.Assert(stdout.String(), gocheck.Equals, expected)
}

func (s *S) TestRebalanceContainersInfo(c *gocheck.C) {
	expected := &cmd.Info{
		Name:    "containers-rebalance",
		Usage:   "containers-rebalance [--dry] [-y/--assume-yes]",
		Desc:    "Move containers creating a more even distribution between docker nodes.",
		MinArgs: 0,
	}
	c.Assert((&rebalanceContainersCmd{}).Info(), gocheck.DeepEquals, expected)
}

func (s *S) TestRebalanceContainersRun(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
	}
	msg, _ := json.Marshal(progressLog{Message: "progress msg"})
	result := string(msg)
	expectedDry := "true"
	trans := &cmdtest.ConditionalTransport{
		Transport: cmdtest.Transport{Message: result, Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			defer req.Body.Close()
			body, err := ioutil.ReadAll(req.Body)
			c.Assert(err, gocheck.IsNil)
			expected := map[string]string{
				"dry": expectedDry,
			}
			result := map[string]string{}
			err = json.Unmarshal(body, &result)
			c.Assert(expected, gocheck.DeepEquals, result)
			return req.URL.Path == "/docker/containers/rebalance" && req.Method == "POST"
		},
	}
	manager := cmd.NewManager("admin", "0.1", "admin-ver", &stdout, &stderr, nil, nil)
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, manager)
	cmd := rebalanceContainersCmd{}
	err := cmd.Flags().Parse(true, []string{"--dry", "-y"})
	c.Assert(err, gocheck.IsNil)
	err = cmd.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	expected := "progress msg\n"
	c.Assert(stdout.String(), gocheck.Equals, expected)
	expectedDry = "false"
	cmd2 := rebalanceContainersCmd{}
	cmd2.Flags().Parse(true, []string{"-y"})
	err = cmd2.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestRebalanceContainersRunAskingForConfirmation(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
		Stdin:  bytes.NewBufferString("y"),
	}
	msg, _ := json.Marshal(progressLog{Message: "progress msg"})
	result := string(msg)
	trans := &cmdtest.ConditionalTransport{
		Transport: cmdtest.Transport{Message: result, Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			defer req.Body.Close()
			body, err := ioutil.ReadAll(req.Body)
			c.Assert(err, gocheck.IsNil)
			expected := map[string]string{
				"dry": "false",
			}
			result := map[string]string{}
			err = json.Unmarshal(body, &result)
			c.Assert(expected, gocheck.DeepEquals, result)
			return req.URL.Path == "/docker/containers/rebalance" && req.Method == "POST"
		},
	}
	manager := cmd.NewManager("admin", "0.1", "admin-ver", &stdout, &stderr, nil, nil)
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, manager)
	cmd := rebalanceContainersCmd{}
	err := cmd.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, "Are you sure you want to rebalance containers? (y/n) progress msg\n")
	cmd2 := rebalanceContainersCmd{}
	err = cmd2.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestRebalanceContainersRunGivingUp(c *gocheck.C) {
	var stdout bytes.Buffer
	context := cmd.Context{
		Stdout: &stdout,
		Stdin:  bytes.NewBufferString("n\n"),
	}
	cmd := rebalanceContainersCmd{}
	err := cmd.Run(&context, nil)
	c.Assert(err, gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, "Are you sure you want to rebalance containers? (y/n) Abort.\n")
}

func (s *S) TestFixContainersCmdRun(c *gocheck.C) {
	var buf bytes.Buffer
	context := cmd.Context{Stdout: &buf, Stderr: &buf}
	trans := &cmdtest.ConditionalTransport{
		Transport: cmdtest.Transport{Message: "", Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			return req.URL.Path == "/docker/fix-containers" && req.Method == "POST"
		},
	}
	manager := cmd.NewManager("admin", "0.1", "admin-ver", &buf, &buf, nil, nil)
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, manager)
	cmd := fixContainersCmd{}
	err := cmd.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(buf.String(), gocheck.Equals, "")
}

func (s *S) TestFixContainersCmdInfo(c *gocheck.C) {
	expected := cmd.Info{
		Name:  "fix-containers",
		Usage: "fix-containers",
		Desc:  "Fix containers that are broken in the cluster.",
	}
	command := fixContainersCmd{}
	info := command.Info()
	c.Assert(*info, gocheck.DeepEquals, expected)
}
