// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package exec/testing provides fake implementations of the exec package.
//
// These implementations can be used to mock out the Executor in tests.
package testing

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"sync"
)

type command struct {
	name string
	args []string
}

func (c *command) GetName() string {
	return c.name
}

func (c *command) GetArgs() []string {
	return c.args
}

type FakeExecutor struct {
	cmds   []command
	mut    sync.RWMutex
	Output map[string][]byte
}

func (e *FakeExecutor) hasOutputForArgs(args []string) (bool, []byte) {
	var generic []byte
	sArgs := strings.Join(args, " ")
	for k, v := range e.Output {
		switch k {
		case sArgs:
			return true, v
		case "*":
			generic = v
		}
	}
	if generic != nil {
		return true, generic
	}
	return false, nil
}

func (e *FakeExecutor) Execute(cmd string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	c := command{name: cmd, args: args}
	e.mut.Lock()
	e.cmds = append(e.cmds, c)
	e.mut.Unlock()
	has, out := e.hasOutputForArgs(args)
	if has {
		stdout.Write(out)
	}
	return nil
}

func (e *FakeExecutor) ExecutedCmd(cmd string, args []string) bool {
	e.mut.RLock()
	defer e.mut.RUnlock()
	for _, c := range e.cmds {
		if c.name == cmd && reflect.DeepEqual(c.args, args) {
			return true
		}
	}
	return false
}

func (e *FakeExecutor) GetCommands(cmdName string) []command {
	var cmds []command
	for _, cmd := range e.cmds {
		if cmd.name == cmdName {
			cmds = append(cmds, cmd)
		}
	}
	return cmds
}

type ErrorExecutor struct {
	FakeExecutor
}

func (e *ErrorExecutor) Execute(cmd string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	e.FakeExecutor.Execute(cmd, args, stdin, stderr, stderr)
	return errors.New("")
}

func (e *ErrorExecutor) ExecutedCmd(cmd string, args []string) bool {
	return e.FakeExecutor.ExecutedCmd(cmd, args)
}
