package main

import (
	"os"
)

func buildManager(name string) Manager {
	m := NewManager(name, os.Stdout, os.Stderr)
	m.Register(&Login{})
	m.Register(&Logout{})
	m.Register(&User{})
	m.Register(&App{})
	m.Register(&AppRun{})
	m.Register(&Env{})
	m.Register(&Key{})
	m.Register(&Team{})
	m.Register(&Target{})
	return m
}

func main() {
	name := ExtractProgramName(os.Args[0])
	manager := buildManager(name)
	args := os.Args[1:]
	manager.Run(args)
}
