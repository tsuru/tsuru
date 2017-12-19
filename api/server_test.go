// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"time"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/auth"
	"gopkg.in/check.v1"
)

func authorizedTsuruHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	fmt.Fprint(w, r.Method)
	return nil
}

func selectAvailablePort() (string, error) {
	var err error
	for i := 0; i < 20; i++ {
		port := strconv.Itoa(rand.Intn(20000) + 8000)
		var conn net.Listener
		conn, err = net.Listen("tcp", "localhost:"+port)
		if err == nil {
			conn.Close()
			return port, nil
		}
	}
	return "", err
}

func waitForServer(addr string) error {
	var err error
	for i := 0; i < 100; i++ {
		_, err = net.DialTimeout("tcp", addr, 1*time.Second)
		if err == nil {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return err
}

func (s *S) TestRegisterHandlerMakesHandlerAvailableViaGet(c *check.C) {
	RegisterHandler("/foo/bar", "GET", AuthorizationRequiredHandler(authorizedTsuruHandler))
	defer resetHandlers()
	rec := httptest.NewRecorder()
	req, err := http.NewRequest("GET", "http://example.com/foo/bar", nil)
	req.Header.Set("Authorization", "bearer "+s.token.GetValue())
	c.Assert(err, check.IsNil)
	m := RunServer(true)
	m.ServeHTTP(rec, req)
	c.Assert("GET", check.Equals, rec.Body.String())
}

func (s *S) TestRegisterHandlerMakesHandlerAvailableViaPost(c *check.C) {
	RegisterHandler("/foo/bar", "POST", AuthorizationRequiredHandler(authorizedTsuruHandler))
	defer resetHandlers()
	rec := httptest.NewRecorder()
	req, err := http.NewRequest("POST", "http://example.com/foo/bar", nil)
	req.Header.Set("Authorization", "bearer "+s.token.GetValue())
	c.Assert(err, check.IsNil)
	m := RunServer(true)
	m.ServeHTTP(rec, req)
	c.Assert("POST", check.Equals, rec.Body.String())
}

func (s *S) TestRegisterHandlerMakesHandlerAvailableViaPut(c *check.C) {
	RegisterHandler("/foo/bar", "PUT", AuthorizationRequiredHandler(authorizedTsuruHandler))
	defer resetHandlers()
	rec := httptest.NewRecorder()
	req, err := http.NewRequest("PUT", "http://example.com/foo/bar", nil)
	req.Header.Set("Authorization", "bearer "+s.token.GetValue())
	c.Assert(err, check.IsNil)
	m := RunServer(true)
	m.ServeHTTP(rec, req)
	c.Assert("PUT", check.Equals, rec.Body.String())
}

func (s *S) TestRegisterHandlerMakesHandlerAvailableViaDelete(c *check.C) {
	RegisterHandler("/foo/bar", "DELETE", AuthorizationRequiredHandler(authorizedTsuruHandler))
	defer resetHandlers()
	rec := httptest.NewRecorder()
	req, err := http.NewRequest("DELETE", "http://example.com/foo/bar", nil)
	req.Header.Set("Authorization", "bearer "+s.token.GetValue())
	c.Assert(err, check.IsNil)
	m := RunServer(true)
	m.ServeHTTP(rec, req)
	c.Assert("DELETE", check.Equals, rec.Body.String())
}

func (s *S) TestIsNotAdmin(c *check.C) {
	RegisterHandler("/foo/bar", "POST", AuthorizationRequiredHandler(authorizedTsuruHandler))
	defer resetHandlers()
	rec := httptest.NewRecorder()
	req, err := http.NewRequest("POST", "http://example.com/foo/bar", nil)
	req.Header.Set("Authorization", "bearer "+s.token.GetValue())
	c.Assert(err, check.IsNil)
	m := RunServer(true)
	m.ServeHTTP(rec, req)
	c.Assert("POST", check.Equals, rec.Body.String())
}

func (s *S) TestRunHTTPServer(c *check.C) {
	port, err := selectAvailablePort()
	c.Assert(err, check.IsNil)
	config.Set("listen", "0.0.0.0:"+port)
	config.Set("queue:mongo-url", "127.0.0.1:27017")
	config.Set("queue:mongo-database", "queuedb")

	go RunServer(false)

	err = waitForServer("localhost:" + port)
	c.Assert(err, check.IsNil)
}

func (s *S) TestRunHTTPSServer(c *check.C) {
	port, err := selectAvailablePort()
	c.Assert(err, check.IsNil)
	config.Set("listen", "0.0.0.0:"+port)
	config.Set("queue:mongo-url", "127.0.0.1:27017")
	config.Set("queue:mongo-database", "queuedb")
	config.Set("use-tls", true)
	config.Set("tls:cert-file", "./testdata/cert.pem")
	config.Set("tls:key-file", "./testdata/key.pem")
	defer config.Unset("use-tls")

	go RunServer(false)

	err = waitForServer("localhost:" + port)
	c.Assert(err, check.IsNil)
}

func (s *S) TestRunHTTPAndHTTPSServers(c *check.C) {
	httpPort, err := selectAvailablePort()
	c.Assert(err, check.IsNil)
	httpsPort, err := selectAvailablePort()
	c.Assert(err, check.IsNil)
	config.Set("listen", "0.0.0.0:"+httpPort)
	config.Set("queue:mongo-url", "127.0.0.1:27017")
	config.Set("queue:mongo-database", "queuedb")
	config.Set("use-tls", true)
	config.Set("tls:listen", "0.0.0.0:"+httpsPort)
	config.Set("tls:cert-file", "./testdata/cert.pem")
	config.Set("tls:key-file", "./testdata/key.pem")
	defer config.Unset("use-tls")

	go RunServer(false)

	err = waitForServer("localhost:" + httpPort)
	c.Assert(err, check.IsNil)
	err = waitForServer("localhost:" + httpsPort)
	c.Assert(err, check.IsNil)
}
