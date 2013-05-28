// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"github.com/globocom/tsuru/cmd"
	"os"
)

const (
	version = "0.5.1"
	header  = "Supported-Crane"
)

func buildManager(name string) *cmd.Manager {
	m := cmd.BuildBaseManager(name, version, header)
	m.Register(&ServiceCreate{})
	m.Register(&ServiceRemove{})
	m.Register(&ServiceList{})
	m.Register(&ServiceUpdate{})
	m.Register(&ServiceDocGet{})
	m.Register(&ServiceDocAdd{})
	m.Register(&ServiceTemplate{})
	return m
}

func main() {
	name := cmd.ExtractProgramName(os.Args[0])
	manager := buildManager(name)
	args := os.Args[1:]
	manager.Run(args)
}
