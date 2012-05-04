package cmd

import "fmt"

type AppsCommand struct{}

func (c *AppsCommand) Run() error {
	fmt.Println("app list")
	return nil
}

func (c *AppsCommand) Info() *Info {
	return &Info{Name: "apps"}
}
