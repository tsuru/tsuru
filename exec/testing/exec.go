// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package testing provides fake implementations of the exec package.
//
// These implementations can be used to mock out the Executor in tests.
package testing

import (
	"errors"
	"reflect"
	"strings"
	"sync"

	"github.com/tsuru/tsuru/exec"
	"github.com/tsuru/tsuru/safe"
)

type command struct {
	name string
	args []string
	envs []string
}

func (c *command) GetName() string {
	return c.name
}

func (c *command) GetArgs() []string {
	return c.args
}

// GetEnvs returns the command environment variables.
func (c *command) GetEnvs() []string {
	return c.envs
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

func (e *FakeExecutor) Execute(opts exec.ExecuteOptions) error {
	c := command{name: opts.Cmd, args: opts.Args, envs: opts.Envs}
	e.mut.Lock()
	e.cmds = append(e.cmds, c)
	e.mut.Unlock()
	has, out := e.hasOutputForArgs(opts.Args)
	if has {
		opts.Stdout.Write(out)
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

func (e *ErrorExecutor) Execute(opts exec.ExecuteOptions) error {
	e.FakeExecutor.Execute(opts)
	return errors.New("")
}

// RetryExecutor succeeds after N failures.
type RetryExecutor struct {
	FakeExecutor

	// How many times will it fail before succeeding?
	Failures int64
	calls    safe.Counter
}

func (e *RetryExecutor) Execute(opts exec.ExecuteOptions) error {
	defer e.calls.Increment()
	var err error
	succeed := e.Failures <= e.calls.Val()
	if !succeed {
		opts.Stdout = opts.Stderr
		err = errors.New("")
	}
	e.FakeExecutor.Execute(opts)
	return err
}

// FailLaterExecutor is the opposite of RetryExecutor. It fails after N
// Succeeds.
type FailLaterExecutor struct {
	FakeExecutor

	Succeeds int64
	calls    safe.Counter
}

func (e *FailLaterExecutor) Execute(opts exec.ExecuteOptions) error {
	defer e.calls.Increment()
	var err error
	fail := e.Succeeds <= e.calls.Val()
	if fail {
		opts.Stdout = opts.Stderr
		err = errors.New("")
	}
	e.FakeExecutor.Execute(opts)
	return err
}
