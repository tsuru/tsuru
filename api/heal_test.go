// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"fmt"
	"github.com/tsuru/tsuru/heal"
	"io/ioutil"
	"launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
)

type HealerSuite struct{}

var _ = gocheck.Suite(&HealerSuite{})

type FakeHealer struct {
	called bool
}

// FakeHealer always needs heal.
func (h *FakeHealer) NeedsHeal() bool {
	return true
}

func (h *FakeHealer) Heal() error {
	h.called = true
	return nil
}

func (s *HealerSuite) TestHealers(c *gocheck.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/healers", nil)
	c.Assert(err, gocheck.IsNil)
	err = healers(recorder, request, nil)
	c.Assert(err, gocheck.IsNil)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusOK)
	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, gocheck.IsNil)
	h := map[string]string{}
	err = json.Unmarshal(body, &h)
	c.Assert(err, gocheck.IsNil)
	expected := map[string]string{}
	p, _ := getProvisioner()
	for healer := range heal.All(p) {
		expected[healer] = fmt.Sprintf("/healers/%s", healer)
	}
	c.Assert(h, gocheck.DeepEquals, expected)
}

func (s *HealerSuite) TestHealer(c *gocheck.C) {
	fake := &FakeHealer{}
	p, _ := getProvisioner()
	heal.Register(p, "fake", fake)
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/healers/fake?:healer=fake", nil)
	c.Assert(err, gocheck.IsNil)
	err = healer(recorder, request, nil)
	c.Assert(err, gocheck.IsNil)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusOK)
	c.Assert(fake.called, gocheck.Equals, true)
}
