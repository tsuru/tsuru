// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package testing

import (
	esource "github.com/antage/eventsource/http"
	"launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
)

func (s *S) TestFakeEventSourceServeHTTP(c *gocheck.C) {
	fake := NewFakeEventSource()
	req, _ := http.NewRequest("GET", "/", nil)
	recorder := httptest.NewRecorder()
	fake.ServeHTTP(recorder, req)
	c.Assert(fake.requests, gocheck.DeepEquals, []*http.Request{req})
}

func (s *S) TestFakeEventSourceSendMessage(c *gocheck.C) {
	fake := NewFakeEventSource()
	fake.SendMessage("tick-tack", "tick", "1")
	fake.SendMessage("tick-tack", "tick", "2")
	fake.SendMessage("tick-tack", "tick", "3")
	want := []Event{
		{data: "tick-tack", event: "tick", id: "1"},
		{data: "tick-tack", event: "tick", id: "2"},
		{data: "tick-tack", event: "tick", id: "3"},
	}
	c.Assert(fake.events, gocheck.DeepEquals, want)
}

func (s *S) TestFakeEventSourceConsumersCount(c *gocheck.C) {
	fake := NewFakeEventSource()
	c.Assert(fake.ConsumersCount(), gocheck.Equals, 0)
}

func (s *S) TestFakeEventSourceClose(c *gocheck.C) {
	fake := NewFakeEventSource()
	fake.SendMessage("tick-tack", "tick", "1")
	fake.SendMessage("tick-tack", "tick", "2")
	fake.SendMessage("tick-tack", "tick", "3")
	req, _ := http.NewRequest("GET", "/", nil)
	recorder := httptest.NewRecorder()
	fake.ServeHTTP(recorder, req)
	fake.Close()
	c.Assert(fake.events, gocheck.IsNil)
	c.Assert(fake.requests, gocheck.IsNil)
}

func (s *S) TestFakeEventSourceSatisfiesTheInterface(c *gocheck.C) {
	var _ esource.EventSource = &FakeEventSource{}
}
