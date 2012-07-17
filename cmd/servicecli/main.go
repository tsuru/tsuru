package main

import (
	"github.com/timeredbull/tsuru/cmd"
	"os"
)

func buildManager(name string) cmd.Manager {
	m := cmd.NewManager(name, os.Stdout, os.Stderr)
	m.Register(&cmd.Login{})
	m.Register(&cmd.Logout{})
	m.Register(&cmd.User{})
	m.Register(&cmd.Team{})
	m.Register(&cmd.Target{})
	m.Register(&ServiceCreate{})
	m.Register(&ServiceRemove{})
	return m
}

func main() {
	name := cmd.ExtractProgramName(os.Args[0])
	manager := buildManager(name)
	args := os.Args[1:]
	manager.Run(args)
}
