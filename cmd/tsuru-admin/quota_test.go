// Copyright 2013 tsuru authors. All rights reserved.
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

func (s *S) TestChangeQuotaInfo(c *gocheck.C) {
	expected := &cmd.Info{
		Name:    "quota-update",
		Usage:   "quota-update [--owner/-o owner's name] [--quota/-q number of quotas]",
		Desc:    `Update quotas.`,
		MinArgs: 0,
	}
	c.Assert((&changeQuota{}).Info(), gocheck.DeepEquals, expected)
}

func (s *S) TestChangeQuotaRun(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
	}

	expected := "Quotas successfully changed!\n"
	trans := &testing.ConditionalTransport{
		Transport: testing.Transport{Message: "", Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			return req.URL.Path == "/quota/qwertyuiop" && req.Method == "PUT"
		},
	}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, manager)
	command := changeQuota{}
	command.Flags().Parse(true, []string{"--owner", "qwertyuiop", "--quota", "1"})
	err := command.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, expected)
}

func (s *S) TestChangeQuotaFlagSet(c *gocheck.C) {
	command := changeQuota{}
	flagset := command.Flags()
	flagset.Parse(true, []string{"--quota", "1"})
	quota := flagset.Lookup("quota")
	c.Check(quota, gocheck.NotNil)
	c.Check(quota.Name, gocheck.Equals, "quota")
	c.Check(quota.Usage, gocheck.Equals, "The number of quotas changed")
	c.Check(quota.Value.String(), gocheck.Equals, "1")
	c.Check(quota.DefValue, gocheck.Equals, "0")
	squota := flagset.Lookup("q")
	c.Check(squota, gocheck.NotNil)
	c.Check(squota.Name, gocheck.Equals, "q")
	c.Check(squota.Usage, gocheck.Equals, "The number of quotas changed")
	c.Check(squota.Value.String(), gocheck.Equals, "1")
	c.Check(squota.DefValue, gocheck.Equals, "0")
}
