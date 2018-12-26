// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmds

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"testing"

	"github.com/tsuru/tsuru/cmd"
	"github.com/tsuru/tsuru/cmd/cmdtest"
	tsuruIo "github.com/tsuru/tsuru/io"
	"github.com/tsuru/tsuru/provision/docker/types"
	check "gopkg.in/check.v1"
)

func Test(t *testing.T) {
	check.TestingT(t)
}

var _ = check.Suite(&S{})

type S struct{}

func (s *S) SetUpSuite(c *check.C) {
	os.Setenv("TSURU_TARGET", "http://localhost")
}

func (s *S) TestDockerLogUpdateRun(c *check.C) {
	var stdout, stderr bytes.Buffer
	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
	}
	msg := tsuruIo.SimpleJsonMessage{Message: "success!!!"}
	result, _ := json.Marshal(msg)
	trans := &cmdtest.ConditionalTransport{
		Transport: cmdtest.Transport{Message: string(result), Status: http.StatusNoContent},
		CondFunc: func(req *http.Request) bool {
			err := req.ParseForm()
			c.Assert(err, check.IsNil)
			c.Assert(req.Form, check.DeepEquals, url.Values{
				"restart":   []string{"false"},
				"pool":      []string{""},
				"Driver":    []string{"x"},
				"LogOpts.a": []string{"1"},
				"LogOpts.b": []string{"2"},
			})
			return req.URL.Path == "/1.0/docker/logs" && req.Method == "POST"
		},
	}
	manager := cmd.NewManager("admin", "0.1", "admin-ver", &stdout, &stderr, nil, nil)
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, manager)
	updateCmd := dockerLogUpdate{}
	err := updateCmd.Flags().Parse(true, []string{"--log-driver", "x", "--log-opt", "a=1", "--log-opt", "b=2"})
	c.Assert(err, check.IsNil)
	err = updateCmd.Run(&context, client)
	c.Assert(err, check.IsNil)
	c.Assert(stdout.String(), check.Equals, "success!!!")
}

func (s *S) TestDockerLogUpdateForPoolRun(c *check.C) {
	var stdout, stderr bytes.Buffer
	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
	}
	msg := tsuruIo.SimpleJsonMessage{Message: "success!!!"}
	result, _ := json.Marshal(msg)
	trans := &cmdtest.ConditionalTransport{
		Transport: cmdtest.Transport{Message: string(result), Status: http.StatusNoContent},
		CondFunc: func(req *http.Request) bool {
			err := req.ParseForm()
			c.Assert(err, check.IsNil)
			c.Assert(req.Form, check.DeepEquals, url.Values{
				"restart":   []string{"false"},
				"pool":      []string{"p1"},
				"Driver":    []string{"x"},
				"LogOpts.a": []string{"1"},
				"LogOpts.b": []string{"2"},
			})
			return req.URL.Path == "/1.0/docker/logs" && req.Method == "POST"
		},
	}
	manager := cmd.NewManager("admin", "0.1", "admin-ver", &stdout, &stderr, nil, nil)
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, manager)
	updateCmd := dockerLogUpdate{}
	err := updateCmd.Flags().Parse(true, []string{"--pool", "p1", "--log-driver", "x", "--log-opt", "a=1", "--log-opt", "b=2"})
	c.Assert(err, check.IsNil)
	err = updateCmd.Run(&context, client)
	c.Assert(err, check.IsNil)
	c.Assert(stdout.String(), check.Equals, "success!!!")
}

func (s *S) TestDockerLogInfoRun(c *check.C) {
	var stdout, stderr bytes.Buffer
	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
	}
	conf := map[string]types.DockerLogConfig{
		"":   {Driver: "x", LogOpts: map[string]string{"a": "1", "b": "2"}},
		"p1": {Driver: "x", LogOpts: map[string]string{"a": "9"}},
		"p2": {Driver: "bs"},
	}
	result, _ := json.Marshal(conf)
	trans := &cmdtest.ConditionalTransport{
		Transport: cmdtest.Transport{Message: string(result), Status: http.StatusNoContent},
		CondFunc: func(req *http.Request) bool {
			return req.URL.Path == "/1.0/docker/logs" && req.Method == "GET"
		},
	}
	manager := cmd.NewManager("admin", "0.1", "admin-ver", &stdout, &stderr, nil, nil)
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, manager)
	cmd := dockerLogInfo{}
	err := cmd.Run(&context, client)
	c.Assert(err, check.IsNil)
	c.Assert(stdout.String(), check.Equals, `Log driver [default]: x
+------+-------+
| Name | Value |
+------+-------+
| a    | 1     |
| b    | 2     |
+------+-------+

Log driver [pool p1]: x
+------+-------+
| Name | Value |
+------+-------+
| a    | 9     |
+------+-------+

Log driver [pool p2]: bs
`)
}
