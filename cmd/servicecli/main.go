package main

import (
	"github.com/timeredbull/tsuru/cmd"
	"os"
)

func buildManager(name string) cmd.Manager {
	m := cmd.BuildBaseManager(name)
	m.Register(&ServiceCreate{})
	m.Register(&ServiceRemove{})
	m.Register(&ServiceList{})
	return m
}

func main() {
	name := cmd.ExtractProgramName(os.Args[0])
	manager := buildManager(name)
	args := os.Args[1:]
	manager.Run(args)
}
