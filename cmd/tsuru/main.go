// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"github.com/tsuru/tsuru/cmd"
	"github.com/tsuru/tsuru/cmd/tsuru-base"
	"os"
)

const (
	version = "0.9.0"
	header  = "Supported-Tsuru"
)

func buildManager(name string) *cmd.Manager {
	lookup := func(m *cmd.Manager, args []string) error {
		context := cmd.Context{Args: args}
		client := cmd.NewClient(nil, nil, m)
		command := plugin{}
		return command.Run(&context, client)
	}
	m := cmd.BuildBaseManager(name, version, header, lookup)
	m.Register(&tsuru.AppRun{})
	m.Register(&tsuru.AppInfo{})
	m.Register(&AppCreate{})
	m.Register(&AppRemove{})
	m.Register(&UnitAdd{})
	m.Register(&UnitRemove{})
	m.Register(tsuru.AppList{})
	m.Register(&tsuru.AppLog{})
	m.Register(&tsuru.AppGrant{})
	m.Register(&tsuru.AppRevoke{})
	m.Register(&tsuru.AppRestart{})
	m.Register(&tsuru.AppStart{})
	m.Register(&tsuru.SetCName{})
	m.Register(&tsuru.UnsetCName{})
	m.Register(&tsuru.EnvGet{})
	m.Register(&tsuru.EnvSet{})
	m.Register(&tsuru.EnvUnset{})
	m.Register(&KeyAdd{})
	m.Register(&KeyRemove{})
	m.Register(tsuru.ServiceList{})
	m.Register(tsuru.ServiceAdd{})
	m.Register(tsuru.ServiceRemove{})
	m.Register(tsuru.ServiceDoc{})
	m.Register(tsuru.ServiceInfo{})
	m.Register(tsuru.ServiceInstanceStatus{})
	m.Register(&tsuru.ServiceBind{})
	m.Register(&tsuru.ServiceUnbind{})
	m.Register(platformList{})
	m.Register(&pluginInstall{})
	m.Register(&pluginRemove{})
	m.Register(&pluginList{})
	m.Register(swap{})
	return m
}

func main() {
	name := cmd.ExtractProgramName(os.Args[0])
	manager := buildManager(name)
	manager.Run(os.Args[1:])
}
