// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package testing

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	check "gopkg.in/check.v1"
)

func Test(t *testing.T) { check.TestingT(t) }

type S struct{}

var _ = check.Suite(&S{})

func (s *S) TestNewServerFreePort(c *check.C) {
	server, err := NewServer("127.0.0.1:0")
	c.Assert(err, check.IsNil)
	defer server.Stop()
	conn, err := net.Dial("tcp", server.listener.Addr().String())
	c.Assert(err, check.IsNil)
	c.Assert(conn.Close(), check.IsNil)
}

func (s *S) TestNewServerSpecificPort(c *check.C) {
	server, err := NewServer("127.0.0.1:8599")
	c.Assert(err, check.IsNil)
	defer server.Stop()
	conn, err := net.Dial("tcp", server.listener.Addr().String())
	c.Assert(err, check.IsNil)
	c.Assert(conn.Close(), check.IsNil)
}

func (s *S) TestNewServerListenError(c *check.C) {
	listen, err := net.Listen("tcp", "127.0.0.1:0")
	c.Assert(err, check.IsNil)
	defer listen.Close()
	server, err := NewServer(listen.Addr().String())
	c.Assert(err, check.ErrorMatches, `^.*bind: address already in use$`)
	c.Assert(server, check.IsNil)
}

func (s *S) TestServerStop(c *check.C) {
	server, err := NewServer("127.0.0.1:0")
	c.Assert(err, check.IsNil)
	err = server.Stop()
	c.Assert(err, check.IsNil)
	_, err = net.Dial("tcp", server.listener.Addr().String())
	c.Assert(err, check.ErrorMatches, `^.*connection refused$`)
}

func (s *S) TestAddr(c *check.C) {
	server, err := NewServer("127.0.0.1:0")
	c.Assert(err, check.IsNil)
	defer server.Stop()
	expected := server.listener.Addr().String()
	c.Assert(server.Addr(), check.Equals, expected)
}

func (s *S) TestListTags(c *check.C) {
	repo := Repository{Name: "app/image", Tags: map[string]string{"v1": "abc"}}
	server, err := NewServer("127.0.0.1:0")
	c.Assert(err, check.IsNil)
	defer server.Stop()
	server.Repos = []Repository{repo}
	recorder := httptest.NewRecorder()
	request, _ := http.NewRequest("GET", "/v2/app/image/tags/list", nil)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var got tagListResponse
	err = json.NewDecoder(recorder.Body).Decode(&got)
	c.Assert(err, check.IsNil)
	c.Assert(got, check.DeepEquals, tagListResponse{Name: "app/image", Tags: []string{"v1"}})
}

func (s *S) TestGetDigest(c *check.C) {
	repo := Repository{Name: "app/image", Tags: map[string]string{"v1": "abc"}}
	server, err := NewServer("127.0.0.1:0")
	c.Assert(err, check.IsNil)
	defer server.Stop()
	server.Repos = []Repository{repo}
	recorder := httptest.NewRecorder()
	request, _ := http.NewRequest("HEAD", "/v2/app/image/manifests/v1", nil)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Docker-Content-Digest"), check.Equals, "abc")
}

func (s *S) TestRemoveTag(c *check.C) {
	server, err := NewServer("127.0.0.1:0")
	c.Assert(err, check.IsNil)
	defer server.Stop()
	server.Repos = []Repository{{Name: "app/image", Tags: map[string]string{"v1": "abc"}}}
	recorder := httptest.NewRecorder()
	request, _ := http.NewRequest("DELETE", "/v2/app/image/manifests/abc", nil)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusAccepted)
	c.Assert(server.Repos, check.HasLen, 1)
	c.Assert(server.Repos[0].Tags, check.HasLen, 0)
}

func (s *S) TestRemoveTagNotFound(c *check.C) {
	server, err := NewServer("127.0.0.1:0")
	c.Assert(err, check.IsNil)
	defer server.Stop()
	recorder := httptest.NewRecorder()
	request, _ := http.NewRequest("DELETE", "/v2/app/image/manifests/abc", nil)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, "unknown repository name=app/image\n")
}
