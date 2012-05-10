// +build ignore

package main

import (
	"github.com/timeredbull/tsuru/cmd"
	"os"
)

func main() {
	manager := cmd.NewManager(os.Stdout, os.Stderr)
	manager.Register(&cmd.AppsCommand{})
	manager.Register(&cmd.LoginCommand{})
	manager.Register(&cmd.AddUserCommand{})
	manager.Register(&cmd.CreateAppCommand{})
	//removing the command name from args
	args := os.Args[1:]
	manager.Run(args)
}
