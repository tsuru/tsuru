// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"bytes"
	"net/http"
	"strings"

	ttesting "github.com/tsuru/tsuru/cmd/testing"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/fs/testing"
	"launchpad.net/gocheck"
)

func (s *S) TestShouldSetCloseToTrue(c *gocheck.C) {
	request, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, gocheck.IsNil)
	transport := ttesting.Transport{
		Status:  http.StatusOK,
		Message: "OK",
	}
	var buf bytes.Buffer
	context := Context{
		Stdout: &buf,
	}
	client := NewClient(&http.Client{Transport: &transport}, &context, manager)
	client.Verbosity = 2
	client.Do(request)
	c.Assert(request.Close, gocheck.Equals, true)
	c.Assert(strings.Replace(buf.String(), "\n", "\\n", -1), gocheck.Matches,
		``+
			`.*<Request uri="/">.*`+
			`GET / HTTP/1.1\r\\n.*`+
			`Connection: close.*`+
			`Authorization: bearer.*`+
			`<Response uri="/">.*`+
			`HTTP/0.0 200 OK.*`)
}

func (s *S) TestShouldReturnBodyMessageOnError(c *gocheck.C) {
	request, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, gocheck.IsNil)
	var buf bytes.Buffer
	context := Context{
		Stdout: &buf,
	}
	client := NewClient(
		&http.Client{Transport: &ttesting.Transport{Message: "You must be authenticated to execute this command.", Status: http.StatusUnauthorized}},
		&context,
		manager)
	client.Verbosity = 2
	response, err := client.Do(request)
	c.Assert(response, gocheck.NotNil)
	c.Assert(err, gocheck.NotNil)
	expectedMsg := "You must be authenticated to execute this command."
	c.Assert(err.Error(), gocheck.Equals, expectedMsg)
	httpErr, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(httpErr.Code, gocheck.Equals, http.StatusUnauthorized)
	c.Assert(httpErr.Message, gocheck.Equals, expectedMsg)
	c.Assert(strings.Replace(buf.String(), "\n", "\\n", -1), gocheck.Matches,
		``+
			`.*<Request uri="/">.*`+
			`GET / HTTP/1.1\r\\n.*`+
			`Connection: close.*`+
			`Authorization: bearer.*`+
			`<Response uri="/">.*`+
			`HTTP/0.0 401 Unauthorized.*`+
			`You must be authenticated to execute this command\..*`)
}

func (s *S) TestShouldReturnErrorWhenServerIsDown(c *gocheck.C) {
	rfs := &testing.RecordingFs{FileContent: "http://tsuru.google.com"}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	request, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, gocheck.IsNil)
	var buf bytes.Buffer
	context := Context{
		Stdout: &buf,
	}
	client := NewClient(&http.Client{}, &context, manager)
	client.Verbosity = 2
	_, err = client.Do(request)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "Failed to connect to tsuru server (http://tsuru.google.com), it's probably down.")
	c.Assert(strings.Replace(buf.String(), "\n", "\\n", -1), gocheck.Matches,
		``+
			`.*<Request uri="/">.*`+
			`GET / HTTP/1.1\r\\n.*`+
			`Connection: close.*`+
			`Authorization: bearer.*`)
}

func (s *S) TestShouldNotIncludeTheHeaderAuthorizationWhenTheTsuruTokenFileIsMissing(c *gocheck.C) {
	fsystem = &testing.FileNotFoundFs{}
	defer func() {
		fsystem = nil
	}()
	request, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, gocheck.IsNil)
	trans := ttesting.Transport{Message: "", Status: http.StatusOK}
	var buf bytes.Buffer
	context := Context{
		Stdout: &buf,
	}
	client := NewClient(&http.Client{Transport: &trans}, &context, manager)
	client.Verbosity = 2
	_, err = client.Do(request)
	c.Assert(err, gocheck.IsNil)
	header := map[string][]string(request.Header)
	_, ok := header["Authorization"]
	c.Assert(ok, gocheck.Equals, false)
	c.Assert(strings.Replace(buf.String(), "\n", "\\n", -1), gocheck.Matches,
		``+
			`.*<Request uri="/">.*`+
			`GET / HTTP/1.1\r\\n.*`+
			`Connection: close.*`)
}

func (s *S) TestShouldIncludeTheHeaderAuthorizationWhenTsuruTokenFileExists(c *gocheck.C) {
	fsystem = &testing.RecordingFs{FileContent: "mytoken"}
	defer func() {
		fsystem = nil
	}()
	request, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, gocheck.IsNil)
	trans := ttesting.Transport{Message: "", Status: http.StatusOK}
	var buf bytes.Buffer
	context := Context{
		Stdout: &buf,
	}
	client := NewClient(&http.Client{Transport: &trans}, &context, manager)
	client.Verbosity = 2
	_, err = client.Do(request)
	c.Assert(err, gocheck.IsNil)
	c.Assert(request.Header.Get("Authorization"), gocheck.Equals, "bearer mytoken")
	c.Assert(strings.Replace(buf.String(), "\n", "\\n", -1), gocheck.Matches,
		``+
			`.*<Request uri="/">.*`+
			`GET / HTTP/1.1\r\\n.*`+
			`Connection: close.*`+
			`Authorization: bearer.*`)
}

func (s *S) TestShouldValidateVersion(c *gocheck.C) {
	var buf bytes.Buffer
	request, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, gocheck.IsNil)
	context := Context{
		Stderr: &buf,
	}
	trans := ttesting.Transport{
		Message: "",
		Status:  http.StatusOK,
		Headers: map[string][]string{"Supported-Tsuru": {"0.3"}},
	}
	manager := Manager{
		name:          "glb",
		version:       "0.2.1",
		versionHeader: "Supported-Tsuru",
	}
	client := NewClient(&http.Client{Transport: &trans}, &context, &manager)
	_, err = client.Do(request)
	c.Assert(err, gocheck.IsNil)
	expected := `#####################################################################

WARNING: You're using an unsupported version of glb.

You must have at least version 0.3, your current
version is 0.2.1.

Please go to http://docs.tsuru.io/en/latest/using/install-client.html
and download the last version.

#####################################################################

`
	c.Assert(buf.String(), gocheck.Equals, expected)
}

func (s *S) TestShouldSkipValidationIfThereIsNoSupportedHeaderDeclared(c *gocheck.C) {
	var buf bytes.Buffer
	request, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, gocheck.IsNil)
	context := Context{
		Stderr: &buf,
	}
	trans := ttesting.Transport{Message: "", Status: http.StatusOK, Headers: map[string][]string{"Supported-Tsuru": {"0.3"}}}
	manager := Manager{
		version: "0.2.1",
	}
	client := NewClient(&http.Client{Transport: &trans}, &context, &manager)
	_, err = client.Do(request)
	c.Assert(err, gocheck.IsNil)
	c.Assert(buf.String(), gocheck.Equals, "")
}

func (s *S) TestShouldSkupValidationIfServerDoesNotReturnSupportedHeader(c *gocheck.C) {
	var buf bytes.Buffer
	request, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, gocheck.IsNil)
	context := Context{
		Stderr: &buf,
	}
	trans := ttesting.Transport{Message: "", Status: http.StatusOK}
	manager := Manager{
		name:          "glb",
		version:       "0.2.1",
		versionHeader: "Supported-Tsuru",
	}
	client := NewClient(&http.Client{Transport: &trans}, &context, &manager)
	_, err = client.Do(request)
	c.Assert(err, gocheck.IsNil)
	c.Assert(buf.String(), gocheck.Equals, "")
}
