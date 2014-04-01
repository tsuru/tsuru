// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"github.com/globocom/tsuru/cmd"
	"github.com/globocom/tsuru/fs/testing"
	"io/ioutil"
	"launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
)

func (s *S) TestPluginInstallInfo(c *gocheck.C) {
	expected := &cmd.Info{
		Name:    "plugin-install",
		Usage:   "plugin-install <plugin-name> <plugin-url>",
		Desc:    "Install tsuru plugins.",
		MinArgs: 2,
	}
	c.Assert(pluginInstall{}.Info(), gocheck.DeepEquals, expected)
}

func (s *S) TestPluginInstall(c *gocheck.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "fakeplugin")
	}))
	defer ts.Close()
	rfs := testing.RecordingFs{}
	fsystem = &rfs
	defer func() {
		fsystem = nil
	}()
	context := cmd.Context{
		Args: []string{"myplugin", ts.URL},
	}
	client := cmd.NewClient(nil, nil, manager)
	command := pluginInstall{}
	err := command.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	pluginsPath := cmd.JoinWithUserDir(".tsuru", "plugins")
	hasAction := rfs.HasAction(fmt.Sprintf("mkdirall %s with mode 0755", pluginsPath))
	c.Assert(hasAction, gocheck.Equals, true)
	pluginPath := cmd.JoinWithUserDir(".tsuru", "plugins", "myplugin")
	hasAction = rfs.HasAction(fmt.Sprintf("openfile %s with mode 0755", pluginPath))
	c.Assert(hasAction, gocheck.Equals, true)
	f, err := rfs.Open(pluginPath)
	c.Assert(err, gocheck.IsNil)
	data, err := ioutil.ReadAll(f)
	c.Assert(err, gocheck.IsNil)
	c.Assert("fakeplugin\n", gocheck.Equals, string(data))
}
