// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"bytes"
	"github.com/globocom/tsuru/fs/testing"
	. "launchpad.net/gocheck"
	"net/http"
)

func (s *S) TestShouldReturnBodyMessageOnError(c *C) {
	request, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, IsNil)

	client := NewClient(&http.Client{Transport: &transport{msg: "You must be authenticated to execute this command.", status: http.StatusUnauthorized}}, nil, manager)
	response, err := client.Do(request)
	c.Assert(response, IsNil)
	c.Assert(err.Error(), Equals, "You must be authenticated to execute this command.")
}

func (s *S) TestShouldReturnErrorWhenServerIsDown(c *C) {
	rfs := &testing.RecordingFs{FileContent: "http://tsuru.google.com"}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	request, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, IsNil)
	client := NewClient(&http.Client{}, nil, manager)
	_, err = client.Do(request)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "Failed to connect to tsuru server (http://tsuru.google.com), it's probably down.")
}

func (s *S) TestShouldNotIncludeTheHeaderAuthorizationWhenTheTsuruTokenFileIsMissing(c *C) {
	fsystem = &testing.FailureFs{}
	defer func() {
		fsystem = nil
	}()
	request, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, IsNil)
	trans := transport{msg: "", status: http.StatusOK}
	client := NewClient(&http.Client{Transport: &trans}, nil, manager)
	_, err = client.Do(request)
	c.Assert(err, IsNil)
	header := map[string][]string(request.Header)
	_, ok := header["Authorization"]
	c.Assert(ok, Equals, false)
}

func (s *S) TestShouldIncludeTheHeaderAuthorizationWhenTsuruTokenFileExists(c *C) {
	fsystem = &testing.RecordingFs{FileContent: "mytoken"}
	defer func() {
		fsystem = nil
	}()
	request, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, IsNil)
	trans := transport{msg: "", status: http.StatusOK}
	client := NewClient(&http.Client{Transport: &trans}, nil, manager)
	_, err = client.Do(request)
	c.Assert(err, IsNil)
	c.Assert(request.Header.Get("Authorization"), Equals, "mytoken")
}

func (s *S) TestShouldValidateVersion(c *C) {
	var buf bytes.Buffer
	request, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, IsNil)
	context := Context{
		Stderr: &buf,
	}
	trans := transport{msg: "", status: http.StatusOK, headers: map[string][]string{"Supported-Tsuru": {"0.3"}}}
	manager := Manager{
		name:          "glb",
		version:       "0.2.1",
		versionHeader: "Supported-Tsuru",
	}
	client := NewClient(&http.Client{Transport: &trans}, &context, &manager)
	_, err = client.Do(request)
	c.Assert(err, IsNil)
	expected := `############################################################

WARNING: You're using an unsupported version of glb.

You must have at least version 0.3, your current
version is 0.2.1.

Please go to https://github.com/globocom/tsuru/downloads
and download the last version.

############################################################

`
	c.Assert(buf.String(), Equals, expected)
}

func (s *S) TestShouldSkipValidationIfThereIsNoSupportedHeaderDeclared(c *C) {
	var buf bytes.Buffer
	request, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, IsNil)
	context := Context{
		Stderr: &buf,
	}
	trans := transport{msg: "", status: http.StatusOK, headers: map[string][]string{"Supported-Tsuru": {"0.3"}}}
	manager := Manager{
		version: "0.2.1",
	}
	client := NewClient(&http.Client{Transport: &trans}, &context, &manager)
	_, err = client.Do(request)
	c.Assert(err, IsNil)
	c.Assert(buf.String(), Equals, "")
}

func (s *S) TestShouldSkupValidationIfServerDoesNotReturnSupportedHeader(c *C) {
	var buf bytes.Buffer
	request, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, IsNil)
	context := Context{
		Stderr: &buf,
	}
	trans := transport{msg: "", status: http.StatusOK}
	manager := Manager{
		name:          "glb",
		version:       "0.2.1",
		versionHeader: "Supported-Tsuru",
	}
	client := NewClient(&http.Client{Transport: &trans}, &context, &manager)
	_, err = client.Do(request)
	c.Assert(err, IsNil)
	c.Assert(buf.String(), Equals, "")
}
