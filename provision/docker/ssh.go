// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"encoding/json"
	"github.com/bmizerany/pat"
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

func sshHandler(w http.ResponseWriter, r *http.Request) {
	var input cmdInput
	defer r.Body.Close()
	err := json.NewDecoder(r.Body).Decode(&input)
	if err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	w = &io.FlushingWriter{ResponseWriter: w}
	sshArgs := []string{r.URL.Query().Get(":ip"), "-l", "ubuntu", "-o", "StrictHostKeyChecking no", "--", input.Cmd}
	sshArgs = append(sshArgs, input.Args...)
	executor().Execute("ssh", sshArgs, nil, w, w)
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
	m := pat.New()
	m.Post("/container/:ip/cmd", http.HandlerFunc(sshHandler))
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
