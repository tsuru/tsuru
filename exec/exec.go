// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package exec provides a interface to run external commans as an
// abstraction layer.
package exec

import (
	"io"
	"os/exec"
)

// ExecuteOptions specify parameters to the Execute method.
type ExecuteOptions struct {
	Cmd    string
	Args   []string
	Envs   []string
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

type Executor interface {
	// Execute executes the specified command.
	Execute(opts ExecuteOptions) error
}

type OsExecutor struct{}

func (OsExecutor) Execute(opts ExecuteOptions) error {
	c := exec.Command(opts.Cmd, opts.Args...)
	c.Stdin = opts.Stdin
	c.Stdout = opts.Stdout
	c.Stderr = opts.Stderr
	c.Env = opts.Envs
	return c.Run()
}
