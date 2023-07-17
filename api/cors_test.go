// Copyright 2023 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"net/http"

	"github.com/tsuru/config"
	check "gopkg.in/check.v1"
)

func (s *S) TestCorsMiddleware(c *check.C) {
	cors := corsMiddleware()
	c.Assert(cors, check.IsNil)
}

func (s *S) TestCorsMiddlewareWithList(c *check.C) {
	config.Set("cors:allowed-origins", []string{"g1.globo.com"})
	defer config.Unset("cors:allowed-origins")

	cors := corsMiddleware()
	c.Assert(cors, check.Not(check.IsNil))

	req, _ := http.NewRequest("GET", "/", nil)
	req.Header.Set("Origin", "g1.globo.com")
	c.Assert(cors.OriginAllowed(req), check.Equals, true)

	req, _ = http.NewRequest("GET", "/", nil)
	req.Header.Set("Origin", "gshow.globo.com")
	c.Assert(cors.OriginAllowed(req), check.Equals, false)
}
