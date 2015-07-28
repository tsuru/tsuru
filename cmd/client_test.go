// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/tsuru/tsuru/cmd/cmdtest"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/fs/fstest"
	"gopkg.in/check.v1"
)

func (s *S) TestShouldSetCloseToTrue(c *check.C) {
	request, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, check.IsNil)
	transport := cmdtest.Transport{
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
	c.Assert(request.Close, check.Equals, true)
	c.Assert(buf.String(), check.Matches,
		`(?s)`+
			`.*<Request uri="/">.*`+
			`GET / HTTP/1.1\r\n.*`+
			`Connection: close.*`+
			`Authorization: bearer.*`+
			`<Response uri="/">.*`+
			`HTTP/0.0 200 OK.*`)
}

func (s *S) TestShouldReturnBodyMessageOnError(c *check.C) {
	request, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, check.IsNil)
	var buf bytes.Buffer
	context := Context{
		Stdout: &buf,
	}
	client := NewClient(
		&http.Client{Transport: &cmdtest.Transport{Message: "You can't do this", Status: http.StatusForbidden}},
		&context,
		manager)
	client.Verbosity = 2
	response, err := client.Do(request)
	c.Assert(response, check.NotNil)
	c.Assert(err, check.NotNil)
	expectedMsg := "You can't do this"
	c.Assert(err.Error(), check.Equals, expectedMsg)
	httpErr, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(httpErr.Code, check.Equals, http.StatusForbidden)
	c.Assert(httpErr.Message, check.Equals, expectedMsg)
	c.Assert(buf.String(), check.Matches,
		`(?s)`+
			`.*<Request uri="/">.*`+
			`GET / HTTP/1.1\r\n.*`+
			`Connection: close.*`+
			`Authorization: bearer.*`+
			`<Response uri="/">.*`+
			`HTTP/0.0 403 Forbidden.*`+
			`You can't do this.*`)
}

func (s *S) TestShouldReturnStatusMessageOnErrorWhenBodyIsEmpty(c *check.C) {
	request, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, check.IsNil)
	var buf bytes.Buffer
	context := Context{
		Stdout: &buf,
	}
	client := NewClient(
		&http.Client{
			Transport: &cmdtest.Transport{
				Message: "",
				Status:  http.StatusServiceUnavailable,
			},
		},
		&context,
		manager)
	client.Verbosity = 2
	response, err := client.Do(request)
	c.Assert(response, check.NotNil)
	c.Assert(err, check.NotNil)
	expectedMsg := "503 Service Unavailable"
	c.Assert(err.Error(), check.Equals, expectedMsg)
	httpErr, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(httpErr.Code, check.Equals, http.StatusServiceUnavailable)
	c.Assert(httpErr.Message, check.Equals, expectedMsg)
	c.Assert(buf.String(), check.Matches,
		`(?s)`+
			`.*<Request uri="/">.*`+
			`GET / HTTP/1.1\r\n.*`+
			`Connection: close.*`+
			`Authorization: bearer.*`+
			`<Response uri="/">.*`+
			`HTTP/0.0 503 Service Unavailable\r\n`+
			`Content-Length: 0\r\n`+
			`\r\n`+
			`\*+ </Response uri="/">.*`)
}

func (s *S) TestShouldHandleUnauthorizedErrorSpecially(c *check.C) {
	request, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, check.IsNil)
	var buf bytes.Buffer
	context := Context{
		Stdout: &buf,
	}
	client := NewClient(
		&http.Client{Transport: &cmdtest.Transport{Message: "You can't do this", Status: http.StatusUnauthorized}},
		&context,
		manager)
	response, err := client.Do(request)
	c.Assert(response, check.NotNil)
	c.Assert(err, check.Equals, errUnauthorized)
}

func (s *S) TestShouldReturnErrorWhenServerIsDown(c *check.C) {
	rfs := &fstest.RecordingFs{FileContent: "http://tsuru.google.com"}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	request, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, check.IsNil)
	var buf bytes.Buffer
	context := Context{
		Stdout: &buf,
	}
	client := NewClient(&http.Client{}, &context, manager)
	client.Verbosity = 2
	_, err = client.Do(request)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Failed to connect to tsuru server (http://tsuru.google.com), it's probably down.")
	c.Assert(strings.Replace(buf.String(), "\n", "\\n", -1), check.Matches,
		``+
			`.*<Request uri="/">.*`+
			`GET / HTTP/1.1\r\\n.*`+
			`Connection: close.*`+
			`Authorization: bearer.*`)
}

