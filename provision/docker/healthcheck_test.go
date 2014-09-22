// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"bytes"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"time"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"gopkg.in/mgo.v2/bson"
	"launchpad.net/gocheck"
)

func (s *S) TestHealthcheck(c *gocheck.C) {
	var requests []*http.Request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()
	a := app.App{Name: "myapp1", CustomData: map[string]interface{}{
		"healthcheck": map[string]interface{}{
			"path":   "/x/y",
			"method": "Post",
			"status": http.StatusCreated,
		},
	}}
	err := s.storage.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.storage.Apps().RemoveAll(bson.M{"name": a.Name})
	url, _ := url.Parse(server.URL)
	host, port, _ := net.SplitHostPort(url.Host)
	cont := container{AppName: a.Name, HostAddr: host, HostPort: port}
	buf := bytes.Buffer{}
	err = runHealthcheck(&cont, &buf)
	c.Assert(err, gocheck.IsNil)
	c.Assert(requests, gocheck.HasLen, 1)
	c.Assert(requests[0].URL.Path, gocheck.Equals, "/x/y")
	c.Assert(requests[0].Method, gocheck.Equals, "POST")
	c.Assert(buf.String(), gocheck.Equals, " ---> healthcheck successful()\n")
}

func (s *S) TestHealthcheckWithMatch(c *gocheck.C) {
	var requests []*http.Request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		if len(requests) == 1 {
			w.Write([]byte("invalid"))
		} else {
			w.Write([]byte("something"))
		}
	}))
	defer server.Close()
	a := app.App{Name: "myapp1", CustomData: map[string]interface{}{
		"healthcheck": map[string]interface{}{
			"path":   "/x/y",
			"method": "Get",
			"status": 200,
			"match":  ".*some.*",
		},
	}}
	err := s.storage.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.storage.Apps().RemoveAll(bson.M{"name": a.Name})
	url, _ := url.Parse(server.URL)
	host, port, _ := net.SplitHostPort(url.Host)
	cont := container{AppName: a.Name, HostAddr: host, HostPort: port}
	buf := bytes.Buffer{}
	err = runHealthcheck(&cont, &buf)
	c.Assert(err, gocheck.ErrorMatches, ".*unexpected result, expected \"(?s).*some.*\", got: invalid")
	c.Assert(requests, gocheck.HasLen, 1)
	c.Assert(requests[0].Method, gocheck.Equals, "GET")
	err = runHealthcheck(&cont, &buf)
	c.Assert(err, gocheck.IsNil)
	c.Assert(requests, gocheck.HasLen, 2)
	c.Assert(requests[1].URL.Path, gocheck.Equals, "/x/y")
	c.Assert(requests[1].Method, gocheck.Equals, "GET")
}

func (s *S) TestHealthcheckDefaultCheck(c *gocheck.C) {
	var requests []*http.Request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	a := app.App{Name: "myapp1", CustomData: map[string]interface{}{
		"healthcheck": map[string]interface{}{
			"path": "/x/y",
		},
	}}
	err := s.storage.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.storage.Apps().RemoveAll(bson.M{"name": a.Name})
	url, _ := url.Parse(server.URL)
	host, port, _ := net.SplitHostPort(url.Host)
	cont := container{AppName: a.Name, HostAddr: host, HostPort: port}
	buf := bytes.Buffer{}
	err = runHealthcheck(&cont, &buf)
	c.Assert(err, gocheck.IsNil)
	c.Assert(requests, gocheck.HasLen, 1)
	c.Assert(requests[0].Method, gocheck.Equals, "GET")
	c.Assert(requests[0].URL.Path, gocheck.Equals, "/x/y")
}

func (s *S) TestHealthcheckNoHealthcheck(c *gocheck.C) {
	var requests []*http.Request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	a := app.App{Name: "myapp1"}
	err := s.storage.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.storage.Apps().RemoveAll(bson.M{"name": a.Name})
	url, _ := url.Parse(server.URL)
	host, port, _ := net.SplitHostPort(url.Host)
	cont := container{AppName: a.Name, HostAddr: host, HostPort: port}
	buf := bytes.Buffer{}
	err = runHealthcheck(&cont, &buf)
	c.Assert(err, gocheck.IsNil)
	c.Assert(requests, gocheck.HasLen, 0)
}

