// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"github.com/globocom/tsuru/cmd"
	"github.com/globocom/tsuru/cmd/tsuru-base"
	"io/ioutil"
	"launchpad.net/gocheck"
	"net/http"
	"strings"
)

func (s *S) TestAppCreateInfo(c *gocheck.C) {
	expected := &cmd.Info{
		Name:    "app-create",
		Usage:   "app-create <appname> <framework> [--units 1]",
		Desc:    "create a new app.",
		MinArgs: 2,
	}
	c.Assert((&AppCreate{}).Info(), gocheck.DeepEquals, expected)
}

func (s *S) TestAppCreate(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	result := `{"status":"success", "repository_url":"git@tsuru.plataformas.glb.com:ble.git"}`
	expected := `App "ble" is being created with 1 unit!
Use app-info to check the status of the app and its units.
Your repository for "ble" project is "git@tsuru.plataformas.glb.com:ble.git"` + "\n"
	context := cmd.Context{
		Args:   []string{"ble", "django"},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	transport := conditionalTransport{
		transport{msg: result, status: http.StatusOK},
		func(req *http.Request) bool {
			defer req.Body.Close()
			body, err := ioutil.ReadAll(req.Body)
			c.Assert(err, gocheck.IsNil)
			c.Assert(string(body), gocheck.Equals, `{"name":"ble","framework":"django","units":1}`)
			return req.Method == "POST" && req.URL.Path == "/apps"
		},
	}
	client := cmd.NewClient(&http.Client{Transport: &transport}, nil, manager)
	command := AppCreate{}
	command.Flags().Parse(true, nil)
	err := command.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, expected)
}

func (s *S) TestAppCreateMoreThanOneUnit(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	result := `{"status":"success", "repository_url":"git@tsuru.plataformas.glb.com:ble.git"}`
	expected := `App "ble" is being created with 4 units!
Use app-info to check the status of the app and its units.
Your repository for "ble" project is "git@tsuru.plataformas.glb.com:ble.git"` + "\n"
	context := cmd.Context{
		Args:   []string{"ble", "django"},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	transport := conditionalTransport{
		transport{msg: result, status: http.StatusOK},
		func(req *http.Request) bool {
			defer req.Body.Close()
			body, err := ioutil.ReadAll(req.Body)
			c.Assert(err, gocheck.IsNil)
			c.Assert(string(body), gocheck.Equals, `{"name":"ble","framework":"django","units":4}`)
			return req.Method == "POST" && req.URL.Path == "/apps"
		},
	}
	client := cmd.NewClient(&http.Client{Transport: &transport}, nil, manager)
	command := AppCreate{}
	command.Flags().Parse(true, []string{"--units", "4"})
	err := command.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, expected)
}

func (s *S) TestAppCreateZeroUnits(c *gocheck.C) {
	command := AppCreate{}
	command.Flags().Parse(true, []string{"--units", "0"})
	err := command.Run(nil, nil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "Cannot create app with zero units.")
}

func (s *S) TestAppCreateWithInvalidFramework(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	context := cmd.Context{
		Args:   []string{"invalidapp", "lombra"},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	client := cmd.NewClient(&http.Client{Transport: &transport{msg: "", status: http.StatusInternalServerError}}, nil, manager)
	command := AppCreate{}
	command.Flags().Parse(true, nil)
	err := command.Run(&context, client)
	c.Assert(err, gocheck.NotNil)
	c.Assert(stdout.String(), gocheck.Equals, "")
}

func (s *S) TestAppCreateFlags(c *gocheck.C) {
	command := AppCreate{}
	flagset := command.Flags()
	c.Assert(flagset, gocheck.NotNil)
	flagset.Parse(true, []string{"--units", "10"})
	flag := flagset.Lookup("units")
	c.Assert(flag, gocheck.NotNil)
	c.Assert(flag.Name, gocheck.Equals, "units")
	c.Assert(flag.Usage, gocheck.Equals, "How many units should be created with the app.")
	c.Assert(flag.Value.String(), gocheck.Equals, "10")
	c.Assert(flag.DefValue, gocheck.Equals, "1")
}

func (s *S) TestAppRemove(c *gocheck.C) {
	*tsuru.AppName = "ble"
	var stdout, stderr bytes.Buffer
	expected := `Are you sure you want to remove app "ble"? (y/n) App "ble" successfully removed!` + "\n"
	context := cmd.Context{
		Args:   []string{"ble"},
		Stdout: &stdout,
		Stderr: &stderr,
		Stdin:  strings.NewReader("y\n"),
	}
	client := cmd.NewClient(&http.Client{Transport: &transport{msg: "", status: http.StatusOK}}, nil, manager)
	command := AppRemove{}
	err := command.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, expected)
}

