// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"

	"github.com/tsuru/tsuru/exec/exectest"
	"github.com/tsuru/tsuru/fs/fstest"
	"gopkg.in/check.v1"
)

func (s *S) TestPort(c *check.C) {
	c.Assert(":0", check.Equals, port(map[string]string{}))
	c.Assert(":4242", check.Equals, port(map[string]string{"port": "4242"}))
}

func (s *S) TestOpen(c *check.C) {
	fexec := exectest.FakeExecutor{}
	execut = &fexec
	defer func() {
		execut = nil
	}()
	url := "http://someurl"
	err := open(url)
	c.Assert(err, check.IsNil)
	if runtime.GOOS == "linux" {
		c.Assert(fexec.ExecutedCmd("xdg-open", []string{url}), check.Equals, true)
	} else {
		c.Assert(fexec.ExecutedCmd("open", []string{url}), check.Equals, true)
	}
}

func (s *S) TestCallbackHandler(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"token": "xpto"}`))
	}))
	defer ts.Close()
	rfs := &fstest.RecordingFs{}
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
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	handler(recorder, request)
	c.Assert(<-finish, check.Equals, true)
	expectedPage := fmt.Sprintf(callbackPage, successMarkup)
	c.Assert(expectedPage, check.Equals, recorder.Body.String())
	file, err := rfs.Open(JoinWithUserDir(".tsuru_token"))
	c.Assert(err, check.IsNil)
	data, err := ioutil.ReadAll(file)
	c.Assert(err, check.IsNil)
	c.Assert(string(data), check.Equals, "xpto")
}
