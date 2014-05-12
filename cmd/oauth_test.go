// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	etesting "github.com/tsuru/tsuru/exec/testing"
	"launchpad.net/gocheck"
	"os"
	"runtime"
)

func (s *S) TestClientID(c *gocheck.C) {
	err := os.Setenv("TSURU_AUTH_CLIENTID", "someid")
	c.Assert(err, gocheck.IsNil)
	c.Assert("someid", gocheck.Equals, clientID())
}

func (s *S) TestPort(c *gocheck.C) {
	err := os.Setenv("TSURU_AUTH_SERVER_PORT", ":4242")
	c.Assert(err, gocheck.IsNil)
	c.Assert(":4242", gocheck.Equals, port())
	err = os.Setenv("TSURU_AUTH_SERVER_PORT", "")
	c.Assert(err, gocheck.IsNil)
	c.Assert(":0", gocheck.Equals, port())
}

func (s *S) TestOpen(c *gocheck.C) {
	fexec := etesting.FakeExecutor{}
	execut = &fexec
	defer func() {
		execut = nil
	}()
	url := "http://someurl"
	err := open(url)
	c.Assert(err, gocheck.IsNil)
	if runtime.GOOS == "linux" {
		c.Assert(fexec.ExecutedCmd("xdg-open", []string{url}), gocheck.Equals, true)
	} else {
		c.Assert(fexec.ExecutedCmd("open", []string{url}), gocheck.Equals, true)
	}
}
