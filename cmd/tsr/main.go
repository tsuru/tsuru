// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"github.com/globocom/tsuru/cmd"
	"os"
)

func buildManager() *cmd.Manager {
	m := cmd.NewManager("tsr", "0.1.0", "", os.Stdout, os.Stderr, os.Stdin)
	m.Register(&apiCmd{})
	return m
}

func main() {
	m := buildManager()
	m.Run(os.Args[1:])
}
