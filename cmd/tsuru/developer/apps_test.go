// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"github.com/globocom/tsuru/cmd"
	"github.com/globocom/tsuru/cmd/tsuru"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"net/http"
	"strings"
)

func (s *S) TestAppCreateInfo(c *C) {
	expected := &cmd.Info{
		Name:    "app-create",
		Usage:   "app-create <appname> <framework>",
		Desc:    "create a new app.",
		MinArgs: 2,
	}
	c.Assert((&AppCreate{}).Info(), DeepEquals, expected)
}

func (s *S) TestAppCreate(c *C) {
	var stdout, stderr bytes.Buffer
	result := `{"status":"success", "repository_url":"git@tsuru.plataformas.glb.com:ble.git"}`
	expected := `App "ble" is being created!
Check its status with app-list.
Your repository for "ble" project is "git@tsuru.plataformas.glb.com:ble.git"` + "\n"
	context := cmd.Context{
		Args:   []string{"ble", "django"},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	client := cmd.NewClient(&http.Client{Transport: &transport{msg: result, status: http.StatusOK}}, nil, manager)
	command := AppCreate{}
	err := command.Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(stdout.String(), Equals, expected)
}

func (s *S) TestAppCreateWithInvalidFramework(c *C) {
	var stdout, stderr bytes.Buffer
	context := cmd.Context{
		Args:   []string{"invalidapp", "lombra"},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	client := cmd.NewClient(&http.Client{Transport: &transport{msg: "", status: http.StatusInternalServerError}}, nil, manager)
	command := AppCreate{}
	err := command.Run(&context, client)
	c.Assert(err, NotNil)
	c.Assert(stdout.String(), Equals, "")
}

func (s *S) TestAppRemove(c *C) {
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
	c.Assert(err, IsNil)
	c.Assert(stdout.String(), Equals, expected)
}

func (s *S) TestAppRemoveWithoutAsking(c *C) {
	*tsuru.AppName = "ble"
	*tsuru.AssumeYes = true
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
	c.Assert(err, IsNil)
	c.Assert(stdout.String(), Equals, expected)
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

func (s *S) TestAppRemoveWithoutArgs(c *C) {
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
	c.Assert(err, IsNil)
	c.Assert(stdout.String(), Equals, expected)
}

func (s *S) TestAppRemoveWithoutConfirmation(c *C) {
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
	c.Assert(err, IsNil)
	c.Assert(stdout.String(), Equals, expected)
}

func (s *S) TestAppRemoveInfo(c *C) {
	expected := &cmd.Info{
		Name:  "app-remove",
		Usage: "app-remove [--app appname] [--assume-yes]",
		Desc: `removes an app.

If you don't provide the app name, tsuru will try to guess it.`,
		MinArgs: 0,
	}
	c.Assert((&AppRemove{}).Info(), DeepEquals, expected)
}

func (s *S) TestAddUnit(c *C) {
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
			c.Assert(err, IsNil)
			c.Assert(string(b), Equals, "3")
			return req.URL.Path == "/apps/radio/units" && req.Method == "PUT"
		},
	}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, manager)
	command := AddUnit{}
	err := command.Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(called, Equals, true)
	expected := "Units successfully added!\n"
	c.Assert(stdout.String(), Equals, expected)
}

func (s *S) TestAddUnitFailure(c *C) {
	*tsuru.AppName = "radio"
	var stdout, stderr bytes.Buffer
	context := cmd.Context{
		Args:   []string{"3"},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	client := cmd.NewClient(&http.Client{Transport: &transport{msg: "Failed to add.", status: 500}}, nil, manager)
	command := AddUnit{}
	err := command.Run(&context, client)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "Failed to add.")
}

func (s *S) TestAddUnitInfo(c *C) {
	expected := &cmd.Info{
		Name:    "add-unit",
		Usage:   "add-unit <# of units> [--app appname]",
		Desc:    "add new units to an app.",
		MinArgs: 1,
	}
	c.Assert((&AddUnit{}).Info(), DeepEquals, expected)
}

func (s *S) TestAddUnitIsACommand(c *C) {
	var _ cmd.Command = &AddUnit{}
}

func (s *S) TestAddUnitIsAnInfoer(c *C) {
	var _ cmd.Infoer = &AddUnit{}
}
