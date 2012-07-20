package main

import (
	"github.com/timeredbull/tsuru/cmd"
	"os"
)

func buildManager(name string) cmd.Manager {
	m := cmd.BuildBaseManager(name)
	m.Register(&App{})
	m.Register(&AppRun{})
	m.Register(&Env{})
	m.Register(&Key{})
	m.Register(&Service{})
	return m
}

func main() {
	name := cmd.ExtractProgramName(os.Args[0])
	manager := buildManager(name)
	args := os.Args[1:]
	manager.Run(args)
}
