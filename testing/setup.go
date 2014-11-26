// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package testing provide test helpers for various actions.
package testing

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"

	"github.com/tsuru/config"
	"github.com/tsuru/gandalf/testing"
	"gopkg.in/mgo.v2"
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

// starts a new httptest.Server and returns it
// Also changes git:host, git:port and git:protocol to match the server's url
func StartGandalfTestServer(h http.Handler) *httptest.Server {
	ts := testing.TestServer(h)
	config.Set("git:api-server", ts.URL)
	return ts
}

func SetTargetFile(c *gocheck.C, target []byte) []string {
	return writeHomeFile(c, ".tsuru_target", target)
}

func SetTokenFile(c *gocheck.C, token []byte) []string {
	return writeHomeFile(c, ".tsuru_token", token)
}

func RollbackFile(rollbackCmds []string) {
	exec.Command(rollbackCmds[0], rollbackCmds[1:]...).Run()
}

func writeHomeFile(c *gocheck.C, filename string, content []byte) []string {
	file := os.Getenv("HOME") + "/" + filename
	_, err := os.Stat(file)
	var recover []string
	if err == nil {
		var old string
		for i := 0; err == nil; i++ {
			old = file + fmt.Sprintf(".old-%d", i)
			_, err = os.Stat(old)
		}
		recover = []string{"mv", old, file}
		exec.Command("mv", file, old).Run()
	} else {
		recover = []string{"rm", file}
	}
	f, err := os.Create(file)
	c.Assert(err, gocheck.IsNil)
	f.Write(content)
	f.Close()
	return recover
}

func ClearAllCollections(db *mgo.Database) error {
	colls, err := db.CollectionNames()
	if err != nil {
		return err
	}
	for _, collName := range colls {
		if strings.Index(collName, "system.") != -1 {
			continue
		}
		coll := db.C(collName)
		_, err = coll.RemoveAll(nil)
		if err != nil {
			coll.DropCollection()
		}
	}
	return nil
}
