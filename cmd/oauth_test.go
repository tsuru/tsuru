// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	etesting "github.com/tsuru/tsuru/exec/testing"
	ftesting "github.com/tsuru/tsuru/fs/testing"
	"io/ioutil"
	"launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
)

func (s *S) TestAuthorizeUrl(c *gocheck.C) {
	c.Assert("", gocheck.Equals, authorizeUrl())
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"name": "oauth", "data": {"authorizeUrl": "http://ble:%s"}}`))
	}))
	defer ts.Close()
	writeTarget(ts.URL)
	c.Assert("http://ble:%s", gocheck.Equals, authorizeUrl())
}

func (s *S) TestPort(c *gocheck.C) {
	c.Assert(":0", gocheck.Equals, port())
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"name": "oauth", "data": {"port": "4242"}}`))
	}))
	defer ts.Close()
	writeTarget(ts.URL)
	c.Assert(":4242", gocheck.Equals, port())
}

func (s *S) TestOpen(c *gocheck.C) {
	fexec := etesting.FakeExecutor{}
	execut = &fexec
	defer func() {
		execut = nil
	}()
	url := "http://someurl"
	err := open(url)
	c.Assert(err, gocheck.IsNil)
	if runtime.GOOS == "linux" {
		c.Assert(fexec.ExecutedCmd("xdg-open", []string{url}), gocheck.Equals, true)
	} else {
		c.Assert(fexec.ExecutedCmd("open", []string{url}), gocheck.Equals, true)
	}
}

func (s *S) TestCallbackHandler(c *gocheck.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"token": "xpto"}`))
	}))
	defer ts.Close()
	rfs := &ftesting.RecordingFs{}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	writeTarget(ts.URL)
	redirectUrl := "someurl"
	finish := make(chan bool, 1)
	handler := callback(redirectUrl, finish)
	body := `{"code":"xpto"}`
	request, err := http.NewRequest("GET", "/", strings.NewReader(body))
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	handler(recorder, request)
	c.Assert(<-finish, gocheck.Equals, true)
	expected := "<html><head><script>window.close();</script></head></html>"
	c.Assert(expected, gocheck.Equals, recorder.Body.String())
	file, err := rfs.Open(JoinWithUserDir(".tsuru_token"))
	c.Assert(err, gocheck.IsNil)
	data, err := ioutil.ReadAll(file)
	c.Assert(err, gocheck.IsNil)
	c.Assert(string(data), gocheck.Equals, "xpto")
}
