// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package testing provide test helpers for various actions.
package testing

import (
	"github.com/globocom/config"
	"launchpad.net/goamz/iam/iamtest"
	"launchpad.net/goamz/s3/s3test"
	. "launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
)

type T struct {
	Admin     user
	AdminTeam team
	S3Server  *s3test.Server
	IamServer *iamtest.Server
	GitHost   string
	GitPort   string
	GitProt   string
}

type user struct {
	Email    string
	Password string
}

type team struct {
	Name  string `bson:"_id"`
	Users []string
}

func (t *T) StartAmzS3AndIAM(c *C) {
	var err error
	t.S3Server, err = s3test.NewServer(&s3test.Config{Send409Conflict: true})
	c.Assert(err, IsNil)
	config.Set("aws:s3:endpoint", t.S3Server.URL())
	t.IamServer, err = iamtest.NewServer()
	c.Assert(err, IsNil)
	config.Set("aws:iam:endpoint", t.IamServer.URL())
	config.Unset("aws:s3:bucketEndpoint")
}

func (t *T) SetGitConfs(c *C) {
	t.GitHost, _ = config.GetString("git:host")
	t.GitPort, _ = config.GetString("git:port")
	t.GitProt, _ = config.GetString("git:protocol")
}

func (t *T) RollbackGitConfs(c *C) {
	config.Set("git:host", t.GitHost)
	config.Set("git:port", t.GitPort)
	config.Set("git:protocol", t.GitProt)
}

// starts a new httptest.Server and returns it
// Also changes git:host, git:port and git:protocol to match the server's url
func (t *T) StartGandalfTestServer(h http.Handler) *httptest.Server {
	ts := httptest.NewServer(h)
	pieces := strings.Split(ts.URL, "://")
	protocol := pieces[0]
	hostPart := strings.Split(pieces[1], ":")
	port := hostPart[1]
	host := hostPart[0]
	config.Set("git:host", host)
	portInt, _ := strconv.ParseInt(port, 10, 0)
	config.Set("git:port", portInt)
	config.Set("git:protocol", protocol)
	return ts
}
