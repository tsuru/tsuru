// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"bytes"
	"encoding/json"
	"net/http"

	"github.com/ajg/form"
	"github.com/tsuru/tsuru/cmd"
	"github.com/tsuru/tsuru/cmd/cmdtest"
	tsuruIo "github.com/tsuru/tsuru/io"
	"gopkg.in/check.v1"
)

func (s *S) TestMoveContainersRun(c *check.C) {
	var stdout, stderr bytes.Buffer
	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
		Args:   []string{"from", "to"},
	}
	msg, _ := json.Marshal(tsuruIo.SimpleJsonMessage{Message: "progress msg"})
	result := string(msg)
	trans := &cmdtest.ConditionalTransport{
		Transport: cmdtest.Transport{Message: result, Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			path := req.URL.Path == "/1.0/docker/containers/move"
			method := req.Method == "POST"
			from := req.FormValue("from") == "from"
			to := req.FormValue("to") == "to"
			return path && method && from && to
		},
	}
	manager := cmd.NewManager("admin", "0.1", "admin-ver", &stdout, &stderr, nil, nil)
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, manager)
	cmd := moveContainersCmd{}
	err := cmd.Run(&context, client)
	c.Assert(err, check.IsNil)
	expected := "progress msg"
	c.Assert(stdout.String(), check.Equals, expected)
}

func (s *S) TestMoveContainerRun(c *check.C) {
	var stdout, stderr bytes.Buffer
	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
		Args:   []string{"contId", "toHost"},
	}
	msg, _ := json.Marshal(tsuruIo.SimpleJsonMessage{Message: "progress msg"})
	result := string(msg)
	trans := &cmdtest.ConditionalTransport{
		Transport: cmdtest.Transport{Message: result, Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			path := req.URL.Path == "/1.0/docker/container/contId/move"
			method := req.Method == "POST"
			to := req.FormValue("to") == "toHost"
			return path && method && to
		},
	}
	manager := cmd.NewManager("admin", "0.1", "admin-ver", &stdout, &stderr, nil, nil)
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, manager)
	cmd := moveContainerCmd{}
	err := cmd.Run(&context, client)
	c.Assert(err, check.IsNil)
	expected := "progress msg"
	c.Assert(stdout.String(), check.Equals, expected)
}

func (s *S) TestRebalanceContainersRun(c *check.C) {
	var stdout, stderr bytes.Buffer
	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
	}
	msg, _ := json.Marshal(tsuruIo.SimpleJsonMessage{Message: "progress msg"})
	result := string(msg)
	expectedRebalance := rebalanceOptions{
		Dry: true,
	}
	trans := &cmdtest.ConditionalTransport{
		Transport: cmdtest.Transport{Message: result, Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			req.ParseForm()
			var params rebalanceOptions
			dec := form.NewDecoder(nil)
			dec.IgnoreUnknownKeys(true)
			err := dec.DecodeValues(&params, req.Form)
			c.Assert(err, check.IsNil)
			c.Assert(params, check.DeepEquals, expectedRebalance)
			path := req.URL.Path == "/1.0/docker/containers/rebalance"
			method := req.Method == "POST"
			return path && method
		},
	}
	manager := cmd.NewManager("admin", "0.1", "admin-ver", &stdout, &stderr, nil, nil)
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, manager)
	cmd := rebalanceContainersCmd{}
	err := cmd.Flags().Parse(true, []string{"--dry", "-y"})
	c.Assert(err, check.IsNil)
	err = cmd.Run(&context, client)
	c.Assert(err, check.IsNil)
	expected := "progress msg"
	c.Assert(stdout.String(), check.Equals, expected)
	expectedRebalance = rebalanceOptions{
		Dry: false,
	}
	cmd2 := rebalanceContainersCmd{}
	cmd2.Flags().Parse(true, []string{"-y"})
	err = cmd2.Run(&context, client)
	c.Assert(err, check.IsNil)
}

func (s *S) TestRebalanceContainersRunWithFilters(c *check.C) {
	var stdout, stderr bytes.Buffer
	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
	}
	msg, _ := json.Marshal(tsuruIo.SimpleJsonMessage{Message: "progress msg"})
	result := string(msg)
	expectedRebalance := rebalanceOptions{
		Dry:            false,
		MetadataFilter: map[string]string{"pool": "x", "a": "b"},
		AppFilter:      []string{"x", "y"},
	}
	trans := &cmdtest.ConditionalTransport{
		Transport: cmdtest.Transport{Message: result, Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			req.ParseForm()
			var params rebalanceOptions
			dec := form.NewDecoder(nil)
			dec.IgnoreUnknownKeys(true)
			err := dec.DecodeValues(&params, req.Form)
			c.Assert(err, check.IsNil)
			c.Assert(params, check.DeepEquals, expectedRebalance)
			path := req.URL.Path == "/1.0/docker/containers/rebalance"
			method := req.Method == "POST"
			return path && method
		},
	}
	manager := cmd.NewManager("admin", "0.1", "admin-ver", &stdout, &stderr, nil, nil)
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, manager)
	cmd := rebalanceContainersCmd{}
	err := cmd.Flags().Parse(true, []string{"-y", "--metadata", "pool=x", "--metadata", "a=b", "--app", "x", "--app", "y"})
	c.Assert(err, check.IsNil)
	err = cmd.Run(&context, client)
	c.Assert(err, check.IsNil)
	expected := "progress msg"
	c.Assert(stdout.String(), check.Equals, expected)
}

func (s *S) TestRebalanceContainersRunAskingForConfirmation(c *check.C) {
	var stdout, stderr bytes.Buffer
	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
		Stdin:  bytes.NewBufferString("y"),
	}
	msg, _ := json.Marshal(tsuruIo.SimpleJsonMessage{Message: "progress msg"})
	result := string(msg)
	trans := &cmdtest.ConditionalTransport{
		Transport: cmdtest.Transport{Message: result, Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			req.ParseForm()
			var params rebalanceOptions
			dec := form.NewDecoder(nil)
			dec.IgnoreUnknownKeys(true)
			err := dec.DecodeValues(&params, req.Form)
			c.Assert(err, check.IsNil)
			c.Assert(params.Dry, check.Equals, false)
			path := req.URL.Path == "/1.0/docker/containers/rebalance"
			method := req.Method == "POST"
			return path && method
		},
	}
	manager := cmd.NewManager("admin", "0.1", "admin-ver", &stdout, &stderr, nil, nil)
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, manager)
	cmd := rebalanceContainersCmd{}
	err := cmd.Run(&context, client)
	c.Assert(err, check.IsNil)
	c.Assert(stdout.String(), check.Equals, "Are you sure you want to rebalance containers? (y/n) progress msg")
	cmd2 := rebalanceContainersCmd{}
	err = cmd2.Run(&context, client)
	c.Assert(err, check.IsNil)
}

func (s *S) TestRebalanceContainersRunGivingUp(c *check.C) {
	var stdout bytes.Buffer
	context := cmd.Context{
		Stdout: &stdout,
		Stdin:  bytes.NewBufferString("n\n"),
	}
	cmd := rebalanceContainersCmd{}
	err := cmd.Run(&context, nil)
	c.Assert(err, check.IsNil)
	c.Assert(stdout.String(), check.Equals, "Are you sure you want to rebalance containers? (y/n) Abort.\n")
}
