// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cloudstack

import (
	"fmt"
	"github.com/tsuru/config"
	"launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func Test(t *testing.T) { gocheck.TestingT(t) }

type cloudstackSuite struct{}

var _ = gocheck.Suite(&cloudstackSuite{})

func (s *cloudstackSuite) SetUpSuite(c *gocheck.C) {
	config.Set("cloudstack:api-key", "test")
	config.Set("cloudstack:secret-key", "test")
	config.Set("cloudstack:url", "test")
}

func (s *cloudstackSuite) TestCreateMachine(c *gocheck.C) {
	fakeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-type", "application/json")
		fmt.Fprintln(w, `{"id": "test", "jobid": "test"}`)
	}))
	defer fakeServer.Close()
	config.Set("cloudstack:url", fakeServer.URL)
	var cs CloudstackIaaS
	params := map[string]interface{}{"name": "test"}
	vm, err := cs.CreateMachine(params)
	c.Assert(err, gocheck.IsNil)
	c.Assert(vm, gocheck.NotNil)
	c.Assert(vm.GetAddress(), gocheck.NotNil)
}

func (s *cloudstackSuite) TestBuildUrlToCloudstack(c *gocheck.C) {
	params := map[string]interface{}{"atest": "2"}
	urlBuilded, err := buildUrl("commandTest", params)
	c.Assert(err, gocheck.IsNil)
	u, err := url.Parse(urlBuilded)
	c.Assert(err, gocheck.IsNil)
	q, err := url.ParseQuery(u.RawQuery)
	c.Assert(err, gocheck.IsNil)
	c.Assert(q["signature"], gocheck.NotNil)
	c.Assert(q["apiKey"], gocheck.NotNil)
	c.Assert(q["atest"], gocheck.NotNil)
	c.Assert(q["response"], gocheck.DeepEquals, []string{"json"})
	c.Assert(q["command"], gocheck.DeepEquals, []string{"commandTest"})
}
