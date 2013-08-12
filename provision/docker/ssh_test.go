// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"github.com/globocom/tsuru/exec/testing"
	"launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
	"strings"
)

var _ = gocheck.Suite(SSHSuite{})

type SSHSuite struct{}

func (SSHSuite) TestExecuteCommandHandler(c *gocheck.C) {
	output := []byte(". ..")
	out := map[string][][]byte{"*": {output}}
	fexec := &testing.FakeExecutor{Output: out}
	setExecut(fexec)
	defer setExecut(nil)
	data := `{"cmd":"ls","args":["-l", "-a"]}`
	body := strings.NewReader(data)
	request, _ := http.NewRequest("POST", "/container/10.10.10.10/cmd?:ip=10.10.10.10", body)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	sshHandler(recorder, request)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusOK)
	c.Assert(recorder.Body.String(), gocheck.Equals, string(output))
	args := []string{
		"10.10.10.10", "-l", "ubuntu",
		"-o", "StrictHostKeyChecking no",
		"--", "ls", "-l", "-a",
	}
	c.Assert(fexec.ExecutedCmd("ssh", args), gocheck.Equals, true)
}

func (SSHSuite) TestExecuteCommandHandlerInvalidJSON(c *gocheck.C) {
	data := `}}}}---;"`
	body := strings.NewReader(data)
	request, _ := http.NewRequest("POST", "/container/10.10.10.10/cmd?:ip=10.10.10.10", body)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	sshHandler(recorder, request)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), gocheck.Equals, "Invalid JSON\n")
}
