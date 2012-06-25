package cmd

import (
	"os"
	"strings"
)

func extractProgramName(path string) string {
	parts := strings.Split(path, "/")
	return parts[len(parts)-1]
}

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
	name := extractProgramName(os.Args[0])
	manager := buildManager(name)
	args := os.Args[1:]
	manager.Run(args)
}
