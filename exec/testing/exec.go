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
	cmd  string
	args []string
}

func (e *FakeExecutor) Execute(cmd string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	e.cmd = cmd
	e.args = args
	return nil
}

func (e *FakeExecutor) ExecutedCmd(cmd string, args []string) bool {
	return cmd == e.cmd && reflect.DeepEqual(args, e.args)
}
