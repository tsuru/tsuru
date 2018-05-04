// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmdtest

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"testing"

	"gopkg.in/check.v1"
)

type S struct{}

var _ = check.Suite(S{})

func Test(t *testing.T) {
	check.TestingT(t)
}

func (S) TestTransport(c *check.C) {
	var t http.RoundTripper = &Transport{
		Message: "Ok",
		Status:  http.StatusOK,
		Headers: map[string][]string{"Authorization": {"something"}},
	}
	req, _ := http.NewRequest("GET", "/", nil)
	r, err := t.RoundTrip(req)
	c.Assert(err, check.IsNil)
	c.Assert(r.StatusCode, check.Equals, http.StatusOK)
	defer r.Body.Close()
	b, _ := ioutil.ReadAll(r.Body)
	c.Assert(string(b), check.Equals, "Ok")
	c.Assert(r.Header.Get("Authorization"), check.Equals, "something")
}

func (S) TestBodyTransport(c *check.C) {
	var t http.RoundTripper = &BodyTransport{
		Body:    ioutil.NopCloser(bytes.NewReader([]byte("Ok"))),
		Status:  http.StatusOK,
		Headers: map[string][]string{"Authorization": {"something"}},
	}
	req, _ := http.NewRequest("GET", "/", nil)
	r, err := t.RoundTrip(req)
	c.Assert(err, check.IsNil)
	c.Assert(r.StatusCode, check.Equals, http.StatusOK)
	defer r.Body.Close()
	b, _ := ioutil.ReadAll(r.Body)
	c.Assert(string(b), check.Equals, "Ok")
	c.Assert(r.Header.Get("Authorization"), check.Equals, "something")
}

func (S) TestConditionalTransport(c *check.C) {
	var t http.RoundTripper = &ConditionalTransport{
		Transport: Transport{
			Message: "Ok",
			Status:  http.StatusOK,
		},
		CondFunc: func(req *http.Request) bool {
			return req.URL.Path == "/something"
		},
	}
	req, _ := http.NewRequest("GET", "/something", nil)
	r, err := t.RoundTrip(req)
	c.Assert(err, check.IsNil)
	c.Assert(r.StatusCode, check.Equals, http.StatusOK)
	defer r.Body.Close()
	b, _ := ioutil.ReadAll(r.Body)
	c.Assert(string(b), check.Equals, "Ok")
	req, _ = http.NewRequest("GET", "/", nil)
	r, err = t.RoundTrip(req)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "condition failed")
	c.Assert(r.StatusCode, check.Equals, http.StatusInternalServerError)
}

func (S) TestMultiConditionalTransport(c *check.C) {
	t1 := ConditionalTransport{
		Transport: Transport{
			Message: "Unauthorized",
			Status:  http.StatusUnauthorized,
		},
		CondFunc: func(req *http.Request) bool {
			return req.URL.Path == "/something"
		},
	}
	t2 := ConditionalTransport{
		Transport: Transport{
			Message: "OK",
			Status:  http.StatusOK,
		},
		CondFunc: func(req *http.Request) bool {
			return req.URL.Path == "/something"
		},
	}
	m := MultiConditionalTransport{
		ConditionalTransports: []ConditionalTransport{t1, t2},
	}
	c.Assert(m.ConditionalTransports, check.HasLen, 2)
	req, _ := http.NewRequest("GET", "/something", nil)
	r, err := m.RoundTrip(req)
	c.Assert(err, check.IsNil)
	c.Assert(r.StatusCode, check.Equals, http.StatusUnauthorized)
	c.Assert(m.ConditionalTransports, check.HasLen, 1)
	r, err = m.RoundTrip(req)
	c.Assert(err, check.IsNil)
	c.Assert(r.StatusCode, check.Equals, http.StatusOK)
	c.Assert(m.ConditionalTransports, check.HasLen, 0)
}

func (S) TestAnyConditionalTransport(c *check.C) {
	t1 := ConditionalTransport{
		Transport: Transport{
			Message: "some response",
			Status:  http.StatusUnauthorized,
		},
		CondFunc: func(req *http.Request) bool {
			return req.URL.Path == "/foo" && req.Method == http.MethodPost
		},
	}
	t2 := ConditionalTransport{
		Transport: Transport{
			Message: "something else",
			Status:  http.StatusOK,
		},
		CondFunc: func(req *http.Request) bool {
			return req.URL.Path == "/bar"
		},
	}
	a := AnyConditionalTransport{
		ConditionalTransports: []ConditionalTransport{t1, t2},
	}
	req, _ := http.NewRequest(http.MethodGet, "/bar", nil)
	r, err := a.RoundTrip(req)
	c.Assert(err, check.IsNil)
	c.Assert(r.StatusCode, check.Equals, http.StatusOK)
	body, _ := ioutil.ReadAll(r.Body)
	c.Assert(string(body), check.Equals, "something else")
	r, err = a.RoundTrip(req)
	c.Assert(err, check.IsNil)
	c.Assert(r.StatusCode, check.Equals, http.StatusOK)
	body, _ = ioutil.ReadAll(r.Body)
	c.Assert(string(body), check.Equals, "something else")
	req, _ = http.NewRequest(http.MethodGet, "/foo", nil)
	r, err = a.RoundTrip(req)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "all conditions failed")
	c.Assert(r.StatusCode, check.Equals, http.StatusInternalServerError)
	req, _ = http.NewRequest(http.MethodPost, "/foo", nil)
	r, err = a.RoundTrip(req)
	c.Assert(err, check.IsNil)
	c.Assert(r.StatusCode, check.Equals, http.StatusUnauthorized)
	body, _ = ioutil.ReadAll(r.Body)
	c.Assert(string(body), check.Equals, "some response")
}
