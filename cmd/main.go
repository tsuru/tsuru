// +build ignore

package main

import (
	"github.com/timeredbull/tsuru/cmd"
	"os"
)

func main() {
	manager := cmd.NewManager(os.Stdout, os.Stderr)
	manager.Register(&cmd.AppsCommand{})
	//removing the command name from args
	args := os.Args[1:]
	manager.Run(args)
}
