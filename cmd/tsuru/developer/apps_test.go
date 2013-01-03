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
		Usage:   "app-create <appname> <framework> [--units 1]",
		Desc:    "create a new app.",
		MinArgs: 2,
	}
	c.Assert((&AppCreate{}).Info(), DeepEquals, expected)
}

func (s *S) TestAppCreate(c *C) {
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
			c.Assert(err, IsNil)
			c.Assert(string(body), Equals, `{"name":"ble","framework":"django","units":1}`)
			return req.Method == "POST" && req.URL.Path == "/apps"
		},
	}
	client := cmd.NewClient(&http.Client{Transport: &transport}, nil, manager)
	command := AppCreate{}
	err := command.Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(stdout.String(), Equals, expected)
}

func (s *S) TestAppCreateMoreThanOneUnit(c *C) {
	*NumUnits = 4
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
			c.Assert(err, IsNil)
			c.Assert(string(body), Equals, `{"name":"ble","framework":"django","units":4}`)
			return req.Method == "POST" && req.URL.Path == "/apps"
		},
	}
	client := cmd.NewClient(&http.Client{Transport: &transport}, nil, manager)
	command := AppCreate{}
	err := command.Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(stdout.String(), Equals, expected)
}

func (s *S) TestAppCreateZeroUnits(c *C) {
	*NumUnits = 0
	command := AppCreate{}
	err := command.Run(nil, nil)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "Cannot create app with zero units.")
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

func (s *S) TestUnitAdd(c *C) {
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
	command := UnitAdd{}
	err := command.Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(called, Equals, true)
	expected := "Units successfully added!\n"
	c.Assert(stdout.String(), Equals, expected)
}

func (s *S) TestUnitAddFailure(c *C) {
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
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "Failed to add.")
}

func (s *S) TestUnitAddInfo(c *C) {
	expected := &cmd.Info{
		Name:    "unit-add",
		Usage:   "unit-add <# of units> [--app appname]",
		Desc:    "add new units to an app.",
		MinArgs: 1,
	}
	c.Assert((&UnitAdd{}).Info(), DeepEquals, expected)
}

func (s *S) TestUnitAddIsACommand(c *C) {
	var _ cmd.Command = &UnitAdd{}
}

func (s *S) TestUnitAddIsAnInfoer(c *C) {
	var _ cmd.Infoer = &UnitAdd{}
}

func (s *S) TestUnitRemove(c *C) {
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
			c.Assert(err, IsNil)
			c.Assert(string(b), Equals, "2")
			return req.URL.Path == "/apps/vapor/units" && req.Method == "DELETE"
		},
	}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, manager)
	command := UnitRemove{}
	err := command.Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(called, Equals, true)
	expected := "Units successfully removed!\n"
	c.Assert(stdout.String(), Equals, expected)
}

func (s *S) TestUnitRemoveFailure(c *C) {
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
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "Failed to remove.")
}

func (s *S) TestUnitRemoveInfo(c *C) {
	expected := cmd.Info{
		Name:    "unit-remove",
		Usage:   "unit-remove <# of units> [--app appname]",
		Desc:    "remove units from an app.",
		MinArgs: 1,
	}
	c.Assert((&UnitRemove{}).Info(), DeepEquals, &expected)
}

func (s *S) TestUnitRemoveIsACommand(c *C) {
	var _ cmd.Command = &UnitRemove{}
}

func (s *S) TestUnitRemoveIsAnInfoer(c *C) {
	var _ cmd.Infoer = &UnitRemove{}
}
