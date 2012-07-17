package main

import (
	"github.com/timeredbull/tsuru/cmd"
	. "launchpad.net/gocheck"
	"net/http"
)

func (s *S) TestServiceInfo(c *C) {
	cmd := Service{}
	i := cmd.Info()
	c.Assert(i.Name, Equals, "service")
	c.Assert(i.Usage, Equals, "service (init|list|create|remove|update) [args]")
	c.Assert(i.Desc, Equals, "manage services.")
	c.Assert(i.MinArgs, Equals, 1)
}

func (s *S) TestServiceSubcommand(c *C) {
	cmd := Service{}
	sc := cmd.Subcommands()
	c.Assert(sc["create"], FitsTypeOf, &ServiceCreate{})
}

func (s *S) TestServiceCreateInfo(c *C) {
	desc := "Creates a service based on a passed manifest. The manifest format should be a yaml and follow the standard described in the documentation (should link to it here)"
	cmd := ServiceCreate{}
	i := cmd.Info()
	c.Assert(i.Name, Equals, "create")
	c.Assert(i.Usage, Equals, "create path/to/manifesto")
	c.Assert(i.Desc, Equals, desc)
	c.Assert(i.MinArgs, Equals, 1)
}

func (s *S) TestServiceCreateRun(c *C) {
	result := "service someservice successfully created"
	args := []string{"testdata/manifest.yml"}
	context := cmd.Context{
		Cmds:   []string{},
		Args:   args,
		Stdout: manager.Stdout,
		Stderr: manager.Stderr,
	}
	client := cmd.NewClient(&http.Client{Transport: &transport{msg: result, status: http.StatusOK}})
	err := (&ServiceCreate{}).Run(&context, client)
	c.Assert(err, IsNil)
}
