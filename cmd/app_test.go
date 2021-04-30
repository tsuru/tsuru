// Copyright 2021 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"github.com/tsuru/gnuflag"
	check "gopkg.in/check.v1"
)

var appflag = &gnuflag.Flag{
	Name:     "app",
	Usage:    "The name of the app.",
	Value:    nil,
	DefValue: "",
}

var appshortflag = &gnuflag.Flag{
	Name:     "a",
	Usage:    "The name of the app.",
	Value:    nil,
	DefValue: "",
}

func (s *S) TestAppNameMixInWithFlagDefined(c *check.C) {
	g := AppNameMixIn{}
	g.Flags().Parse(true, []string{"--app", "myapp"})
	name, err := g.AppName()
	c.Assert(err, check.IsNil)
	c.Assert(name, check.Equals, "myapp")
}

func (s *S) TestAppNameMixInWithShortFlagDefined(c *check.C) {
	g := AppNameMixIn{}
	g.Flags().Parse(true, []string{"-a", "myapp"})
	name, err := g.AppName()
	c.Assert(err, check.IsNil)
	c.Assert(name, check.Equals, "myapp")
}

func (s *S) TestAppNameMixInWithoutFlagDefinedFails(c *check.C) {
	g := AppNameMixIn{}
	name, err := g.AppName()
	c.Assert(name, check.Equals, "")
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, `The name of the app is required.

Use the --app flag to specify it.

`)
}

func (s *S) TestAppNameMixInFlags(c *check.C) {
	var flags []gnuflag.Flag
	expected := []gnuflag.Flag{*appshortflag, *appflag}
	command := AppNameMixIn{}
	flagset := command.Flags()
	flagset.VisitAll(func(f *gnuflag.Flag) {
		f.Value = nil
		flags = append(flags, *f)
	})
	c.Assert(flags, check.DeepEquals, expected)
}
