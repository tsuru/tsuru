// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"github.com/globocom/tsuru/cmd"
	"github.com/globocom/tsuru/cmd/testing"
	"io/ioutil"
	"launchpad.net/gocheck"
	"net/http"
	"os"
)

func (s *S) TestServiceCreateInfo(c *gocheck.C) {
	desc := "Creates a service based on a passed manifest. The manifest format should be a yaml and follow the standard described in the documentation (should link to it here)"
	cmd := ServiceCreate{}
	i := cmd.Info()
	c.Assert(i.Name, gocheck.Equals, "create")
	c.Assert(i.Usage, gocheck.Equals, "create path/to/manifesto")
	c.Assert(i.Desc, gocheck.Equals, desc)
	c.Assert(i.MinArgs, gocheck.Equals, 1)
}

func (s *S) TestServiceCreateRun(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	args := []string{"testdata/manifest.yml"}
	context := cmd.Context{
		Args:   args,
		Stdout: &stdout,
		Stderr: &stderr,
	}
	trans := testing.Transport{Message: "success", Status: http.StatusOK}
	client := cmd.NewClient(&http.Client{Transport: &trans}, nil, manager)
	err := (&ServiceCreate{}).Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, "success")
}

func (s *S) TestServiceRemoveRun(c *gocheck.C) {
	var (
		called         bool
		stdout, stderr bytes.Buffer
	)
	context := cmd.Context{
		Args:   []string{"my-service"},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	trans := testing.ConditionalTransport{
		Transport: testing.Transport{
			Message: "",
			Status:  http.StatusNoContent,
		},
		CondFunc: func(req *http.Request) bool {
			called = true
			return req.Method == "DELETE" && req.URL.Path == "/services/my-service"
		},
	}
	client := cmd.NewClient(&http.Client{Transport: &trans}, nil, manager)
	err := (&ServiceRemove{}).Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(called, gocheck.Equals, true)
	c.Assert(stdout.String(), gocheck.Equals, "Service successfully removed.\n")
}

func (s *S) TestServiceRemoveRunWithRequestFailure(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	context := cmd.Context{
		Args:   []string{"my-service"},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	trans := testing.Transport{
		Message: "This service cannot be removed because it has instances.\nPlease remove these instances before removing the service.",
		Status:  http.StatusForbidden,
	}
	client := cmd.NewClient(&http.Client{Transport: &trans}, nil, manager)
	err := (&ServiceRemove{}).Run(&context, client)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, trans.Message)
}

func (s *S) TestServiceRemoveIsACommand(c *gocheck.C) {
	var _ cmd.Command = &ServiceRemove{}
}

func (s *S) TestServiceRemoveInfo(c *gocheck.C) {
	expected := &cmd.Info{
		Name:    "remove",
		Usage:   "remove <servicename>",
		Desc:    "removes a service from catalog",
		MinArgs: 1,
	}
	c.Assert((&ServiceRemove{}).Info(), gocheck.DeepEquals, expected)
}

func (s *S) TestServiceListInfo(c *gocheck.C) {
	cmd := ServiceList{}
	i := cmd.Info()
	c.Assert(i.Name, gocheck.Equals, "list")
	c.Assert(i.Usage, gocheck.Equals, "list")
	c.Assert(i.Desc, gocheck.Equals, "list services that belongs to user's team and it's service instances.")
}

func (s *S) TestServiceListRun(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	response := `[{"service": "mysql", "instances": ["my_db"]}]`
	expected := `+----------+-----------+
| Services | Instances |
+----------+-----------+
| mysql    | my_db     |
+----------+-----------+
`
	trans := testing.Transport{Message: response, Status: http.StatusOK}
	client := cmd.NewClient(&http.Client{Transport: &trans}, nil, manager)
	context := cmd.Context{
		Args:   []string{},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	err := (&ServiceList{}).Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, expected)
}

func (s *S) TestServiceListRunWithNoServicesReturned(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	response := `[]`
	expected := ""
	trans := testing.Transport{Message: response, Status: http.StatusOK}
	client := cmd.NewClient(&http.Client{Transport: &trans}, nil, manager)
	context := cmd.Context{
		Args:   []string{},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	err := (&ServiceList{}).Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, expected)
}

func (s *S) TestServiceUpdate(c *gocheck.C) {
	var (
		called         bool
		stdout, stderr bytes.Buffer
	)
	trans := testing.ConditionalTransport{
		Transport: testing.Transport{
			Message: "",
			Status:  http.StatusNoContent,
		},
		CondFunc: func(req *http.Request) bool {
			called = true
			return req.Method == "PUT" && req.URL.Path == "/services"
		},
	}
	client := cmd.NewClient(&http.Client{Transport: &trans}, nil, manager)
	context := cmd.Context{
		Args:   []string{"testdata/manifest.yml"},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	err := (&ServiceUpdate{}).Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(called, gocheck.Equals, true)
	c.Assert(stdout.String(), gocheck.Equals, "Service successfully updated.\n")
}

func (s *S) TestServiceUpdateIsACommand(c *gocheck.C) {
	var _ cmd.Command = &ServiceUpdate{}
}

func (s *S) TestServiceUpdateInfo(c *gocheck.C) {
	expected := &cmd.Info{
		Name:    "update",
		Usage:   "update <path/to/manifesto>",
		Desc:    "Update service data, extracting it from the given manifesto file.",
		MinArgs: 1,
	}
	c.Assert((&ServiceUpdate{}).Info(), gocheck.DeepEquals, expected)
}

func (s *S) TestServiceDocAdd(c *gocheck.C) {
	var (
		called         bool
		stdout, stderr bytes.Buffer
	)
	trans := testing.ConditionalTransport{
		Transport: testing.Transport{Message: "", Status: http.StatusNoContent},
		CondFunc: func(req *http.Request) bool {
			called = true
			return req.Method == "PUT" && req.URL.Path == "/services/serv/doc"
		},
	}
	client := cmd.NewClient(&http.Client{Transport: &trans}, nil, manager)
	context := cmd.Context{
		Args:   []string{"serv", "testdata/doc.md"},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	err := (&ServiceDocAdd{}).Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(called, gocheck.Equals, true)
	c.Assert(stdout.String(), gocheck.Equals, "Documentation for 'serv' successfully updated.\n")
}

func (s *S) TestServiceDocAddInfo(c *gocheck.C) {
	expected := &cmd.Info{
		Name:    "doc-add",
		Usage:   "service doc-add <service> <path/to/docfile>",
		Desc:    "Update service documentation, extracting it from the given file.",
		MinArgs: 2,
	}
	c.Assert((&ServiceDocAdd{}).Info(), gocheck.DeepEquals, expected)
}

func (s *S) TestServiceDocGet(c *gocheck.C) {
	var (
		called         bool
		stdout, stderr bytes.Buffer
	)
	trans := testing.ConditionalTransport{
		Transport: testing.Transport{Message: "some doc", Status: http.StatusNoContent},
		CondFunc: func(req *http.Request) bool {
			called = true
			return req.Method == "GET" && req.URL.Path == "/services/serv/doc"
		},
	}
	client := cmd.NewClient(&http.Client{Transport: &trans}, nil, manager)
	context := cmd.Context{
		Args:   []string{"serv"},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	err := (&ServiceDocGet{}).Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(called, gocheck.Equals, true)
	c.Assert(context.Stdout.(*bytes.Buffer).String(), gocheck.Equals, "some doc")
}

func (s *S) TestServiceDocGetInfo(c *gocheck.C) {
	expected := &cmd.Info{
		Name:    "doc-get",
		Usage:   "service doc-get <service>",
		Desc:    "Shows service documentation.",
		MinArgs: 1,
	}
	c.Assert((&ServiceDocGet{}).Info(), gocheck.DeepEquals, expected)
}

func (s *S) TestServiceTemplateInfo(c *gocheck.C) {
	got := (&ServiceTemplate{}).Info()
	usg := `template
e.g.: $ crane template`
	expected := &cmd.Info{
		Name:  "template",
		Usage: usg,
		Desc:  "Generates a manifest template file and places it in current path",
	}
	c.Assert(got, gocheck.DeepEquals, expected)
}

func (s *S) TestServiceTemplateRun(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	trans := testing.Transport{Message: "", Status: http.StatusOK}
	client := cmd.NewClient(&http.Client{Transport: &trans}, nil, manager)
	ctx := cmd.Context{
		Args:   []string{},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	err := (&ServiceTemplate{}).Run(&ctx, client)
	defer os.Remove("./manifest.yaml")
	c.Assert(err, gocheck.IsNil)
	expected := "Generated file \"manifest.yaml\" in current path\n"
	c.Assert(stdout.String(), gocheck.Equals, expected)
	f, err := os.Open("./manifest.yaml")
	c.Assert(err, gocheck.IsNil)
	fc, err := ioutil.ReadAll(f)
	manifest := `id: servicename
endpoint:
  production: production-endpoint.com
  test: test-endpoint.com:8080`
	c.Assert(string(fc), gocheck.Equals, manifest)
}
