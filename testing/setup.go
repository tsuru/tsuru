// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package testing provide test helpers for various actions.
package testing

import (
	"github.com/tsuru/config"
	"github.com/tsuru/gandalf/testing"
	"io/ioutil"
	"launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
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

// starts a new httptest.Server and returns it
// Also changes git:host, git:port and git:protocol to match the server's url
func StartGandalfTestServer(h http.Handler) *httptest.Server {
	ts := testing.TestServer(h)
	config.Set("git:api-server", ts.URL)
	return ts
}

func SetTargetFile(c *gocheck.C) []string {
	targetFile := os.Getenv("HOME") + "/.tsuru_target"
	_, err := os.Stat(targetFile)
	var recover []string
	if err == nil {
		old := targetFile + ".old"
		recover = []string{"mv", old, targetFile}
		exec.Command("mv", targetFile, old).Run()
	} else {
		recover = []string{"rm", targetFile}
	}
	f, err := os.Create(targetFile)
	c.Assert(err, gocheck.IsNil)
	f.Write([]byte("http://localhost"))
	f.Close()
	return recover
}

func RollbackTargetFile(rollbackCmds []string) {
	exec.Command(rollbackCmds[0], rollbackCmds[1:]...).Run()
}
