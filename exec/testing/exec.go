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

// hasOutputForArgs checks if a slice of args have a specific outout assigned to it
// and returns a boolean indicating that, if the args does have an assigned output,
// returns it.
func hasOutputForArgs(out map[string][]byte, args []string) (bool, []byte) {
	sArgs := strings.Join(args, " ")
	for k, v := range out {
		if k == sArgs || k == "*" {
			return true, v
		}
	}
	return false, []byte{}
}

func (e *FakeExecutor) hasOutputForArgs(args []string) (bool, []byte) {
	return hasOutputForArgs(e.Output, args)
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
	cmds   []command
	mut    sync.RWMutex
	Output map[string][]byte
}

func (e *ErrorExecutor) hasOutputForArgs(args []string) (bool, []byte) {
	return hasOutputForArgs(e.Output, args)
}

func (e *ErrorExecutor) Execute(cmd string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	c := command{name: cmd, args: args}
	e.mut.Lock()
	e.cmds = append(e.cmds, c)
	e.mut.Unlock()
	has, out := e.hasOutputForArgs(args)
	if has {
		stderr.Write(out)
	}
	return errors.New("")
}

func (e *ErrorExecutor) ExecutedCmd(cmd string, args []string) bool {
	e.mut.RLock()
	defer e.mut.RUnlock()
	for _, c := range e.cmds {
		if c.name == cmd && reflect.DeepEqual(c.args, args) {
			return true
		}
	}
	return false
}
