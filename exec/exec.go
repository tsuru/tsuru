// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package exec provides a interface to run external commans as an
// abstraction layer.
package exec

import "io"

type Executor interface {
	// Execute executes the specified command.
	Execute(cmds []string, in io.Reader, out, err io.Writer) error
}
