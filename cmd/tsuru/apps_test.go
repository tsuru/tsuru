// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"github.com/globocom/tsuru/cmd"
	"github.com/globocom/tsuru/cmd/testing"
	"github.com/globocom/tsuru/cmd/tsuru-base"
	"io/ioutil"
	"launchpad.net/gocheck"
	"net/http"
	"strings"
)

func (s *S) TestAppCreateInfo(c *gocheck.C) {
	expected := &cmd.Info{
		Name:    "app-create",
		Usage:   "app-create <appname> <platform> [--memory/-m memory_in_mb] [--swap/-s swap_in_mb]",
		Desc:    "create a new app.",
		MinArgs: 2,
	}
	c.Assert((&AppCreate{}).Info(), gocheck.DeepEquals, expected)
}

func (s *S) TestAppCreate(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	result := `{"status":"success", "repository_url":"git@tsuru.plataformas.glb.com:ble.git"}`
	expected := `App "ble" is being created!
Use app-info to check the status of the app and its units.
Your repository for "ble" project is "git@tsuru.plataformas.glb.com:ble.git"` + "\n"
	context := cmd.Context{
		Args:   []string{"ble", "django"},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	trans := testing.ConditionalTransport{
		Transport: testing.Transport{Message: result, Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			defer req.Body.Close()
			body, err := ioutil.ReadAll(req.Body)
			c.Assert(err, gocheck.IsNil)
			c.Assert(string(body), gocheck.Equals, `{"name":"ble","platform":"django","memory":"0","swap":"0"}`)
			return req.Method == "POST" && req.URL.Path == "/apps"
		},
	}
	client := cmd.NewClient(&http.Client{Transport: &trans}, nil, manager)
	command := AppCreate{}
	err := command.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, expected)
}

func (s *S) TestAppCreateWithInvalidFramework(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	context := cmd.Context{
		Args:   []string{"invalidapp", "lombra"},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	client := cmd.NewClient(&http.Client{Transport: &testing.Transport{Message: "", Status: http.StatusInternalServerError}}, nil, manager)
	command := AppCreate{}
	err := command.Run(&context, client)
	c.Assert(err, gocheck.NotNil)
	c.Assert(stdout.String(), gocheck.Equals, "")
}

func (s *S) TestAppCreateFlags(c *gocheck.C) {
	command := AppCreate{}
	flagset := command.Flags()
	c.Assert(flagset, gocheck.NotNil)
	flagset.Parse(true, []string{"-m", "10"})
	memory := flagset.Lookup("memory")
	usage := "The maximum amount of memory reserved to each container for this app"
	c.Check(memory, gocheck.NotNil)
	c.Check(memory.Name, gocheck.Equals, "memory")
	c.Check(memory.Usage, gocheck.Equals, usage)
	c.Check(memory.Value.String(), gocheck.Equals, "10")
	c.Check(memory.DefValue, gocheck.Equals, "0")
	smemory := flagset.Lookup("m")
	c.Check(smemory, gocheck.NotNil)
	c.Check(smemory.Name, gocheck.Equals, "m")
	c.Check(smemory.Usage, gocheck.Equals, usage)
	c.Check(smemory.Value.String(), gocheck.Equals, "10")
	c.Check(smemory.DefValue, gocheck.Equals, "0")
	swap := flagset.Lookup("swap")
	usage = "The maximum amount of swap reserved to each container for this app"
	c.Check(swap, gocheck.NotNil)
	c.Check(swap.Name, gocheck.Equals, "swap")
	c.Check(swap.Usage, gocheck.Equals, usage)
	c.Check(swap.Value.String(), gocheck.Equals, "10")
	c.Check(swap.DefValue, gocheck.Equals, "0")
	sswap := flagset.Lookup("s")
	c.Check(sswap, gocheck.NotNil)
	c.Check(sswap.Name, gocheck.Equals, "m")
	c.Check(sswap.Usage, gocheck.Equals, usage)
	c.Check(sswap.Value.String(), gocheck.Equals, "10")
	c.Check(sswap.DefValue, gocheck.Equals, "0")
}

func (s *S) TestAppRemove(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	expected := `Are you sure you want to remove app "ble"? (y/n) App "ble" successfully removed!` + "\n"
	context := cmd.Context{
		Args:   []string{"ble"},
		Stdout: &stdout,
		Stderr: &stderr,
		Stdin:  strings.NewReader("y\n"),
	}
	client := cmd.NewClient(&http.Client{Transport: &testing.Transport{Message: "", Status: http.StatusOK}}, nil, manager)
	command := AppRemove{}
	command.Flags().Parse(true, []string{"-a", "ble"})
	err := command.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, expected)
}

