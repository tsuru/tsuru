// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"fmt"
	"github.com/globocom/tsuru/cmd"
	etesting "github.com/globocom/tsuru/exec/testing"
	ftesting "github.com/globocom/tsuru/fs/testing"
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
	rfs := ftesting.RecordingFs{}
	fsystem = &rfs
	defer func() {
		fsystem = nil
	}()
	var stdout bytes.Buffer
	context := cmd.Context{
		Args:   []string{"myplugin", ts.URL},
		Stdout: &stdout,
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
	expected := `Plugin "myplugin" successfully installed!` + "\n"
	c.Assert(expected, gocheck.Equals, stdout.String())
}

func (s *S) TestPluginInstallIsACommand(c *gocheck.C) {
	var _ cmd.Command = &pluginInstall{}
}

func (s *S) TestPluginInfo(c *gocheck.C) {
	expected := &cmd.Info{
		Name:    "plugin",
		Usage:   "plugin <plugin-name> [<args>]",
		Desc:    "Execute tsuru plugins.",
		MinArgs: 1,
	}
	c.Assert(plugin{}.Info(), gocheck.DeepEquals, expected)
}

func (s *S) TestPlugin(c *gocheck.C) {
	fexec := etesting.FakeExecutor{}
	execut = &fexec
	defer func() {
		execut = nil
	}()
	context := cmd.Context{
		Args: []string{"myplugin"},
	}
	client := cmd.NewClient(nil, nil, manager)
	command := plugin{}
	err := command.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	pluginPath := cmd.JoinWithUserDir(".tsuru", "plugins", "myplugin")
	c.Assert(fexec.ExecutedCmd(pluginPath, nil), gocheck.Equals, true)
}

func (s *S) TestPluginIsACommand(c *gocheck.C) {
	var _ cmd.Command = &plugin{}
}

func (s *S) TestPluginRemoveInfo(c *gocheck.C) {
	expected := &cmd.Info{
		Name:    "plugin-remove",
		Usage:   "plugin-remove <plugin-name>",
		Desc:    "Remove tsuru plugins.",
		MinArgs: 1,
	}
	c.Assert(pluginRemove{}.Info(), gocheck.DeepEquals, expected)
}

func (s *S) TestPluginRemove(c *gocheck.C) {
	rfs := ftesting.RecordingFs{}
	fsystem = &rfs
	defer func() {
		fsystem = nil
	}()
	var stdout bytes.Buffer
	context := cmd.Context{
		Args:   []string{"myplugin"},
		Stdout: &stdout,
	}
	client := cmd.NewClient(nil, nil, manager)
	command := pluginRemove{}
	err := command.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	pluginPath := cmd.JoinWithUserDir(".tsuru", "plugins", "myplugin")
	hasAction := rfs.HasAction(fmt.Sprintf("remove %s", pluginPath))
	c.Assert(hasAction, gocheck.Equals, true)
	expected := `Plugin "myplugin" successfully removed!` + "\n"
	c.Assert(expected, gocheck.Equals, stdout.String())
}

func (s *S) TestPluginRemoveIsACommand(c *gocheck.C) {
	var _ cmd.Command = &pluginRemove{}
}

func (s *S) TestPluginListInfo(c *gocheck.C) {
	expected := &cmd.Info{
		Name:    "plugin-list",
		Usage:   "plugin-list",
		Desc:    "List installed tsuru plugins.",
		MinArgs: 0,
	}
	c.Assert(pluginList{}.Info(), gocheck.DeepEquals, expected)
}

func (s *S) TestPluginListIsACommand(c *gocheck.C) {
	var _ cmd.Command = &pluginList{}
}
