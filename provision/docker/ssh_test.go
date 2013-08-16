// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"encoding/json"
	"github.com/globocom/tsuru/cmd"
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
	c.Assert(recorder.Header().Get("Content-Type"), gocheck.Equals, "text")
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

func (SSHSuite) TestRemoveHostHandler(c *gocheck.C) {
	output := []byte(". ..")
	out := map[string][][]byte{"*": {output}}
	fexec := &testing.FakeExecutor{Output: out}
	setExecut(fexec)
	defer setExecut(nil)
	request, _ := http.NewRequest("DELETE", "/container/10.10.10.10?:ip=10.10.10.10", nil)
	recorder := httptest.NewRecorder()
	removeHostHandler(recorder, request)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusOK)
	args := []string{"-R", "10.10.10.10"}
	c.Assert(fexec.ExecutedCmd("ssh-keygen", args), gocheck.Equals, true)
	c.Assert(recorder.Body.String(), gocheck.Equals, ". ..")
}

func (SSHSuite) TestSSHAgentCmdInfo(c *gocheck.C) {
	desc := `Start HTTP agent for running commands on Docker via SSH.

By default, the agent will listen on 0.0.0.0:4545. Use --listen or -l to
specify the address to listen on.
`
	expected := &cmd.Info{
		Name:  "docker-ssh-agent",
		Usage: "docker-ssh-agent",
		Desc:  desc,
	}
	c.Assert((&sshAgentCmd{}).Info(), gocheck.DeepEquals, expected)
}

func (SSHSuite) TestSSHAgentCmdFlags(c *gocheck.C) {
	cmd := sshAgentCmd{}
	flags := cmd.Flags()
	flags.Parse(true, []string{})
	flag := flags.Lookup("l")
	c.Check(flag.Name, gocheck.Equals, "l")
	c.Check(flag.DefValue, gocheck.Equals, "0.0.0.0:4545")
	c.Check(flag.Usage, gocheck.Equals, "Address to listen on")
	flag = flags.Lookup("listen")
	c.Check(flag.Name, gocheck.Equals, "listen")
	c.Check(flag.DefValue, gocheck.Equals, "0.0.0.0:4545")
	c.Check(flag.Usage, gocheck.Equals, "Address to listen on")
	c.Check(cmd.listen, gocheck.Equals, "0.0.0.0:4545")
}

type FakeSSHServer struct {
	requests []*http.Request
	bodies   []cmdInput
	output   string
}

func (h *FakeSSHServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var input cmdInput
	defer r.Body.Close()
	err := json.NewDecoder(r.Body).Decode(&input)
	if err != nil {
		panic(err)
	}
	h.requests = append(h.requests, r)
	h.bodies = append(h.bodies, input)
	w.Write([]byte(h.output))
}
