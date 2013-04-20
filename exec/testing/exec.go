// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package exec/testing provides fake implementations of the exec package.
//
// These implementations can be used to mock out the Executor in tests.
package testing

import (
	"io"
	"reflect"
)

type FakeExecutor struct {
	cmds map[string][]string
}

func (e *FakeExecutor) Execute(cmd string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if e.cmds == nil {
		e.cmds = make(map[string][]string)
	}
	e.cmds[cmd] = args
	return nil
}

func (e *FakeExecutor) ExecutedCmd(cmd string, args []string) bool {
	for c, a := range e.cmds {
		if cmd == c && reflect.DeepEqual(a, args) {
			return true
		}
	}
	return false
}