func (s *S) TestAppRemoveWithoutAsking(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	expected := `App "ble" successfully removed!` + "\n"
	context := cmd.Context{
		Args:   []string{"ble"},
		Stdout: &stdout,
		Stderr: &stderr,
		Stdin:  strings.NewReader("y\n"),
	}
	client := cmd.NewClient(&http.Client{Transport: &testing.Transport{Message: "", Status: http.StatusOK}}, nil, manager)
	command := AppRemove{}
	command.Flags().Parse(true, []string{"-a", "ble", "-y"})
	err := command.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, expected)
}

func (s *S) TestAppRemoveFlags(c *gocheck.C) {
	command := AppRemove{}
	flagset := command.Flags()
	c.Assert(flagset, gocheck.NotNil)
	flagset.Parse(true, []string{"-a", "ashamed", "-y"})
	app := flagset.Lookup("app")
	c.Check(app, gocheck.NotNil)
	c.Check(app.Name, gocheck.Equals, "app")
	c.Check(app.Usage, gocheck.Equals, "The name of the app.")
	c.Check(app.Value.String(), gocheck.Equals, "ashamed")
	c.Check(app.DefValue, gocheck.Equals, "")
	sapp := flagset.Lookup("a")
	c.Check(sapp, gocheck.NotNil)
	c.Check(sapp.Name, gocheck.Equals, "a")
	c.Check(sapp.Usage, gocheck.Equals, "The name of the app.")
	c.Check(sapp.Value.String(), gocheck.Equals, "ashamed")
	c.Check(sapp.DefValue, gocheck.Equals, "")
	assume := flagset.Lookup("assume-yes")
	c.Check(assume, gocheck.NotNil)
	c.Check(assume.Name, gocheck.Equals, "assume-yes")
	c.Check(assume.Usage, gocheck.Equals, "Don't ask for confirmation, just remove the app.")
	c.Check(assume.Value.String(), gocheck.Equals, "true")
	c.Check(assume.DefValue, gocheck.Equals, "false")
	sassume := flagset.Lookup("y")
	c.Check(sassume, gocheck.NotNil)
	c.Check(sassume.Name, gocheck.Equals, "y")
	c.Check(sassume.Usage, gocheck.Equals, "Don't ask for confirmation, just remove the app.")
	c.Check(sassume.Value.String(), gocheck.Equals, "true")
	c.Check(sassume.DefValue, gocheck.Equals, "false")
	c.Check(command.yes, gocheck.Equals, true)
}

type FakeGuesser struct {
	guesses []string
	name    string
}

func (f *FakeGuesser) HasGuess(path string) bool {
	for _, g := range f.guesses {
		if g == path {
			return true
		}
	}
	return false
}

func (f *FakeGuesser) GuessName(path string) (string, error) {
	f.guesses = append(f.guesses, path)
	return f.name, nil
}

func (s *S) TestAppRemoveWithoutArgs(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	context := cmd.Context{
		Args:   nil,
		Stdout: &stdout,
		Stderr: &stderr,
		Stdin:  strings.NewReader("y\n"),
	}
	expected := `Are you sure you want to remove app "secret"? (y/n) App "secret" successfully removed!` + "\n"
	trans := &testing.ConditionalTransport{
		Transport: testing.Transport{Message: "", Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			return req.URL.Path == "/apps/secret" && req.Method == "DELETE"
		},
	}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, manager)
	fake := FakeGuesser{name: "secret"}
	guessCommand := tsuru.GuessingCommand{G: &fake}
	command := AppRemove{GuessingCommand: guessCommand}
	err := command.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, expected)
}