func (s *S) TestHealthcheckNoPath(c *gocheck.C) {
	var requests []*http.Request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	a := app.App{Name: "myapp1", CustomData: map[string]interface{}{
		"healthcheck": map[string]interface{}{
			"method": "GET",
			"status": 200,
		},
	}}
	err := s.storage.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.storage.Apps().RemoveAll(bson.M{"name": a.Name})
	url, _ := url.Parse(server.URL)
	host, port, _ := net.SplitHostPort(url.Host)
	cont := container{AppName: a.Name, HostAddr: host, HostPort: port}
	buf := bytes.Buffer{}
	err = runHealthcheck(&cont, &buf)
	c.Assert(err, gocheck.IsNil)
	c.Assert(requests, gocheck.HasLen, 0)
}

func (s *S) TestHealthcheckKeepsTryingWithServerDown(c *gocheck.C) {
	var requests []*http.Request
	lock := sync.Mutex{}
	shouldRun := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lock.Lock()
		defer lock.Unlock()
		requests = append(requests, r)
		if shouldRun {
			w.WriteHeader(http.StatusOK)
		} else {
			hj := w.(http.Hijacker)
			conn, _, _ := hj.Hijack()
			conn.Close()
		}
	}))
	a := app.App{Name: "myapp1", CustomData: map[string]interface{}{
		"healthcheck": map[string]interface{}{
			"path": "/x/y",
		},
	}}
	err := s.storage.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.storage.Apps().RemoveAll(bson.M{"name": a.Name})
	url, _ := url.Parse(server.URL)
	host, port, _ := net.SplitHostPort(url.Host)
	cont := container{AppName: a.Name, HostAddr: host, HostPort: port}
	buf := bytes.Buffer{}
	go func() {
		time.Sleep(1 * time.Second)
		lock.Lock()
		defer lock.Unlock()
		shouldRun = true
	}()
	err = runHealthcheck(&cont, &buf)
	c.Assert(err, gocheck.IsNil)
	c.Assert(buf.String(), gocheck.Matches, `(?s).*---> healthcheck fail.*?Trying again in 3s.*---> healthcheck successful.*`)
	c.Assert(requests, gocheck.HasLen, 2)
	c.Assert(requests[0].Method, gocheck.Equals, "GET")
	c.Assert(requests[0].URL.Path, gocheck.Equals, "/x/y")
	c.Assert(requests[1].Method, gocheck.Equals, "GET")
	c.Assert(requests[1].URL.Path, gocheck.Equals, "/x/y")
}

func (s *S) TestHealthcheckErrorsAfterMaxTime(c *gocheck.C) {
	a := app.App{Name: "myapp1", CustomData: map[string]interface{}{
		"healthcheck": map[string]interface{}{
			"path": "/x/y",
		},
	}}
	err := s.storage.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.storage.Apps().RemoveAll(bson.M{"name": a.Name})
	url, _ := url.Parse("http://some-invalid-server-name.some-invalid-server-name.com:9123")
	host, port, _ := net.SplitHostPort(url.Host)
	cont := container{AppName: a.Name, HostAddr: host, HostPort: port}
	buf := bytes.Buffer{}
	config.Set("docker:healthcheck:max-time", 1)
	defer config.Unset("docker:healthcheck:max-time")
	done := make(chan struct{})
	go func() {
		err = runHealthcheck(&cont, &buf)
		close(done)
	}()
	select {
	case <-time.After(5 * time.Second):
		c.Fatal("Timed out waiting for healthcheck to fail")
	case <-done:
	}
	c.Assert(err, gocheck.ErrorMatches, "healthcheck fail.*lookup some-invalid-server-name.some-invalid-server-name.com: no such host")
}
