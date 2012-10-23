// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"bytes"
	"errors"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"net/http"
	"os"
	"testing"
)

func Test(t *testing.T) { TestingT(t) }

type S struct {
	stdin *os.File
}

type transport struct {
	msg    string
	status int
}

type conditionalTransport struct {
	transport
	condFunc func(*http.Request) bool
}

var _ = Suite(&S{})
var manager *Manager

func (t *transport) RoundTrip(req *http.Request) (resp *http.Response, err error) {
	resp = &http.Response{
		Body:       ioutil.NopCloser(bytes.NewBufferString(t.msg)),
		StatusCode: t.status,
	}
	return resp, nil
}

func (t *conditionalTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if !t.condFunc(req) {
		return &http.Response{Body: nil, StatusCode: 500}, errors.New("condition failed")
	}
	return t.transport.RoundTrip(req)
}

func (s *S) SetUpTest(c *C) {
	var stdout, stderr bytes.Buffer
	manager = NewManager("glb", "1.0", &stdout, &stderr, os.Stdin)
	var exiter recordingExiter
	manager.e = &exiter
}
