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
	"gopkg.in/check.v1"
)

func (s *S) TestBsEnvSetRun(c *check.C) {
	var stdout, stderr bytes.Buffer
	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
		Args:   []string{"A=1", "B=2"},
	}
	trans := &cmdtest.ConditionalTransport{
		Transport: cmdtest.Transport{Message: "", Status: http.StatusNoContent},
		CondFunc: func(req *http.Request) bool {
			defer req.Body.Close()
			body, err := ioutil.ReadAll(req.Body)
			c.Assert(err, check.IsNil)
			expected := bsConfig{
				Envs: []bsEnv{{Name: "A", Value: "1"}, {Name: "B", Value: "2"}},
			}
			var conf bsConfig
			err = json.Unmarshal(body, &conf)
			c.Assert(conf, check.DeepEquals, expected)
			return req.URL.Path == "/docker/bs/env" && req.Method == "POST"
		},
	}
	manager := cmd.NewManager("admin", "0.1", "admin-ver", &stdout, &stderr, nil, nil)
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, manager)
	cmd := bsEnvSetCmd{}
	err := cmd.Run(&context, client)
	c.Assert(err, check.IsNil)
	c.Assert(stdout.String(), check.Equals, "Variables successfully set.\n")
}

func (s *S) TestBsEnvSetRunForPool(c *check.C) {
	var stdout, stderr bytes.Buffer
	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
		Args:   []string{"A=1", "B=2"},
	}
	trans := &cmdtest.ConditionalTransport{
		Transport: cmdtest.Transport{Message: "", Status: http.StatusNoContent},
		CondFunc: func(req *http.Request) bool {
			defer req.Body.Close()
			body, err := ioutil.ReadAll(req.Body)
			c.Assert(err, check.IsNil)
			expected := bsConfig{
				Pools: []bsPoolEnvs{{
					Name: "pool1",
					Envs: []bsEnv{{Name: "A", Value: "1"}, {Name: "B", Value: "2"}},
				}},
			}
			var conf bsConfig
			err = json.Unmarshal(body, &conf)
			c.Assert(conf, check.DeepEquals, expected)
			return req.URL.Path == "/docker/bs/env" && req.Method == "POST"
		},
	}
	manager := cmd.NewManager("admin", "0.1", "admin-ver", &stdout, &stderr, nil, nil)
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, manager)
	cmd := bsEnvSetCmd{}
	err := cmd.Flags().Parse(true, []string{"--pool", "pool1"})
	c.Assert(err, check.IsNil)
	err = cmd.Run(&context, client)
	c.Assert(err, check.IsNil)
	c.Assert(stdout.String(), check.Equals, "Variables successfully set.\n")
}

func (s *S) TestBsInfoRun(c *check.C) {
	var stdout, stderr bytes.Buffer
	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
	}
	conf := bsConfig{
		Envs: []bsEnv{
			{Name: "A", Value: "1"},
			{Name: "B", Value: "2"},
		},
		Pools: []bsPoolEnvs{
			{Name: "pool1", Envs: []bsEnv{
				{Name: "A", Value: "9"},
				{Name: "Z", Value: "8"},
			}},
			{Name: "pool2", Envs: []bsEnv{
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
	cmd := bsInfoCmd{}
	err = cmd.Run(&context, client)
	c.Assert(err, check.IsNil)
	c.Assert(stdout.String(), check.Equals, `Environment Variables [Default]:
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
