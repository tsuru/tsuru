package main

import (
	"bytes"
	"github.com/timeredbull/tsuru/cmd"
	. "launchpad.net/gocheck"
	"net/http"
)

func (s *S) TestServiceInfo(c *C) {
	expected := &cmd.Info{
		Name:    "service",
		Usage:   "service (list)",
		Desc:    "manage your services",
		MinArgs: 1,
	}
	command := &Service{}
	c.Assert(command.Info(), DeepEquals, expected)
}

func (s *S) TestServiceShouldBeInfoer(c *C) {
	var infoer cmd.Infoer
	c.Assert(&Service{}, Implements, &infoer)
}

func (s *S) TestServiceList(c *C) {
	output := `{"mysql": ["mysql01", "mysql02"], "oracle": []}`
	expectedPrefix := `+---------+------------------+
| Service | Instances        |`
	lineMysql := "| mysql   | mysql01, mysql02 |"
	lineOracle := "| oracle  |                  |"
	ctx := cmd.Context{
		Cmds:   []string{},
		Args:   []string{},
		Stdout: manager.Stdout,
		Stderr: manager.Stderr,
	}
	trans := &conditionalTransport{
		transport{
			msg:    output,
			status: http.StatusOK,
		},
		func(req *http.Request) bool {
			return req.URL.Path == "/services"
		},
	}
	client := cmd.NewClient(&http.Client{Transport: trans})
	err := (&ServiceList{}).Run(&ctx, client)
	c.Assert(err, IsNil)
	table := manager.Stdout.(*bytes.Buffer).String()
	c.Assert(table, Matches, "^"+expectedPrefix+".*")
	c.Assert(table, Matches, "^.*"+lineMysql+".*")
	c.Assert(table, Matches, "^.*"+lineOracle+".*")
}

func (s *S) TestServiceListWithEmptyResponse(c *C) {
	output := "{}"
	expected := ""
	ctx := cmd.Context{
		Cmds:   []string{},
		Args:   []string{},
		Stdout: manager.Stdout,
		Stderr: manager.Stderr,
	}
	trans := &conditionalTransport{
		transport{
			msg:    output,
			status: http.StatusOK,
		},
		func(req *http.Request) bool {
			return req.URL.Path == "/services"
		},
	}
	client := cmd.NewClient(&http.Client{Transport: trans})
	err := (&ServiceList{}).Run(&ctx, client)
	c.Assert(err, IsNil)
	c.Assert(manager.Stdout.(*bytes.Buffer).String(), Equals, expected)
}

func (s *S) TestInfoServiceList(c *C) {
	expected := &cmd.Info{
		Name:  "list",
		Usage: "service list",
		Desc:  "Get all available services, and user's instances for this services",
	}
	command := &ServiceList{}
	c.Assert(command.Info(), DeepEquals, expected)
}

func (s *S) TestServiceListShouldBeInfoer(c *C) {
	var infoer cmd.Infoer
	c.Assert(&ServiceList{}, Implements, &infoer)
}

func (s *S) TestServiceListShouldBeCommand(c *C) {
	var command cmd.Command
	c.Assert(&ServiceList{}, Implements, &command)
}

func (s *S) TestServiceListIsASubcommandOfService(c *C) {
	command := &Service{}
	subc := command.Subcommands()
	list, ok := subc["list"]
	c.Assert(ok, Equals, true)
	c.Assert(list, FitsTypeOf, &ServiceList{})
}

func (s *S) TestServiceAddShouldBeASubcommandOfService(c *C) {
	command := &Service{}
	subcmds := command.Subcommands()
	add, ok := subcmds["add"]
	c.Assert(ok, Equals, true)
	c.Assert(add, FitsTypeOf, &ServiceAdd{})
}

func (s *S) TestServiceAddInfo(c *C) {
	usage := `service add appname serviceinstancename servicename
    e.g.:
    $ service add tsuru tsuru_db mongodb`
	expected := &cmd.Info{
		Name:    "add",
		Usage:   usage,
		Desc:    "Create a service instance to one or more apps make use of.",
		MinArgs: 3,
	}
	command := &ServiceAdd{}
	c.Assert(command.Info(), DeepEquals, expected)
}

func (s *S) TestServiceAddRun(c *C) {
	result := "service instance my_app_db successfuly created"
	args := []string{
		"my_app",
		"my_app_db",
		"mysql",
	}
	context := cmd.Context{
		Cmds:   []string{},
		Args:   args,
		Stdout: manager.Stdout,
		Stderr: manager.Stderr,
	}
	client := cmd.NewClient(&http.Client{Transport: &transport{msg: result, status: http.StatusOK}})
	err := (&ServiceAdd{}).Run(&context, client)
	c.Assert(err, IsNil)
	obtained := manager.Stdout.(*bytes.Buffer).String()
	c.Assert(obtained, Equals, result)
}
