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
	Output []byte
}

func (e *FakeExecutor) Execute(cmd string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	c := command{name: cmd, args: args}
	e.mut.Lock()
	e.cmds = append(e.cmds, c)
	e.mut.Unlock()
	if e.Output != nil {
		stdout.Write(e.Output)
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
	Output []byte
}

func (e *ErrorExecutor) Execute(cmd string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	c := command{name: cmd, args: args}
	e.mut.Lock()
	e.cmds = append(e.cmds, c)
	e.mut.Unlock()
	if e.Output != nil {
		stderr.Write(e.Output)
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
