// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"github.com/tsuru/tsuru/cmd"
	"github.com/tsuru/tsuru/cmd/testing"
	"launchpad.net/gocheck"
	"net/http"
)

func (s *S) TestPlatformAddInfo(c *gocheck.C) {
	expected := &cmd.Info{
		Name:    "platform-add",
		Usage:   "platform-add <platform name> [--dockerfile/-d Dockerfile]",
		Desc:    "Add new platform to tsuru.",
		MinArgs: 1,
	}

	c.Assert((&platformAdd{}).Info(), gocheck.DeepEquals, expected)
}

func (s *S) TestPlatformAddRun(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
		Args:   []string{"teste"},
	}

	expected := "Platform successfully added!\n"
	trans := &testing.ConditionalTransport{
		Transport: testing.Transport{Message: "", Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			c.Assert(req.Header.Get("Content-Type"), gocheck.Equals, "application/x-www-form-urlencoded")
			return req.URL.Path == "/platforms" && req.Method == "POST"
		},
	}

	client := cmd.NewClient(&http.Client{Transport: trans}, nil, manager)
	command := platformAdd{}
	command.Flags().Parse(true, []string{"--dockerfile", "http://localhost/Dockerfile"})

	err := command.Run(&context, client)

	c.Assert(err, gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, expected)
}

func (s *S) TestPlatformAddFlagSet(c *gocheck.C) {
	message := "The dockerfile url to create a platform"
	command := platformAdd{}
	flagset := command.Flags()
	flagset.Parse(true, []string{"--dockerfile", "dockerfile"})

	dockerfile := flagset.Lookup("dockerfile")
	c.Check(dockerfile.Name, gocheck.Equals, "dockerfile")
	c.Check(dockerfile.Usage, gocheck.Equals, message)
	c.Check(dockerfile.DefValue, gocheck.Equals, "")

	sdockerfile := flagset.Lookup("d")
	c.Check(sdockerfile.Name, gocheck.Equals, "d")
	c.Check(sdockerfile.Usage, gocheck.Equals, message)
	c.Check(sdockerfile.DefValue, gocheck.Equals, "")
}

func (s *S) TestPlatformUpdateInfo(c *gocheck.C) {
	expected := &cmd.Info{
		Name:    "platform-update",
		Usage:   "platform-update <platform name> [--dockerfile/-d Dockerfile]",
		Desc:    "Update a platform to tsuru.",
		MinArgs: 1,
	}
	c.Assert((&platformUpdate{}).Info(), gocheck.DeepEquals, expected)
}

func (s *S) TestPlatformUpdateFlagSet(c *gocheck.C) {
	dockerfile_message := "The dockerfile url to update a platform"
	command := platformUpdate{}
	flagset := command.Flags()
	flagset.Parse(true, []string{"--dockerfile", "dockerfile"})

	dockerfile := flagset.Lookup("dockerfile")
	c.Check(dockerfile.Name, gocheck.Equals, "dockerfile")
	c.Check(dockerfile.Usage, gocheck.Equals, dockerfile_message)
	c.Check(dockerfile.DefValue, gocheck.Equals, "")

	sdockerfile := flagset.Lookup("d")
	c.Check(sdockerfile.Name, gocheck.Equals, "d")
	c.Check(sdockerfile.Usage, gocheck.Equals, dockerfile_message)
	c.Check(sdockerfile.DefValue, gocheck.Equals, "")
}

func (s *S) TestPlatformUpdateRun(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	name := "teste"
	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
		Args:   []string{name},
	}
	expected := "Platform successfully updated!\n"
	trans := &testing.ConditionalTransport{
		Transport: testing.Transport{Message: "", Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			c.Assert(req.Header.Get("Content-Type"), gocheck.Equals, "application/x-www-form-urlencoded")
			c.Assert(req.FormValue("dockerfile"), gocheck.Equals, "http://localhost/Dockerfile")
			return req.URL.Path == "/platforms/"+name && req.Method == "PUT"
		},
	}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, manager)
	command := platformUpdate{}
	command.Flags().Parse(true, []string{"--dockerfile", "http://localhost/Dockerfile"})
	err := command.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, expected)
}
