// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bs

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"

	"github.com/tsuru/tsuru/cmd"
	"github.com/tsuru/tsuru/cmd/cmdtest"
	"github.com/tsuru/tsuru/io"
	"github.com/tsuru/tsuru/provision"
	"gopkg.in/check.v1"
)

func (s *S) TestBsEnvSetRun(c *check.C) {
	var stdout, stderr bytes.Buffer
	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
		Args:   []string{"A=1", "B=2"},
	}
	msg := io.SimpleJsonMessage{Message: "env-set success"}
	result, _ := json.Marshal(msg)
	trans := &cmdtest.ConditionalTransport{
		Transport: cmdtest.Transport{Message: string(result), Status: http.StatusNoContent},
		CondFunc: func(req *http.Request) bool {
			defer req.Body.Close()
			body, err := ioutil.ReadAll(req.Body)
			c.Assert(err, check.IsNil)
			expected := provision.ScopedConfig{
				Envs: []provision.Entry{{Name: "A", Value: "1"}, {Name: "B", Value: "2"}},
			}
			var conf provision.ScopedConfig
			err = json.Unmarshal(body, &conf)
			c.Assert(conf, check.DeepEquals, expected)
			return req.URL.Path == "/docker/bs/env" && req.Method == "POST"
		},
	}
	manager := cmd.NewManager("admin", "0.1", "admin-ver", &stdout, &stderr, nil, nil)
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, manager)
	cmd := EnvSetCmd{}
	err := cmd.Run(&context, client)
	c.Assert(err, check.IsNil)
	c.Assert(stdout.String(), check.Equals, "env-set success")
}

func (s *S) TestBsEnvSetRunAllowEmpty(c *check.C) {
	var stdout, stderr bytes.Buffer
	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
		Args:   []string{"A=1", "B="},
	}
	msg := io.SimpleJsonMessage{Message: "env-set success"}
	result, _ := json.Marshal(msg)
	trans := &cmdtest.ConditionalTransport{
		Transport: cmdtest.Transport{Message: string(result), Status: http.StatusNoContent},
		CondFunc: func(req *http.Request) bool {
			defer req.Body.Close()
			body, err := ioutil.ReadAll(req.Body)
			c.Assert(err, check.IsNil)
			expected := provision.ScopedConfig{
				Envs: []provision.Entry{{Name: "A", Value: "1"}, {Name: "B", Value: ""}},
			}
			var conf provision.ScopedConfig
			err = json.Unmarshal(body, &conf)
			c.Assert(conf, check.DeepEquals, expected)
			return req.URL.Path == "/docker/bs/env" && req.Method == "POST"
		},
	}
	manager := cmd.NewManager("admin", "0.1", "admin-ver", &stdout, &stderr, nil, nil)
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, manager)
	cmd := EnvSetCmd{}
	err := cmd.Run(&context, client)
	c.Assert(err, check.IsNil)
	c.Assert(stdout.String(), check.Equals, "env-set success")
}

func (s *S) TestBsEnvSetRunInvalidInput(c *check.C) {
	var stdout, stderr bytes.Buffer
	manager := cmd.NewManager("admin", "0.1", "admin-ver", &stdout, &stderr, nil, nil)
	client := cmd.NewClient(&http.Client{}, nil, manager)
	command := EnvSetCmd{}
	err := command.Run(&cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
		Args:   []string{"xxx"},
	}, client)
	c.Assert(err, check.ErrorMatches, "invalid variable values")
	err = command.Run(&cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
		Args:   []string{"a=1", "="},
	}, client)
	c.Assert(err, check.ErrorMatches, "invalid variable values")
}

