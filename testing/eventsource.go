// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package testing

import (
	"net/http"
)

type Event struct {
	id    string
	data  string
	event string
}

type FakeEventSource struct {
	events   []Event
	requests []*http.Request
}

func NewFakeEventSource() *FakeEventSource {
	return &FakeEventSource{}
}

func (e *FakeEventSource) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	e.requests = append(e.requests, r)
}

func (e *FakeEventSource) SendMessage(data, event, id string) {
	e.events = append(e.events, Event{id, data, event})
}

func (e *FakeEventSource) ConsumersCount() int {
	return 0
}

func (e *FakeEventSource) Close() {
	e.events = nil
	e.requests = nil
}
