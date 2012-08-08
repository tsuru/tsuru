package main

import (
	"bytes"
	"github.com/timeredbull/tsuru/cmd"
	. "launchpad.net/gocheck"
	"net/http"
	"os/user"
	"path"
)

func (s *S) TestAddKey(c *C) {
	u, err := user.Current()
	c.Assert(err, IsNil)
	p := path.Join(u.HomeDir, ".ssh", "id_rsa.pub")
	expected := "Key successfully added!\n"
	context := cmd.Context{
		Cmds:   []string{},
		Args:   []string{},
		Stdout: manager.Stdout,
		Stderr: manager.Stderr,
	}
	client := cmd.NewClient(&http.Client{Transport: &transport{msg: "success", status: http.StatusOK}})
	fs := RecordingFs{fileContent: "user-key"}
	command := AddKeyCommand{keyReader{fsystem: &fs}}
	err = command.Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(manager.Stdout.(*bytes.Buffer).String(), Equals, expected)
	c.Assert(fs.HasAction("open "+p), Equals, true)
}

func (s *S) TestAddKeyReturnErrorIfTheKeyDoesNotExist(c *C) {
	context := cmd.Context{
		Cmds:   []string{},
		Args:   []string{},
		Stdout: manager.Stdout,
		Stderr: manager.Stderr,
	}
	fs := FailureFs{RecordingFs{}}
	command := AddKeyCommand{keyReader{fsystem: &fs}}
	err := command.Run(&context, nil)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "You need to have a public rsa key")
}

func (s *S) TestRemoveKey(c *C) {
	u, err := user.Current()
	c.Assert(err, IsNil)
	p := path.Join(u.HomeDir, ".ssh", "id_rsa.pub")
	expected := "Key successfully removed!\n"
	context := cmd.Context{
		Cmds:   []string{},
		Args:   []string{},
		Stdout: manager.Stdout,
		Stderr: manager.Stderr,
	}
	client := cmd.NewClient(&http.Client{Transport: &transport{msg: "success", status: http.StatusOK}})
	fs := RecordingFs{fileContent: "user-key"}
	command := RemoveKey{keyReader{fsystem: &fs}}
	err = command.Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(manager.Stdout.(*bytes.Buffer).String(), Equals, expected)
	c.Assert(fs.HasAction("open "+p), Equals, true)
}

func (s *S) TestRemoveKeyReturnErrorIfTheKeyDoesNotExist(c *C) {
	context := cmd.Context{
		Cmds:   []string{},
		Args:   []string{},
		Stdout: manager.Stdout,
		Stderr: manager.Stderr,
	}
	fs := FailureFs{RecordingFs{}}
	command := RemoveKey{keyReader{fsystem: &fs}}
	err := command.Run(&context, nil)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "You need to have a public rsa key")
}

func (s *S) TestKey(c *C) {
	expect := map[string]interface{}{
		"add":    &AddKeyCommand{},
		"remove": &RemoveKey{},
	}
	command := Key{}
	c.Assert(command.Subcommands(), DeepEquals, expect)
}
