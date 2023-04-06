// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package oauth

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	check "gopkg.in/check.v1"
)

func Test(t *testing.T) { check.TestingT(t) }

type S struct {
	conn   *db.Storage
	server *httptest.Server
	reqs   []*http.Request
	bodies []string
	rsps   map[string]string
}

var _ = check.Suite(&S{})

func (s *S) SetUpSuite(c *check.C) {
	s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		c.Assert(err, check.IsNil)
		s.bodies = append(s.bodies, string(b))
		s.reqs = append(s.reqs, r)
		w.Write([]byte(s.rsps[r.URL.Path]))
	}))
	config.Set("auth:oauth:client-id", "clientid")
	config.Set("auth:oauth:client-secret", "clientsecret")
	config.Set("auth:oauth:scope", "myscope")
	config.Set("auth:oauth:auth-url", s.server.URL+"/auth")
	config.Set("auth:oauth:token-url", s.server.URL+"/token")
	config.Set("auth:oauth:info-url", s.server.URL+"/user")
	config.Set("auth:oauth:collection", "oauth_token")
	config.Set("database:url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("database:name", "tsuru_auth_oauth_test")
	config.Set("auth:user-registration", true)
}

func (s *S) SetUpTest(c *check.C) {
	s.conn, _ = db.Conn()
	s.reqs = make([]*http.Request, 0)
	s.bodies = make([]string, 0)
	s.rsps = make(map[string]string)
}

func (s *S) TearDownTest(c *check.C) {
	err := dbtest.ClearAllCollections(s.conn.Users().Database)
	c.Assert(err, check.IsNil)
	s.conn.Close()
}

func (s *S) TearDownSuite(c *check.C) {
	s.server.Close()
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	dbtest.ClearAllCollections(conn.Users().Database)
}
