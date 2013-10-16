// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"encoding/json"
	"github.com/bmizerany/pat"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/cmd"
	"github.com/globocom/tsuru/io"
	"launchpad.net/gnuflag"
	"net"
	"net/http"
)

type cmdInput struct {
	Cmd  string
	Args []string
}

type sshHandler struct {
	user string
	pkey string
}

func (h *sshHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text")
	var input cmdInput
	defer r.Body.Close()
	err := json.NewDecoder(r.Body).Decode(&input)
	if err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	w = &io.FlushingWriter{ResponseWriter: w}
	sshArgs := []string{
		r.URL.Query().Get(":ip"),
		"-l", h.user, "-i", h.pkey, "-q",
		"-o", "StrictHostKeyChecking no",
		"--", input.Cmd,
	}
	sshArgs = append(sshArgs, input.Args...)
	executor().Execute("ssh", sshArgs, nil, w, w)
}

func removeHostHandler(w http.ResponseWriter, r *http.Request) {
	w = &io.FlushingWriter{ResponseWriter: w}
	executor().Execute("ssh-keygen", []string{"-R", r.URL.Query().Get(":ip")}, nil, w, w)
}

type sshAgentCmd struct {
	listen string
}

func (*sshAgentCmd) Info() *cmd.Info {
	desc := `Start HTTP agent for running commands on Docker via SSH.

By default, the agent will listen on 0.0.0.0:4545. Use --listen or -l to
specify the address to listen on.
`
	return &cmd.Info{
		Name:  "docker-ssh-agent",
		Usage: "docker-ssh-agent",
		Desc:  desc,
	}
}

func (cmd *sshAgentCmd) Run(ctx *cmd.Context, client *cmd.Client) error {
	var handler sshHandler
	m := pat.New()
	m.Post("/container/:ip/cmd", &handler)
	m.Del("/container/:ip", http.HandlerFunc(removeHostHandler))
	listener, err := net.Listen("tcp", cmd.listen)
	if err != nil {
		return err
	}
	defer listener.Close()
	return http.Serve(listener, m)
}

func (cmd *sshAgentCmd) Flags() *gnuflag.FlagSet {
	flags := gnuflag.NewFlagSet("docker-ssh-agent", gnuflag.ExitOnError)
	flags.StringVar(&cmd.listen, "listen", "0.0.0.0:4545", "Address to listen on")
	flags.StringVar(&cmd.listen, "l", "0.0.0.0:4545", "Address to listen on")
	return flags
}

func sshAgentPort() int {
	port, _ := config.GetInt("docker:ssh-agent-port")
	if port == 0 {
		port = 4545
	}
	return port
}
