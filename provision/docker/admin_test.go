// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"bytes"
	"encoding/json"
	"github.com/tsuru/tsuru/cmd"
	"github.com/tsuru/tsuru/cmd/testing"
	"io/ioutil"
	"launchpad.net/gocheck"
	"net/http"
)

func (s *S) TestMoveContainerInfo(c *gocheck.C) {
	expected := &cmd.Info{
		Name:    "containers-move",
		Usage:   "containers-move <from host> <to host>",
		Desc:    "Move all containers from one host to another.\nThis command is especially useful for host maintenance.",
		MinArgs: 2,
	}
	c.Assert((&moveContainerCmd{}).Info(), gocheck.DeepEquals, expected)
}

func (s *S) TestMoveContainerRun(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
		Args:   []string{"from", "to"},
	}
	msg, _ := json.Marshal(progressLog{Message: "progress msg"})
	result := string(msg)
	trans := &testing.ConditionalTransport{
		Transport: testing.Transport{Message: result, Status: http.StatusOK},
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
			return req.URL.Path == "/containers/move" && req.Method == "POST"
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
		Usage:   "containers-rebalance [--dry]",
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
	trans := &testing.ConditionalTransport{
		Transport: testing.Transport{Message: result, Status: http.StatusOK},
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
			return req.URL.Path == "/containers/rebalance" && req.Method == "POST"
		},
	}
	manager := cmd.NewManager("admin", "0.1", "admin-ver", &stdout, &stderr, nil, nil)
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, manager)
	cmd := rebalanceContainersCmd{}
	cmd.Flags().Parse(true, []string{"--dry"})
	err := cmd.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	expected := "progress msg\n"
	c.Assert(stdout.String(), gocheck.Equals, expected)
	expectedDry = "false"
	cmd2 := rebalanceContainersCmd{}
	err = cmd2.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
}
