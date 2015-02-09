// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package repository

import (
	"net/http/httptest"
	"testing"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/api/apitest"
	"github.com/tsuru/tsuru/repository/repositorytest"
	"gopkg.in/check.v1"
)

func Test(t *testing.T) { check.TestingT(t) }

type S struct {
	ts *httptest.Server
	h  *apitest.TestHandler
}

var _ = check.Suite(&S{})

func (s *S) SetUpSuite(c *check.C) {
	config.Set("git:api-server", "http://mygihost:8090")
	config.Set("git:rw-host", "public.mygithost")
	config.Set("git:ro-host", "private.mygithost")
	config.Set("git:unit-repo", "/home/application/current")
	content := `{"git_url":"git://git.tsuru.io/foobar.git","ssh_url":"git@git.tsuru.io:foobar.git"}`
	s.h = &apitest.TestHandler{Content: content}
	s.ts = repositorytest.StartGandalfTestServer(s.h)
}

func (s *S) TearDownSuite(c *check.C) {
	s.ts.Close()
}