func (s *S) TestShouldNotIncludeTheHeaderAuthorizationWhenTheTsuruTokenFileIsMissing(c *check.C) {
	fsystem = &fstest.FileNotFoundFs{}
	defer func() {
		fsystem = nil
	}()
	request, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, check.IsNil)
	trans := cmdtest.Transport{Message: "", Status: http.StatusOK}
	var buf bytes.Buffer
	context := Context{
		Stdout: &buf,
	}
	client := NewClient(&http.Client{Transport: &trans}, &context, manager)
	client.Verbosity = 2
	_, err = client.Do(request)
	c.Assert(err, check.IsNil)
	header := map[string][]string(request.Header)
	_, ok := header["Authorization"]
	c.Assert(ok, check.Equals, false)
	c.Assert(strings.Replace(buf.String(), "\n", "\\n", -1), check.Matches,
		``+
			`.*<Request uri="/">.*`+
			`GET / HTTP/1.1\r\\n.*`+
			`Connection: close.*`)
}

func (s *S) TestShouldIncludeTheHeaderAuthorizationWhenTsuruTokenFileExists(c *check.C) {
	fsystem = &fstest.RecordingFs{FileContent: "mytoken"}
	defer func() {
		fsystem = nil
	}()
	request, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, check.IsNil)
	trans := cmdtest.Transport{Message: "", Status: http.StatusOK}
	var buf bytes.Buffer
	context := Context{
		Stdout: &buf,
	}
	client := NewClient(&http.Client{Transport: &trans}, &context, manager)
	client.Verbosity = 2
	_, err = client.Do(request)
	c.Assert(err, check.IsNil)
	c.Assert(request.Header.Get("Authorization"), check.Equals, "bearer mytoken")
	c.Assert(strings.Replace(buf.String(), "\n", "\\n", -1), check.Matches,
		``+
			`.*<Request uri="/">.*`+
			`GET / HTTP/1.1\r\\n.*`+
			`Connection: close.*`+
			`Authorization: bearer.*`)
}

func (s *S) TestShouldValidateVersion(c *check.C) {
	var buf bytes.Buffer
	request, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, check.IsNil)
	context := Context{
		Stderr: &buf,
	}
	trans := cmdtest.Transport{
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
	c.Assert(err, check.IsNil)
	expected := `#####################################################################

WARNING: You're using an unsupported version of glb.

You must have at least version 0.3, your current
version is 0.2.1.

Please go to http://docs.tsuru.io/en/latest/using/install-client.html
and download the last version.

#####################################################################

`
	c.Assert(buf.String(), check.Equals, expected)
}

func (s *S) TestShouldSkipValidationIfThereIsNoSupportedHeaderDeclared(c *check.C) {
	var buf bytes.Buffer
	request, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, check.IsNil)
	context := Context{
		Stderr: &buf,
	}
	trans := cmdtest.Transport{Message: "", Status: http.StatusOK, Headers: map[string][]string{"Supported-Tsuru": {"0.3"}}}
	manager := Manager{
		version: "0.2.1",
	}
	client := NewClient(&http.Client{Transport: &trans}, &context, &manager)
	_, err = client.Do(request)
	c.Assert(err, check.IsNil)
	c.Assert(buf.String(), check.Equals, "")
}

func (s *S) TestShouldSkupValidationIfServerDoesNotReturnSupportedHeader(c *check.C) {
	var buf bytes.Buffer
	request, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, check.IsNil)
	context := Context{
		Stderr: &buf,
	}
	trans := cmdtest.Transport{Message: "", Status: http.StatusOK}
	manager := Manager{
		name:          "glb",
		version:       "0.2.1",
		versionHeader: "Supported-Tsuru",
	}
	client := NewClient(&http.Client{Transport: &trans}, &context, &manager)
	_, err = client.Do(request)
	c.Assert(err, check.IsNil)
	c.Assert(buf.String(), check.Equals, "")
}

func (s *S) TestStreamJSONResponse(c *check.C) {
	reader := bytes.NewBufferString(`{"message":"hello!"}`)
	var resp http.Response
	resp.Body = ioutil.NopCloser(reader)
	var buf bytes.Buffer
	err := StreamJSONResponse(&buf, &resp)
	c.Assert(err, check.IsNil)
	c.Assert(buf.String(), check.Equals, "hello!")
}
