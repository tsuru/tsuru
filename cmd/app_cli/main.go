package app_cli

import (
	"github.com/timeredbull/tsuru/cmd"
	"os"
	"strings"
)

func extractProgramName(path string) string {
	parts := strings.Split(path, "/")
	return parts[len(parts)-1]
}

func buildManager(name string) cmd.Manager {
	m := cmd.NewManager(name, os.Stdout, os.Stderr)
	m.Register(&cmd.Login{})
	m.Register(&cmd.Logout{})
	m.Register(&cmd.User{})
	m.Register(&App{})
	m.Register(&AppRun{})
	m.Register(&Env{})
	m.Register(&Key{})
	m.Register(&cmd.Team{})
	m.Register(&cmd.Target{})
	return m
}

func main() {
	name := extractProgramName(os.Args[0])
	manager := buildManager(name)
	args := os.Args[1:]
	manager.Run(args)
}
