// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package repository

import (
	"github.com/globocom/config"
	tsrTesting "github.com/globocom/tsuru/testing"
	"launchpad.net/gocheck"
	"net/http/httptest"
	"testing"
)

func Test(t *testing.T) { gocheck.TestingT(t) }

type S struct {
	ts *httptest.Server
	h  *tsrTesting.TestHandler
}

var _ = gocheck.Suite(&S{})

func (s *S) SetUpSuite(c *gocheck.C) {
	config.Set("git:api-server", "http://mygihost:8090")
	config.Set("git:rw-host", "public.mygithost")
	config.Set("git:ro-host", "private.mygithost")
	config.Set("git:unit-repo", "/home/application/current")
	content := `{"ssh_url":"git://git.tsuru.io/foobar.git","git_url":"git@git.tsuru.io:foobar.git"}`
	s.h = &tsrTesting.TestHandler{Content: content}
	t := &tsrTesting.T{}
	s.ts = t.StartGandalfTestServer(s.h)
}

func (s *S) TearDownSuite(c *gocheck.C) {
	s.ts.Close()
}
