package app_cli

import (
	"github.com/timeredbull/tsuru/cmd"
)

func (s *S) TestAddKey(c *C) {
	expected := "Key successfully added!\n"
	context := cmd.Context{[]string{}, []string{}, manager.Stdout, manager.Stderr}
	client := NewClient(&http.Client{Transport: &transport{msg: "", status: http.StatusOK}})
	command := AddKeyCommand{}
	err := command.Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(manager.Stdout.(*bytes.Buffer).String(), Equals, expected)
}

func (s *S) TestRemoveKey(c *C) {
	expected := "Key successfully removed!\n"
	context := cmd.Context{[]string{}, []string{}, manager.Stdout, manager.Stderr}
	client := NewClient(&http.Client{Transport: &transport{msg: "", status: http.StatusOK}})
	command := RemoveKey{}
	err := command.Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(manager.Stdout.(*bytes.Buffer).String(), Equals, expected)
}

func (s *S) TestKey(c *C) {
	expect := map[string]interface{}{
		"add":    &AddKeyCommand{},
		"remove": &RemoveKey{},
	}
	command := Key{}
	c.Assert(command.Subcommands(), DeepEquals, expect)
}
