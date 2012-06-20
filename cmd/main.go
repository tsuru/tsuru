package main

import (
	"os"
)

func main() {
	name := ExtractProgramName(os.Args[0])
	manager := NewManager(name, os.Stdout, os.Stderr)
	manager.Register(&Login{})
	manager.Register(&Logout{})
	manager.Register(&User{})
	manager.Register(&App{})
	manager.Register(&Key{})
	manager.Register(&Team{})
	manager.Register(&Target{})
	manager.Register(&Env{})
	manager.Register(&AppRun{})
	//removing the command name from args
	args := os.Args[1:]
	manager.Run(args)
}
