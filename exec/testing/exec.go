// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package testing provides fake implementations of the exec package.
//
// These implementations can be used to mock out the Executor in tests.
package testing

import (
	"errors"
	"github.com/globocom/tsuru/safe"
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
	cmds    []command
	mut     sync.RWMutex
	callMut sync.Mutex
	calls   map[string]int
	Output  map[string][][]byte
}

func (e *FakeExecutor) hasOutputForArgs(args []string) (bool, []byte) {
	e.callMut.Lock()
	defer e.callMut.Unlock()
	if e.calls == nil {
		e.calls = make(map[string]int)
	}
	var generic []byte
	sArgs := strings.Join(args, " ")
	for k, v := range e.Output {
		switch k {
		case sArgs:
			counter := e.calls[sArgs]
			out := v[counter%len(v)]
			e.calls[sArgs] = counter + 1
			return true, out
		case "*":
			generic = v[e.calls["*"]%len(v)]
		}
	}
	if generic != nil {
		e.calls["*"]++
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
	e.mut.RLock()
	defer e.mut.RUnlock()
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

// RetryExecutor succeeds after N failures.
type RetryExecutor struct {
	FakeExecutor

	// How many times will it fail before succeeding?
	Failures int64
	calls    safe.Counter
}

func (e *RetryExecutor) Execute(cmd string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	defer e.calls.Increment()
	var err error
	succeed := e.Failures <= e.calls.Val()
	if !succeed {
		stdout = stderr
		err = errors.New("")
	}
	e.FakeExecutor.Execute(cmd, args, stdin, stdout, stderr)
	return err
}

// FailLaterExecutor is the opposite of RetryExecutor. It fails after N
// Succeeds.
type FailLaterExecutor struct {
	FakeExecutor

	Succeeds int64
	calls    safe.Counter
}

func (e *FailLaterExecutor) Execute(cmd string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	defer e.calls.Increment()
	var err error
	fail := e.Succeeds <= e.calls.Val()
	if fail {
		stdout = stderr
		err = errors.New("")
	}
	e.FakeExecutor.Execute(cmd, args, stdin, stdout, stderr)
	return err
}
