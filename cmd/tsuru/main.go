package main

import (
	"github.com/timeredbull/tsuru/cmd"
	"os"
)

const version = "0.1"

func buildManager(name string) *cmd.Manager {
	m := cmd.BuildBaseManager(name, version)
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
	name := cmd.ExtractProgramName(os.Args[0])
	manager := buildManager(name)
	args := os.Args[1:]
	manager.Run(args)
}
