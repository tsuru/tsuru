// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package testing provide test helpers for various actions.
package testing

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strconv"

	"github.com/tsuru/config"
	"github.com/tsuru/gandalf/testing"
	"launchpad.net/gocheck"
)

type T struct {
	Admin        user
	AdminTeam    team
	GitAPIServer string
	GitRWHost    string
	GitROHost    string
}

type user struct {
	Email    string
	Password string
}

type team struct {
	Name  string `bson:"_id"`
	Users []string
}

func (t *T) SetGitConfs(c *gocheck.C) {
	t.GitAPIServer, _ = config.GetString("git:api-server")
	t.GitROHost, _ = config.GetString("git:ro-host")
	t.GitRWHost, _ = config.GetString("git:rw-host")
}

func (t *T) RollbackGitConfs(c *gocheck.C) {
	config.Set("git:api-server", t.GitAPIServer)
	config.Set("git:ro-host", t.GitROHost)
	config.Set("git:rw-host", t.GitRWHost)
}

type TestHandler struct {
	Body    []byte
	Method  string
	Url     string
	Content string
	Header  http.Header
}

func (h *TestHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.Method = r.Method
	h.Url = r.URL.String()
	b, _ := ioutil.ReadAll(r.Body)
	h.Body = b
	h.Header = r.Header
	w.Write([]byte(h.Content))
}

type MultiTestHandler struct {
	Body               [][]byte
	Method             []string
	Url                []string
	Content            string
	ConditionalContent map[string]interface{}
	Header             []http.Header
	RspCode            int
}

func (h *MultiTestHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.Method = append(h.Method, r.Method)
	h.Url = append(h.Url, r.URL.String())
	b, _ := ioutil.ReadAll(r.Body)
	h.Body = append(h.Body, b)
	h.Header = append(h.Header, r.Header)
	if h.RspCode == 0 {
		h.RspCode = http.StatusOK
	}
	condContent := h.ConditionalContent[r.URL.String()]
	if content, ok := condContent.(string); ok {
		w.WriteHeader(h.RspCode)
		w.Write([]byte(content))
	} else if content, ok := condContent.([]string); ok {
		code, _ := strconv.Atoi(content[0])
		w.WriteHeader(code)
		w.Write([]byte(content[1]))
	} else {
		w.WriteHeader(h.RspCode)
		w.Write([]byte(h.Content))
	}
}

// starts a new httptest.Server and returns it
// Also changes git:host, git:port and git:protocol to match the server's url
func StartGandalfTestServer(h http.Handler) *httptest.Server {
	ts := testing.TestServer(h)
	config.Set("git:api-server", ts.URL)
	return ts
}
