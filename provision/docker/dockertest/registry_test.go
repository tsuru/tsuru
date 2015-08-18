// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dockertest

import (
	"net/http"

	"github.com/tsuru/config"
	"gopkg.in/check.v1"
)

func (s *S) TestStartRegistryServer(c *check.C) {
	rollback := StartRegistryServer()
	registry, err := config.GetString("docker:registry")
	c.Assert(err, check.IsNil)
	resp, err := http.Get("http://" + registry)
	c.Assert(err, check.IsNil)
	defer resp.Body.Close()
	c.Assert(resp.StatusCode, check.Equals, http.StatusOK)
	rollback()
	_, err = config.GetString("docker:registry")
	c.Assert(err, check.NotNil)
	_, err = http.Get("http://" + registry)
	c.Assert(err, check.NotNil)
}