func (s *S) TestAppRemoveWithoutAsking(c *gocheck.C) {
	*tsuru.AppName = "ble"
	*AssumeYes = true
	var stdout, stderr bytes.Buffer
	expected := `App "ble" successfully removed!` + "\n"
	context := cmd.Context{
		Args:   []string{"ble"},
		Stdout: &stdout,
		Stderr: &stderr,
		Stdin:  strings.NewReader("y\n"),
	}
	client := cmd.NewClient(&http.Client{Transport: &transport{msg: "", status: http.StatusOK}}, nil, manager)
	command := AppRemove{}
	err := command.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, expected)
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
	trans := &conditionalTransport{
		transport{
			msg:    "",
			status: http.StatusOK,
		},
		func(req *http.Request) bool {
			return req.URL.Path == "/apps/secret" && req.Method == "DELETE"
		},
	}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, manager)
	fake := FakeGuesser{name: "secret"}
	guessCommand := tsuru.GuessingCommand{G: &fake}
	command := AppRemove{guessCommand}
	err := command.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, expected)
}

func (s *S) TestAppRemoveWithoutConfirmation(c *gocheck.C) {
	*tsuru.AppName = "ble"
	var stdout, stderr bytes.Buffer
	expected := `Are you sure you want to remove app "ble"? (y/n) Abort.` + "\n"
	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
		Stdin:  strings.NewReader("n\n"),
	}
	command := AppRemove{}
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
	*tsuru.AppName = "radio"
	var stdout, stderr bytes.Buffer
	var called bool
	context := cmd.Context{
		Args:   []string{"3"},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	trans := &conditionalTransport{
		transport{
			msg:    "",
			status: http.StatusOK,
		},
		func(req *http.Request) bool {
			called = true
			b, err := ioutil.ReadAll(req.Body)
			c.Assert(err, gocheck.IsNil)
			c.Assert(string(b), gocheck.Equals, "3")
			return req.URL.Path == "/apps/radio/units" && req.Method == "PUT"
		},
	}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, manager)
	command := UnitAdd{}
	err := command.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(called, gocheck.Equals, true)
	expected := "Units successfully added!\n"
	c.Assert(stdout.String(), gocheck.Equals, expected)
}

func (s *S) TestUnitAddFailure(c *gocheck.C) {
	*tsuru.AppName = "radio"
	var stdout, stderr bytes.Buffer
	context := cmd.Context{
		Args:   []string{"3"},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	client := cmd.NewClient(&http.Client{Transport: &transport{msg: "Failed to add.", status: 500}}, nil, manager)
	command := UnitAdd{}
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

func (s *S) TestUnitAddIsACommand(c *gocheck.C) {
	var _ cmd.Command = &UnitAdd{}
}

func (s *S) TestUnitRemove(c *gocheck.C) {
	*tsuru.AppName = "vapor"
	var stdout, stderr bytes.Buffer
	var called bool
	context := cmd.Context{
		Args:   []string{"2"},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	trans := &conditionalTransport{
		transport{
			msg:    "",
			status: http.StatusOK,
		},
		func(req *http.Request) bool {
			called = true
			b, err := ioutil.ReadAll(req.Body)
			c.Assert(err, gocheck.IsNil)
			c.Assert(string(b), gocheck.Equals, "2")
			return req.URL.Path == "/apps/vapor/units" && req.Method == "DELETE"
		},
	}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, manager)
	command := UnitRemove{}
	err := command.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(called, gocheck.Equals, true)
	expected := "Units successfully removed!\n"
	c.Assert(stdout.String(), gocheck.Equals, expected)
}

func (s *S) TestUnitRemoveFailure(c *gocheck.C) {
	*tsuru.AppName = "opticon"
	var stdout, stderr bytes.Buffer
	context := cmd.Context{
		Args:   []string{"1"},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	client := cmd.NewClient(&http.Client{
		Transport: &transport{msg: "Failed to remove.", status: 500},
	}, nil, manager)
	command := UnitRemove{}
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
