// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package repository

import (
	"github.com/globocom/config"
	tsrTesting "github.com/globocom/tsuru/testing"
	"io/ioutil"
	"launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
	"testing"
)

func Test(t *testing.T) { gocheck.TestingT(t) }

type S struct {
	ts *httptest.Server
	h  *testHandler
}

var _ = gocheck.Suite(&S{})

type testHandler struct {
	body    []byte
	method  string
	url     string
	content string
	header  http.Header
}

func (h *testHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.method = r.Method
	h.url = r.URL.String()
	b, _ := ioutil.ReadAll(r.Body)
	h.body = b
	h.header = r.Header
	w.Write([]byte(h.content))
}

func (s *S) SetUpSuite(c *gocheck.C) {
	config.Set("git:api-server", "http://mygihost:8090")
	config.Set("git:rw-host", "public.mygithost")
	config.Set("git:ro-host", "private.mygithost")
	config.Set("git:unit-repo", "/home/application/current")
	content := `{"ssh_url":"git://git.tsuru.io/foobar.git","git_url":"git@git.tsuru.io:foobar.git"}`
	s.h = &testHandler{content: content}
	t := &tsrTesting.T{}
	s.ts = t.StartGandalfTestServer(s.h)
}

func (s *S) TearDownSuite(c *gocheck.C) {
	s.ts.Close()
}
