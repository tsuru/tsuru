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
	manager.Register(&cmd.LogoutCommand{})
	manager.Register(&cmd.AddUserCommand{})
	manager.Register(&cmd.CreateAppCommand{})
	manager.Register(&cmd.CreateTeamCommand{})
	manager.Register(&cmd.App{})
	manager.Register(&cmd.Key{})
	manager.Register(&cmd.DeleteAppCommand{})
	manager.Register(&cmd.TeamCommand{})
	//removing the command name from args
	args := os.Args[1:]
	manager.Run(args)
}
