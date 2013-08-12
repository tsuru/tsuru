// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"encoding/json"
	"github.com/globocom/tsuru/cmd"
	"github.com/globocom/tsuru/io"
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
}

func (sshAgentCmd) Info() *cmd.Info {
	return nil
}

func (sshAgentCmd) Run(ctx *cmd.Context, client *cmd.Client) error {
	return nil
}
