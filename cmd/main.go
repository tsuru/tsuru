// +build ignore

package main

import (
	"github.com/timeredbull/tsuru/cmd"
	"os"
)

func main() {
	name := cmd.ExtractProgramName(os.Args[0])
	manager := cmd.NewManager(name, os.Stdout, os.Stderr)
	manager.Register(&cmd.Login{})
	manager.Register(&cmd.Logout{})
	manager.Register(&cmd.User{})
	manager.Register(&cmd.App{})
	manager.Register(&cmd.Key{})
	manager.Register(&cmd.Team{})
	manager.Register(&cmd.Target{})
	manager.Register(&cmd.Env{})
	//removing the command name from args
	args := os.Args[1:]
	manager.Run(args)
}
