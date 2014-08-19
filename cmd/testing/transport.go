// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package testing

import (
	"bytes"
	"errors"
	"io/ioutil"
	"net/http"
)

type Transport struct {
	Message string
	Status  int
	Headers map[string][]string
}

func (t *Transport) RoundTrip(req *http.Request) (resp *http.Response, err error) {
	resp = &http.Response{
		Body:       ioutil.NopCloser(bytes.NewBufferString(t.Message)),
		StatusCode: t.Status,
		Header:     http.Header(t.Headers),
	}
	return resp, nil
}

type ConditionalTransport struct {
	Transport
	CondFunc func(*http.Request) bool
}

func (t *ConditionalTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if !t.CondFunc(req) {
		return &http.Response{Body: nil, StatusCode: 500}, errors.New("condition failed")
	}
	return t.Transport.RoundTrip(req)
}

type MultiConditionalTransport struct {
	ConditionalTransports []ConditionalTransport
}

func (m *MultiConditionalTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ct := m.ConditionalTransports[0]
	m.ConditionalTransports = m.ConditionalTransports[1:]
	return ct.RoundTrip(req)
}
