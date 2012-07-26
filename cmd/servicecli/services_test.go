package main

import (
	"bytes"
	"github.com/timeredbull/tsuru/cmd"
	. "launchpad.net/gocheck"
	"net/http"
)

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

func (s *S) TestServiceRemoveRun(c *C) {
	var called bool
	context := cmd.Context{
		Cmds:   []string{},
		Args:   []string{"my-service"},
		Stdout: manager.Stdout,
		Stderr: manager.Stderr,
	}
	trans := &conditionalTransport{
		transport{
			msg:    "",
			status: http.StatusNoContent,
		},
		func(req *http.Request) bool {
			called = true
			return req.Method == "DELETE" && req.URL.Path == "/services/my-service"
		},
	}
	client := cmd.NewClient(&http.Client{Transport: trans})
	err := (&ServiceRemove{}).Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(called, Equals, true)
	c.Assert(manager.Stdout.(*bytes.Buffer).String(), Equals, "Service successfully removed.\n")
}

func (s *S) TestServiceRemoveRunWithRequestFailure(c *C) {
	context := cmd.Context{
		Cmds:   []string{},
		Args:   []string{"my-service"},
		Stdout: manager.Stdout,
		Stderr: manager.Stderr,
	}
	trans := transport{
		msg:    "This service cannot be removed because it has instances.\nPlease remove these instances before removing the service.",
		status: http.StatusForbidden,
	}
	client := cmd.NewClient(&http.Client{Transport: &trans})
	err := (&ServiceRemove{}).Run(&context, client)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, trans.msg)
}

func (s *S) TestServiceRemoveIsACommand(c *C) {
	var command cmd.Command
	c.Assert(&ServiceRemove{}, Implements, &command)
}

func (s *S) TestServiceRemoveInfo(c *C) {
	expected := &cmd.Info{
		Name:    "remove",
		Usage:   "remove <servicename>",
		Desc:    "removes a service from catalog",
		MinArgs: 1,
	}
	c.Assert((&ServiceRemove{}).Info(), DeepEquals, expected)
}

func (s *S) TestServiceRemoveIsAnInfor(c *C) {
	var infoer cmd.Infoer
	c.Assert(&ServiceRemove{}, Implements, &infoer)
}

func (s *S) TestServiceListInfo(c *C) {
	cmd := ServiceList{}
	i := cmd.Info()
	c.Assert(i.Name, Equals, "list")
	c.Assert(i.Usage, Equals, "list")
	c.Assert(i.Desc, Equals, "list services that belongs to user's team and it's service instances.")
}

func (s *S) TestServiceListRun(c *C) {
	response := `[{"service": "mysql", "instances": ["my_db"]}]`
	expected := `+----------+-----------+
| Services | Instances |
+----------+-----------+
| mysql    | my_db     |
+----------+-----------+
`
	trans := transport{msg: response, status: http.StatusOK}
	client := cmd.NewClient(&http.Client{Transport: &trans})
	context := cmd.Context{
		Cmds:   []string{},
		Args:   []string{},
		Stdout: manager.Stdout,
		Stderr: manager.Stderr,
	}
	err := (&ServiceList{}).Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(manager.Stdout.(*bytes.Buffer).String(), Equals, expected)
}

func (s *S) TestServiceListRunWithNoServicesReturned(c *C) {
	response := `[]`
	expected := ""
	trans := transport{msg: response, status: http.StatusOK}
	client := cmd.NewClient(&http.Client{Transport: &trans})
	context := cmd.Context{
		Cmds:   []string{},
		Args:   []string{},
		Stdout: manager.Stdout,
		Stderr: manager.Stderr,
	}
	err := (&ServiceList{}).Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(manager.Stdout.(*bytes.Buffer).String(), Equals, expected)
}

func (s *S) TestServiceUpdate(c *C) {
	var called bool
	trans := conditionalTransport{
		transport{
			msg:    "",
			status: http.StatusNoContent,
		},
		func(req *http.Request) bool {
			called = true
			return req.Method == "PUT" && req.URL.Path == "/services"
		},
	}
	client := cmd.NewClient(&http.Client{Transport: &trans})
	context := cmd.Context{
		Cmds:   []string{},
		Args:   []string{"testdata/manifest.yml"},
		Stdout: manager.Stdout,
		Stderr: manager.Stderr,
	}
	err := (&ServiceUpdate{}).Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(called, Equals, true)
	c.Assert(context.Stdout.(*bytes.Buffer).String(), Equals, "Service successfully updated.\n")
}

func (s *S) TestServiceUpdateIsACommand(c *C) {
	var cmd cmd.Command
	c.Assert(&ServiceUpdate{}, Implements, &cmd)
}

func (s *S) TestServiceUpdateInfo(c *C) {
	expected := &cmd.Info{
		Name:    "update",
		Usage:   "update <path/to/manifesto>",
		Desc:    "Update service data, extracting it from the given manifesto file.",
		MinArgs: 1,
	}
	c.Assert((&ServiceUpdate{}).Info(), DeepEquals, expected)
}

func (s *S) TestServiceUpdateIsAnInfoer(c *C) {
	var infoer cmd.Infoer
	c.Assert(&ServiceUpdate{}, Implements, &infoer)
}

func (s *S) TestServiceTemplateInfo(c *C) {
	got := (&ServiceTemplate{}).Info()
	usg := `template
e.g.: $ crane template`
	expected := &cmd.Info{
		Name:  "template",
		Usage: usg,
		Desc:  "Generates a manifest template file and places it in current path",
	}
	c.Assert(got, DeepEquals, expected)
}