func (s *S) TestAppRemoveWithoutConfirmation(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	expected := `Are you sure you want to remove app "ble"? (y/n) Abort.` + "\n"
	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
		Stdin:  strings.NewReader("n\n"),
	}
	command := AppRemove{}
	command.Flags().Parse(true, []string{"--app", "ble"})
	err := command.Run(&context, nil)
	c.Assert(err, gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, expected)
}

func (s *S) TestAppRemoveInfo(c *gocheck.C) {
	expected := &cmd.Info{
		Name:  "app-remove",
		Usage: "app-remove [--app appname] [--assume-yes]",
		Desc: `removes an app.

If you don't provide the app name, tsuru will try to guess it.`,
		MinArgs: 0,
	}
	c.Assert((&AppRemove{}).Info(), gocheck.DeepEquals, expected)
}

func (s *S) TestUnitAdd(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	var called bool
	context := cmd.Context{
		Args:   []string{"3"},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	trans := &testing.ConditionalTransport{
		Transport: testing.Transport{Message: "", Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			called = true
			b, err := ioutil.ReadAll(req.Body)
			c.Assert(err, gocheck.IsNil)
			c.Assert(string(b), gocheck.Equals, "3")
			return req.URL.Path == "/apps/radio/units" && req.Method == "PUT"
		},
	}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, manager)
	command := UnitAdd{}
	command.Flags().Parse(true, []string{"-a", "radio"})
	err := command.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(called, gocheck.Equals, true)
	expected := "Units successfully added!\n"
	c.Assert(stdout.String(), gocheck.Equals, expected)
}

func (s *S) TestUnitAddFailure(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	context := cmd.Context{
		Args:   []string{"3"},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	client := cmd.NewClient(&http.Client{Transport: &testing.Transport{Message: "Failed to add.", Status: 500}}, nil, manager)
	command := UnitAdd{}
	command.Flags().Parse(true, []string{"-a", "radio"})
	err := command.Run(&context, client)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "Failed to add.")
}

func (s *S) TestUnitAddInfo(c *gocheck.C) {
	expected := &cmd.Info{
		Name:    "unit-add",
		Usage:   "unit-add <# of units> [--app appname]",
		Desc:    "add new units to an app.",
		MinArgs: 1,
	}
	c.Assert((&UnitAdd{}).Info(), gocheck.DeepEquals, expected)
}

func (s *S) TestUnitAddIsFlaggedACommand(c *gocheck.C) {
	var _ cmd.FlaggedCommand = &UnitAdd{}
}

func (s *S) TestUnitRemove(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	var called bool
	context := cmd.Context{
		Args:   []string{"2"},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	trans := &testing.ConditionalTransport{
		Transport: testing.Transport{Message: "", Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			called = true
			b, err := ioutil.ReadAll(req.Body)
			c.Assert(err, gocheck.IsNil)
			c.Assert(string(b), gocheck.Equals, "2")
			return req.URL.Path == "/apps/vapor/units" && req.Method == "DELETE"
		},
	}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, manager)
	command := UnitRemove{}
	command.Flags().Parse(true, []string{"-a", "vapor"})
	err := command.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(called, gocheck.Equals, true)
	expected := "Units successfully removed!\n"
	c.Assert(stdout.String(), gocheck.Equals, expected)
}

func (s *S) TestUnitRemoveFailure(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	context := cmd.Context{
		Args:   []string{"1"},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	client := cmd.NewClient(&http.Client{
		Transport: &testing.Transport{Message: "Failed to remove.", Status: 500},
	}, nil, manager)
	command := UnitRemove{}
	command.Flags().Parse(true, []string{"-a", "vapor"})
	err := command.Run(&context, client)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "Failed to remove.")
}

func (s *S) TestUnitRemoveInfo(c *gocheck.C) {
	expected := cmd.Info{
		Name:    "unit-remove",
		Usage:   "unit-remove <# of units> [--app appname]",
		Desc:    "remove units from an app.",
		MinArgs: 1,
	}
	c.Assert((&UnitRemove{}).Info(), gocheck.DeepEquals, &expected)
}

func (s *S) TestUnitRemoveIsACommand(c *gocheck.C) {
	var _ cmd.Command = &UnitRemove{}
}
