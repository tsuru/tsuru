// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"bytes"
	"errors"
	"io/ioutil"
	"launchpad.net/gocheck"
	"net/http"
	"os"
	"os/exec"
	"testing"
)

func Test(t *testing.T) { gocheck.TestingT(t) }

type S struct {
	stdin   *os.File
	recover []string
}

type transport struct {
	msg     string
	status  int
	headers map[string][]string
}

type conditionalTransport struct {
	transport
	condFunc func(*http.Request) bool
}

var _ = gocheck.Suite(&S{})
var manager *Manager

func (t *transport) RoundTrip(req *http.Request) (resp *http.Response, err error) {
	resp = &http.Response{
		Body:       ioutil.NopCloser(bytes.NewBufferString(t.msg)),
		StatusCode: t.status,
		Header:     http.Header(t.headers),
	}
	return resp, nil
}

func (t *conditionalTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if !t.condFunc(req) {
		return &http.Response{Body: nil, StatusCode: 500}, errors.New("condition failed")
	}
	return t.transport.RoundTrip(req)
}

func (s *S) SetUpSuite(c *gocheck.C) {
	targetFile := os.Getenv("HOME") + "/.tsuru_target"
	_, err := os.Stat(targetFile)
	if err == nil {
		old := targetFile + ".old"
		s.recover = []string{"mv", old, targetFile}
		exec.Command("mv", targetFile, old).Run()
	} else {
		s.recover = []string{"rm", targetFile}
	}
	f, err := os.Create(targetFile)
	c.Assert(err, gocheck.IsNil)
	f.Write([]byte("http://localhost"))
	f.Close()
}

func (s *S) TearDownSuite(c *gocheck.C) {
	exec.Command(s.recover[0], s.recover[1:]...).Run()
}

func (s *S) SetUpTest(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	manager = NewManager("glb", "1.0", "", &stdout, &stderr, os.Stdin)
	var exiter recordingExiter
	manager.e = &exiter
}
