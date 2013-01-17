// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
)

type HealerSuite struct{}

var _ = Suite(&HealerSuite{})

func (s *HealerSuite) TestHealers(c *C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/healers", nil)
	c.Assert(err, IsNil)
	err = healers(recorder, request)
	c.Assert(err, IsNil)
	c.Assert(recorder.Code, Equals, http.StatusOK)
	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, IsNil)
	h := map[string]string{}
	err = json.Unmarshal(body, &h)
	c.Assert(err, IsNil)
	expected := map[string]string{
		"bootstrap": "/healers/bootstrap",
	}
	c.Assert(h, DeepEquals, expected)
}
