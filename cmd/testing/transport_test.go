// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package testing

import (
	"io/ioutil"
	"launchpad.net/gocheck"
	"net/http"
	"testing"
)

type S struct{}

var _ = gocheck.Suite(S{})

func Test(t *testing.T) {
	gocheck.TestingT(t)
}

func (S) TestTransport(c *gocheck.C) {
	var t http.RoundTripper = &Transport{
		Message: "Ok",
		Status:  http.StatusOK,
		Headers: map[string][]string{"Authorization": {"something"}},
	}
	req, _ := http.NewRequest("GET", "/", nil)
	r, err := t.RoundTrip(req)
	c.Assert(err, gocheck.IsNil)
	c.Assert(r.StatusCode, gocheck.Equals, http.StatusOK)
	defer r.Body.Close()
	b, _ := ioutil.ReadAll(r.Body)
	c.Assert(string(b), gocheck.Equals, "Ok")
	c.Assert(r.Header.Get("Authorization"), gocheck.Equals, "something")
}

func (S) TestConditionalTransport(c *gocheck.C) {
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
	c.Assert(err, gocheck.IsNil)
	c.Assert(r.StatusCode, gocheck.Equals, http.StatusOK)
	defer r.Body.Close()
	b, _ := ioutil.ReadAll(r.Body)
	c.Assert(string(b), gocheck.Equals, "Ok")
	req, _ = http.NewRequest("GET", "/", nil)
	r, err = t.RoundTrip(req)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "condition failed")
	c.Assert(r.StatusCode, gocheck.Equals, http.StatusInternalServerError)
}
