// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nodecontainer

import (
	"net/http"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/fsouza/go-dockerclient/testing"
	"github.com/tsuru/config"
	"gopkg.in/check.v1"
)

func (s *S) TestWaitDocker(c *check.C) {
	server, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	defer server.Stop()
	var task runBs
	client, err := docker.NewClient(server.URL())
	c.Assert(err, check.IsNil)
	err = task.waitDocker(client)
	c.Assert(err, check.IsNil)
	server.CustomHandler("/_ping", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}))
	err = task.waitDocker(client)
	c.Assert(err, check.NotNil)
	config.Set("docker:api-timeout", 1)
	defer config.Unset("docker:api-timeout")
	client, err = docker.NewClient("http://169.254.169.254:2375/")
	c.Assert(err, check.IsNil)
	err = task.waitDocker(client)
	c.Assert(err, check.NotNil)
	expectedMsg := `Docker API at "http://169.254.169.254:2375/" didn't respond after 1 seconds`
	c.Assert(err.Error(), check.Equals, expectedMsg)
}
