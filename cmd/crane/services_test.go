package main

import (
	"bytes"
	"github.com/globocom/tsuru/cmd"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"net/http"
	"os"
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
	var stdout, stderr bytes.Buffer
	result := "service someservice successfully created"
	args := []string{"testdata/manifest.yml"}
	context := cmd.Context{
		Args:   args,
		Stdout: &stdout,
		Stderr: &stderr,
	}
	client := cmd.NewClient(&http.Client{Transport: &transport{msg: result, status: http.StatusOK}})
	err := (&ServiceCreate{}).Run(&context, client)
	c.Assert(err, IsNil)
}

func (s *S) TestServiceRemoveRun(c *C) {
	var (
		called         bool
		stdout, stderr bytes.Buffer
	)
	context := cmd.Context{
		Args:   []string{"my-service"},
		Stdout: &stdout,
		Stderr: &stderr,
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
	c.Assert(stdout.String(), Equals, "Service successfully removed.\n")
}

func (s *S) TestServiceRemoveRunWithRequestFailure(c *C) {
	var stdout, stderr bytes.Buffer
	context := cmd.Context{
		Args:   []string{"my-service"},
		Stdout: &stdout,
		Stderr: &stderr,
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
	var stdout, stderr bytes.Buffer
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
		Args:   []string{},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	err := (&ServiceList{}).Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(stdout.String(), Equals, expected)
}

func (s *S) TestServiceListRunWithNoServicesReturned(c *C) {
	var stdout, stderr bytes.Buffer
	response := `[]`
	expected := ""
	trans := transport{msg: response, status: http.StatusOK}
	client := cmd.NewClient(&http.Client{Transport: &trans})
	context := cmd.Context{
		Args:   []string{},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	err := (&ServiceList{}).Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(stdout.String(), Equals, expected)
}

func (s *S) TestServiceUpdate(c *C) {
	var (
		called         bool
		stdout, stderr bytes.Buffer
	)
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
		Args:   []string{"testdata/manifest.yml"},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	err := (&ServiceUpdate{}).Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(called, Equals, true)
	c.Assert(stdout.String(), Equals, "Service successfully updated.\n")
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

func (s *S) TestServiceDocAdd(c *C) {
	var (
		called         bool
		stdout, stderr bytes.Buffer
	)
	trans := conditionalTransport{
		transport{
			msg:    "",
			status: http.StatusNoContent,
		},
		func(req *http.Request) bool {
			called = true
			return req.Method == "PUT" && req.URL.Path == "/services/serv/doc"
		},
	}
	client := cmd.NewClient(&http.Client{Transport: &trans})
	context := cmd.Context{
		Args:   []string{"serv", "testdata/doc.md"},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	err := (&ServiceDocAdd{}).Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(called, Equals, true)
	c.Assert(stdout.String(), Equals, "Documentation for 'serv' successfully updated.\n")
}

func (s *S) TestServiceDocAddInfo(c *C) {
	expected := &cmd.Info{
		Name:    "doc-add",
		Usage:   "service doc-add <service> <path/to/docfile>",
		Desc:    "Update service documentation, extracting it from the given file.",
		MinArgs: 2,
	}
	c.Assert((&ServiceDocAdd{}).Info(), DeepEquals, expected)
}

func (s *S) TestServiceDocGet(c *C) {
	var (
		called         bool
		stdout, stderr bytes.Buffer
	)
	trans := conditionalTransport{
		transport{
			msg:    "some doc",
			status: http.StatusNoContent,
		},
		func(req *http.Request) bool {
			called = true
			return req.Method == "GET" && req.URL.Path == "/services/serv/doc"
		},
	}
	client := cmd.NewClient(&http.Client{Transport: &trans})
	context := cmd.Context{
		Args:   []string{"serv"},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	err := (&ServiceDocGet{}).Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(called, Equals, true)
	c.Assert(context.Stdout.(*bytes.Buffer).String(), Equals, "some doc")
}

func (s *S) TestServiceDocGetInfo(c *C) {
	expected := &cmd.Info{
		Name:    "doc-get",
		Usage:   "service doc-get <service>",
		Desc:    "Shows service documentation.",
		MinArgs: 1,
	}
	c.Assert((&ServiceDocGet{}).Info(), DeepEquals, expected)
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

func (s *S) TestServiceTemplateRun(c *C) {
	var stdout, stderr bytes.Buffer
	trans := transport{msg: "", status: http.StatusOK}
	client := cmd.NewClient(&http.Client{Transport: &trans})
	ctx := cmd.Context{
		Args:   []string{},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	err := (&ServiceTemplate{}).Run(&ctx, client)
	defer os.Remove("./manifest.yaml")
	c.Assert(err, IsNil)
	expected := "Generated file \"manifest.yaml\" in current path\n"
	c.Assert(stdout.String(), Equals, expected)
	f, err := os.Open("./manifest.yaml")
	c.Assert(err, IsNil)
	fc, err := ioutil.ReadAll(f)
	manifest := `id: servicename
endpoint:
  production: production-endpoint.com
  test: test-endpoint.com:8080`
	c.Assert(string(fc), Equals, manifest)
}
