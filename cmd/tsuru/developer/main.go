// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"github.com/globocom/tsuru/cmd"
	"github.com/globocom/tsuru/cmd/tsuru"
	"launchpad.net/gnuflag"
	"os"
)

const (
	version = "0.4"
	header  = "Supported-Tsuru"
)

func buildManager(name string) *cmd.Manager {
	m := cmd.BuildBaseManager(name, version, header)
	m.Register(&tsuru.AppRun{})
	m.Register(&tsuru.AppInfo{})
	m.Register(&AppCreate{})
	m.Register(&AppRemove{})
	m.Register(&UnitAdd{})
	m.Register(&UnitRemove{})
	m.Register(&tsuru.AppList{})
	m.Register(&tsuru.AppLog{})
	m.Register(&tsuru.AppGrant{})
	m.Register(&tsuru.AppRevoke{})
	m.Register(&tsuru.AppRestart{})
	m.Register(&tsuru.EnvGet{})
	m.Register(&tsuru.EnvSet{})
	m.Register(&tsuru.EnvUnset{})
	m.Register(&KeyAdd{})
	m.Register(&KeyRemove{})
	m.Register(&tsuru.ServiceList{})
	m.Register(&tsuru.ServiceAdd{})
	m.Register(&tsuru.ServiceRemove{})
	m.Register(&tsuru.ServiceBind{})
	m.Register(&tsuru.ServiceUnbind{})
	m.Register(&tsuru.ServiceDoc{})
	m.Register(&tsuru.ServiceInfo{})
	m.Register(&tsuru.ServiceInstanceStatus{})
	return m
}

func main() {
	gnuflag.Parse(true)
	name := cmd.ExtractProgramName(os.Args[0])
	manager := buildManager(name)
	args := gnuflag.Args()
	manager.Run(args)
}
