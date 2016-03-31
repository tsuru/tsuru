// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bs

import (
	"github.com/fsouza/go-dockerclient/testing"
	"github.com/tsuru/config"
	"gopkg.in/check.v1"
)

func (s *S) TestWaitDocker(c *check.C) {
	server, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	defer server.Stop()
	var task runBs
	err = task.waitDocker(server.URL())
	c.Assert(err, check.IsNil)
	config.Set("docker:api-timeout", 1)
	defer config.Unset("docker:api-timeout")
	err = task.waitDocker("http://169.254.169.254:2375/")
	c.Assert(err, check.NotNil)
	expectedMsg := `Docker API at "http://169.254.169.254:2375/" didn't respond after 1 seconds`
	c.Assert(err.Error(), check.Equals, expectedMsg)
}
