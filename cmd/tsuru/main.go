// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"github.com/globocom/tsuru/cmd"
	"launchpad.net/gnuflag"
	"os"
)

const (
	version = "0.4"
	header  = "Supported-Tsuru"
)

var appName = gnuflag.String("app", "", "App name for running app related commands.")
var assumeYes = gnuflag.Bool("assume-yes", false, "Don't ask for confirmation on operations.")
var logLines = gnuflag.Int("lines", 10, "The number of log lines to display")
var logSource = gnuflag.String("source", "", "The log from the given source")

func buildManager(name string) *cmd.Manager {
	m := cmd.BuildBaseManager(name, version, header)
	m.Register(&AppRun{})
	m.Register(&AppInfo{})
	m.Register(&AppCreate{})
	m.Register(&AppRemove{})
	m.Register(&AppList{})
	m.Register(&AppLog{})
	m.Register(&AppGrant{})
	m.Register(&AppRevoke{})
	m.Register(&AppRestart{})
	m.Register(&EnvGet{})
	m.Register(&EnvSet{})
	m.Register(&EnvUnset{})
	m.Register(&KeyAdd{})
	m.Register(&KeyRemove{})
	m.Register(&ServiceList{})
	m.Register(&ServiceAdd{})
	m.Register(&ServiceRemove{})
	m.Register(&ServiceBind{})
	m.Register(&ServiceUnbind{})
	m.Register(&ServiceDoc{})
	m.Register(&ServiceInfo{})
	m.Register(&ServiceInstanceStatus{})
	return m
}

func main() {
	gnuflag.Parse(true)
	name := cmd.ExtractProgramName(os.Args[0])
	manager := buildManager(name)
	args := gnuflag.Args()
	manager.Run(args)
}
