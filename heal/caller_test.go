// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package heal

import (
	"fmt"
	"github.com/tsuru/tsuru/log"
	"launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"time"
)

type CallerSuite struct {
	instId string
	token  string
}

var _ = gocheck.Suite(&CallerSuite{})

func (s *CallerSuite) SetUpSuite(c *gocheck.C) {
	log.Init()
}

func (s *CallerSuite) TestHealersFromResource(c *gocheck.C) {
	os.Setenv("TSURU_TOKEN", "token123")
	defer os.Setenv("TSURU_TOKEN", "")
	reqs := []*http.Request{}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqs = append(reqs, r)
		w.Write([]byte(`{"bootstrap":"/bootstrap"}`))
	}))
	defer ts.Close()
	expected := map[string]*healer{
		"bootstrap": {url: fmt.Sprintf("%s/bootstrap", ts.URL)},
	}
	healers, err := healersFromResource(ts.URL)
	c.Assert(err, gocheck.IsNil)
	c.Assert(healers, gocheck.DeepEquals, expected)
	c.Assert(reqs, gocheck.HasLen, 1)
	c.Assert(reqs[0].Header.Get("Authorization"), gocheck.Equals, "bearer token123")
}

func (s *CallerSuite) TestTsuruHealer(c *gocheck.C) {
	os.Setenv("TSURU_TOKEN", "token123")
	defer os.Setenv("TSURU_TOKEN", "")
	var reqs []*http.Request
	var called bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		reqs = append(reqs, r)
	}))
	defer ts.Close()
	h := healer{url: ts.URL}
	err := h.heal()
	c.Assert(err, gocheck.IsNil)
	c.Assert(called, gocheck.Equals, true)
	c.Assert(reqs, gocheck.HasLen, 1)
	c.Assert(reqs[0].Header.Get("Authorization"), gocheck.Equals, "bearer token123")
}

func (s *CallerSuite) TestSetAndGetHealers(c *gocheck.C) {
	h := &healer{url: ""}
	setHealers(map[string]*healer{"test-healer": h})
	healers := getHealers()
	healer, ok := healers["test-healer"]
	c.Assert(healer, gocheck.DeepEquals, h)
	c.Assert(ok, gocheck.Equals, true)
}

func (s *S) TestHealTicker(c *gocheck.C) {
	var called int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.StoreInt32(&called, 1)
	}))
	defer ts.Close()
	h := &healer{url: ts.URL}
	setHealers(map[string]*healer{"ticker-healer": h})
	ch := make(chan time.Time)
	ok := make(chan bool)
	go func() {
		HealTicker(ch)
		ok <- true
	}()
	ch <- time.Now()
	time.Sleep(1 * time.Second)
	close(ch)
	<-ok
	c.Assert(atomic.LoadInt32(&called), gocheck.Equals, int32(1))
}

func (s *S) TestRegisterTicker(c *gocheck.C) {
	var called int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.StoreInt32(&called, 1)
	}))
	defer ts.Close()
	ch := make(chan time.Time)
	ok := make(chan bool)
	go func() {
		RegisterHealerTicker(ch, ts.URL)
		ok <- true
	}()
	ch <- time.Now()
	time.Sleep(1 * time.Second)
	close(ch)
	c.Assert(atomic.LoadInt32(&called), gocheck.Equals, int32(1))
}