func (s *S) TestBsEnvSetRunForPool(c *check.C) {
	var stdout, stderr bytes.Buffer
	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
		Args:   []string{"A=1", "B=2"},
	}
	msg := io.SimpleJsonMessage{Message: "env-set success"}
	result, _ := json.Marshal(msg)
	trans := &cmdtest.ConditionalTransport{
		Transport: cmdtest.Transport{Message: string(result), Status: http.StatusNoContent},
		CondFunc: func(req *http.Request) bool {
			defer req.Body.Close()
			body, err := ioutil.ReadAll(req.Body)
			c.Assert(err, check.IsNil)
			expected := provision.ScopedConfig{
				Pools: []provision.PoolEntry{{
					Name: "pool1",
					Envs: []provision.Entry{{Name: "A", Value: "1"}, {Name: "B", Value: "2"}},
				}},
			}
			var conf provision.ScopedConfig
			err = json.Unmarshal(body, &conf)
			c.Assert(conf, check.DeepEquals, expected)
			return req.URL.Path == "/docker/bs/env" && req.Method == "POST"
		},
	}
	manager := cmd.NewManager("admin", "0.1", "admin-ver", &stdout, &stderr, nil, nil)
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, manager)
	cmd := EnvSetCmd{}
	err := cmd.Flags().Parse(true, []string{"--pool", "pool1"})
	c.Assert(err, check.IsNil)
	err = cmd.Run(&context, client)
	c.Assert(err, check.IsNil)
	c.Assert(stdout.String(), check.Equals, "env-set success")
}

func (s *S) TestBsInfoRun(c *check.C) {
	var stdout, stderr bytes.Buffer
	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
	}
	conf := provision.ScopedConfig{
		Extra: map[string]interface{}{"image": "tsuru/bs"},
		Envs: []provision.Entry{
			{Name: "A", Value: "1"},
			{Name: "B", Value: "2"},
		},
		Pools: []provision.PoolEntry{
			{Name: "pool1", Envs: []provision.Entry{
				{Name: "A", Value: "9"},
				{Name: "Z", Value: "8"},
			}},
			{Name: "pool2", Envs: []provision.Entry{
				{Name: "Y", Value: "7"},
			}},
		},
	}
	result, err := json.Marshal(conf)
	c.Assert(err, check.IsNil)
	trans := &cmdtest.ConditionalTransport{
		Transport: cmdtest.Transport{Message: string(result), Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			return req.URL.Path == "/docker/bs" && req.Method == "GET"
		},
	}
	manager := cmd.NewManager("admin", "0.1", "admin-ver", &stdout, &stderr, nil, nil)
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, manager)
	cmd := InfoCmd{}
	err = cmd.Run(&context, client)
	c.Assert(err, check.IsNil)
	c.Assert(stdout.String(), check.Equals, `Image: tsuru/bs

Environment Variables [Default]:
+------+-------+
| Name | Value |
+------+-------+
| A    | 1     |
| B    | 2     |
+------+-------+

Environment Variables [pool1]:
+------+-------+
| Name | Value |
+------+-------+
| A    | 9     |
| Z    | 8     |
+------+-------+

Environment Variables [pool2]:
+------+-------+
| Name | Value |
+------+-------+
| Y    | 7     |
+------+-------+
`)
}

func (s *S) TestBsUpgradeRun(c *check.C) {
	var called bool
	var stdout, stderr bytes.Buffer
	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
		Args:   []string{"A=1", "B=2"},
	}
	msg := io.SimpleJsonMessage{Message: "it worked!"}
	result, err := json.Marshal(msg)
	trans := &cmdtest.ConditionalTransport{
		Transport: cmdtest.Transport{Message: string(result), Status: http.StatusNoContent},
		CondFunc: func(req *http.Request) bool {
			called = true
			return req.URL.Path == "/docker/bs/upgrade" && req.Method == "POST"
		},
	}
	manager := cmd.NewManager("admin", "0.1", "admin-ver", &stdout, &stderr, nil, nil)
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, manager)
	cmd := UpgradeCmd{}
	err = cmd.Run(&context, client)
	c.Assert(err, check.IsNil)
	c.Assert(stdout.String(), check.Equals, "it worked!")
	c.Assert(called, check.Equals, true)
}
