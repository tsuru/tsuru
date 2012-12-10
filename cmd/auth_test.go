// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"bytes"
	"encoding/json"
	"github.com/globocom/tsuru/fs/testing"
	"io"
	. "launchpad.net/gocheck"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

func (s *S) TestLogin(c *C) {
	fsystem = &testing.RecordingFs{FileContent: "old-token"}
	defer func() {
		fsystem = nil
	}()
	expected := "Password: \nSuccessfully logged in!\n"
	reader := strings.NewReader("chico\n")
	context := Context{[]string{"foo@foo.com"}, manager.stdout, manager.stderr, reader}
	client := NewClient(&http.Client{Transport: &transport{msg: `{"token": "sometoken"}`, status: http.StatusOK}}, nil, manager)
	command := login{}
	err := command.Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(manager.stdout.(*bytes.Buffer).String(), Equals, expected)
	token, err := readToken()
	c.Assert(err, IsNil)
	c.Assert(token, Equals, "sometoken")
}

func (s *S) TestLoginShouldNotDependOnTsuruTokenFile(c *C) {
	fsystem = &testing.FailureFs{}
	defer func() {
		fsystem = nil
	}()
	expected := "Password: \nSuccessfully logged in!\n"
	reader := strings.NewReader("chico\n")
	context := Context{[]string{"foo@foo.com"}, manager.stdout, manager.stderr, reader}
	client := NewClient(&http.Client{Transport: &transport{msg: `{"token":"anothertoken"}`, status: http.StatusOK}}, nil, manager)
	command := login{}
	err := command.Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(manager.stdout.(*bytes.Buffer).String(), Equals, expected)
}

func (s *S) TestLoginShouldReturnErrorIfThePasswordIsNotGiven(c *C) {
	context := Context{[]string{"foo@foo.com"}, manager.stdout, manager.stderr, strings.NewReader("\n")}
	command := login{}
	err := command.Run(&context, nil)
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^You must provide the password!$")
}

func (s *S) TestLogout(c *C) {
	rfs := &testing.RecordingFs{}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	expected := "Successfully logged out!\n"
	context := Context{[]string{}, manager.stdout, manager.stderr, manager.stdin}
	command := logout{}
	err := command.Run(&context, nil)
	c.Assert(err, IsNil)
	c.Assert(manager.stdout.(*bytes.Buffer).String(), Equals, expected)
	tokenPath, err := joinWithUserDir(".tsuru_token")
	c.Assert(err, IsNil)
	c.Assert(rfs.HasAction("remove "+tokenPath), Equals, true)
}

func (s *S) TestLogoutWhenNotLoggedIn(c *C) {
	fsystem = &testing.FailureFs{}
	defer func() {
		fsystem = nil
	}()
	context := Context{[]string{}, manager.stdout, manager.stderr, manager.stdin}
	command := logout{}
	err := command.Run(&context, nil)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "You're not logged in!")
}

func (s *S) TestTeamAddUser(c *C) {
	expected := `User "andorito" was added to the "cobrateam" team` + "\n"
	context := Context{[]string{"cobrateam", "andorito"}, manager.stdout, manager.stderr, manager.stdin}
	command := teamUserAdd{}
	client := NewClient(&http.Client{Transport: &transport{msg: "", status: http.StatusOK}}, nil, manager)
	err := command.Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(manager.stdout.(*bytes.Buffer).String(), Equals, expected)
}

func (s *S) TestTeamAddUserInfo(c *C) {
	expected := &Info{
		Name:    "team-user-add",
		Usage:   "team-user-add <teamname> <useremail>",
		Desc:    "adds a user to a team.",
		MinArgs: 2,
	}
	c.Assert((&teamUserAdd{}).Info(), DeepEquals, expected)
}

func (s *S) TestTeamRemoveUser(c *C) {
	expected := `User "andorito" was removed from the "cobrateam" team` + "\n"
	context := Context{[]string{"cobrateam", "andorito"}, manager.stdout, manager.stderr, manager.stdin}
	command := teamUserRemove{}
	client := NewClient(&http.Client{Transport: &transport{msg: "", status: http.StatusOK}}, nil, manager)
	err := command.Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(manager.stdout.(*bytes.Buffer).String(), Equals, expected)
}

func (s *S) TestTeamRemoveUserInfo(c *C) {
	expected := &Info{
		Name:    "team-user-remove",
		Usage:   "team-user-remove <teamname> <useremail>",
		Desc:    "removes a user from a team.",
		MinArgs: 2,
	}
	c.Assert((&teamUserRemove{}).Info(), DeepEquals, expected)
}

func (s *S) TestTeamCreate(c *C) {
	expected := `Team "core" successfully created!` + "\n"
	context := Context{[]string{"core"}, manager.stdout, manager.stderr, manager.stdin}
	client := NewClient(&http.Client{Transport: &transport{msg: "", status: http.StatusCreated}}, nil, manager)
	command := teamCreate{}
	err := command.Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(manager.stdout.(*bytes.Buffer).String(), Equals, expected)
}

func (s *S) TestTeamCreateInfo(c *C) {
	expected := &Info{
		Name:    "team-create",
		Usage:   "team-create <teamname>",
		Desc:    "creates a new team.",
		MinArgs: 1,
	}
	c.Assert((&teamCreate{}).Info(), DeepEquals, expected)
}

func (s *S) TestTeamRemove(c *C) {
	var (
		buf    bytes.Buffer
		called bool
	)
	context := Context{
		Args:   []string{"evergrey"},
		Stdout: &buf,
		Stdin:  strings.NewReader("y\n"),
	}
	trans := conditionalTransport{
		transport{
			msg:    "",
			status: http.StatusOK,
		},
		func(req *http.Request) bool {
			called = true
			return req.URL.Path == "/teams/evergrey" && req.Method == "DELETE"
		},
	}
	client := NewClient(&http.Client{Transport: &trans}, nil, manager)
	command := teamRemove{}
	err := command.Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(called, Equals, true)
	c.Assert(buf.String(), Equals, `Are you sure you want to remove team "evergrey"? (y/n) Team "evergrey" successfully removed!`+"\n")
}

func (s *S) TestTeamRemoveWithouConfirmation(c *C) {
	var buf bytes.Buffer
	context := Context{
		Args:   []string{"dream-theater"},
		Stdout: &buf,
		Stdin:  strings.NewReader("n\n"),
	}
	command := teamRemove{}
	err := command.Run(&context, nil)
	c.Assert(err, IsNil)
	c.Assert(buf.String(), Equals, `Are you sure you want to remove team "dream-theater"? (y/n) Abort.`+"\n")
}

func (s *S) TestTeamRemoveFailingRequest(c *C) {
	context := Context{
		Args:   []string{"evergrey"},
		Stdout: new(bytes.Buffer),
		Stdin:  strings.NewReader("y\n"),
	}
	client := NewClient(&http.Client{Transport: &transport{msg: "Team evergrey not found.", status: http.StatusNotFound}}, nil, manager)
	command := teamRemove{}
	err := command.Run(&context, client)
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^Team evergrey not found.$")
}

func (s *S) TestTeamRemoveInfo(c *C) {
	expected := &Info{
		Name:    "team-remove",
		Usage:   "team-remove <team-name>",
		Desc:    "removes a team from tsuru server.",
		MinArgs: 1,
	}
	c.Assert((&teamRemove{}).Info(), DeepEquals, expected)
}

func (s *S) TestTeamRemoveIsACommand(c *C) {
	var _ Command = &teamRemove{}
}

func (s *S) TestTeamRemoveIsAnInfoer(c *C) {
	var _ Infoer = &teamRemove{}
}

func (s *S) TestTeamListRun(c *C) {
	var called bool
	trans := &conditionalTransport{
		transport{
			msg:    `[{"name":"timeredbull"},{"name":"cobrateam"}]`,
			status: http.StatusOK,
		},
		func(req *http.Request) bool {
			called = true
			return req.Method == "GET" && req.URL.Path == "/teams"
		},
	}
	expected := `Teams:

  - timeredbull
  - cobrateam
`
	client := NewClient(&http.Client{Transport: trans}, nil, manager)
	err := (&teamList{}).Run(&Context{[]string{}, manager.stdout, manager.stderr, manager.stdin}, client)
	c.Assert(err, IsNil)
	c.Assert(called, Equals, true)
	c.Assert(manager.stdout.(*bytes.Buffer).String(), Equals, expected)
}

func (s *S) TestTeamListRunWithNoContent(c *C) {
	client := NewClient(&http.Client{Transport: &transport{msg: "", status: http.StatusNoContent}}, nil, manager)
	err := (&teamList{}).Run(&Context{[]string{}, manager.stdout, manager.stderr, manager.stdin}, client)
	c.Assert(err, IsNil)
	c.Assert(manager.stdout.(*bytes.Buffer).String(), Equals, "")
}

func (s *S) TestTeamListInfo(c *C) {
	expected := &Info{
		Name:    "team-list",
		Usage:   "team-list",
		Desc:    "List all teams that you are member.",
		MinArgs: 0,
	}
	c.Assert((&teamList{}).Info(), DeepEquals, expected)
}

func (s *S) TestTeamListIsACommand(c *C) {
	var command Command
	c.Assert(&teamList{}, Implements, &command)
}

func (s *S) TeamTeamListIsAnInfoer(c *C) {
	var infoer Infoer
	c.Assert(&teamList{}, Implements, &infoer)
}

func (s *S) TestUserCreateShouldNotDependOnTsuruTokenFile(c *C) {
	fsystem = &testing.FailureFs{}
	defer func() {
		fsystem = nil
	}()
	expected := "Password: \nConfirm: \n" + `User "foo@foo.com" successfully created!` + "\n"
	reader := strings.NewReader("foo123\nfoo123\n")
	context := Context{[]string{"foo@foo.com"}, manager.stdout, manager.stderr, reader}
	client := NewClient(&http.Client{Transport: &transport{msg: "", status: http.StatusCreated}}, nil, manager)
	command := userCreate{}
	err := command.Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(manager.stdout.(*bytes.Buffer).String(), Equals, expected)
}

func (s *S) TestUserCreateReturnErrorIfPasswordsDontMatch(c *C) {
	reader := strings.NewReader("foo123\nfoo1234\n")
	context := Context{[]string{"foo@foo.com"}, manager.stdout, manager.stderr, reader}
	client := NewClient(&http.Client{Transport: &transport{msg: "", status: http.StatusCreated}}, nil, manager)
	command := userCreate{}
	err := command.Run(&context, client)
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^Passwords didn't match.$")
}

func (s *S) TestUserCreate(c *C) {
	expected := "Password: \nConfirm: \n" + `User "foo@foo.com" successfully created!` + "\n"
	context := Context{[]string{"foo@foo.com"}, manager.stdout, manager.stderr, strings.NewReader("foo123\nfoo123\n")}
	client := NewClient(&http.Client{Transport: &transport{msg: "", status: http.StatusCreated}}, nil, manager)
	command := userCreate{}
	err := command.Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(manager.stdout.(*bytes.Buffer).String(), Equals, expected)
}

func (s *S) TestUserCreateShouldReturnErrorIfThePasswordIsNotGiven(c *C) {
	context := Context{[]string{"foo@foo.com"}, manager.stdout, manager.stderr, strings.NewReader("")}
	command := userCreate{}
	err := command.Run(&context, nil)
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^You must provide the password!$")
}

func (s *S) TestUserCreateInfo(c *C) {
	expected := &Info{
		Name:    "user-create",
		Usage:   "user-create <email>",
		Desc:    "creates a user.",
		MinArgs: 1,
	}
	c.Assert((&userCreate{}).Info(), DeepEquals, expected)
}

func (s *S) TestUserRemove(c *C) {
	var (
		buf    bytes.Buffer
		called bool
	)
	context := Context{
		Stdout: &buf,
		Stdin:  strings.NewReader("y\n"),
	}
	trans := conditionalTransport{
		transport{
			msg:    "",
			status: http.StatusOK,
		},
		func(req *http.Request) bool {
			called = true
			return req.Method == "DELETE" && req.URL.Path == "/users"
		},
	}
	client := NewClient(&http.Client{Transport: &trans}, nil, manager)
	command := userRemove{}
	err := command.Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(called, Equals, true)
	c.Assert(buf.String(), Equals, "Are you sure you want to remove your user from tsuru? (y/n) User successfully removed.\n")
}

func (s *S) TestUserRemoveWithoutConfirmation(c *C) {
	var buf bytes.Buffer
	context := Context{
		Stdout: &buf,
		Stdin:  strings.NewReader("n\n"),
	}
	command := userRemove{}
	err := command.Run(&context, nil)
	c.Assert(err, IsNil)
	c.Assert(buf.String(), Equals, "Are you sure you want to remove your user from tsuru? (y/n) Abort.\n")
}

func (s *S) TestUserRemoveWithRequestError(c *C) {
	client := NewClient(&http.Client{Transport: &transport{msg: "User not found.", status: http.StatusNotFound}}, nil, manager)
	command := userRemove{}
	err := command.Run(&Context{Stdout: new(bytes.Buffer), Stdin: strings.NewReader("y\n")}, client)
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^User not found.$")
}

func (s *S) TestUserRemoveInfo(c *C) {
	expected := &Info{
		Name:    "user-remove",
		Usage:   "user-remove",
		Desc:    "removes your user from tsuru server.",
		MinArgs: 0,
	}
	c.Assert((&userRemove{}).Info(), DeepEquals, expected)
}

func (s *S) TestUserRemoveIsACommand(c *C) {
	var cmd Command
	c.Assert(&userRemove{}, Implements, &cmd)
}

func (s *S) TestUserRemoveIsAnInfoer(c *C) {
	var infoer Infoer
	c.Assert(&userRemove{}, Implements, &infoer)
}

func (s *S) TestChangePassword(c *C) {
	var (
		buf    bytes.Buffer
		called bool
		stdin  io.Reader
	)
	stdin = strings.NewReader("gopher\nbbrothers\nbbrothers\n")
	context := Context{
		Stdout: &buf,
		Stdin:  stdin,
	}
	trans := conditionalTransport{
		transport{
			msg:    "",
			status: http.StatusOK,
		},
		func(req *http.Request) bool {
			var got map[string]string
			called = true
			if err := json.NewDecoder(req.Body).Decode(&got); err != nil {
				return false
			}
			cond := got["old"] == "gopher" && got["new"] == "bbrothers"
			return cond && req.Method == "PUT" && req.URL.Path == "/users/password"
		},
	}
	client := NewClient(&http.Client{Transport: &trans}, nil, manager)
	command := changePassword{}
	err := command.Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(called, Equals, true)
	expected := "Current password: \nNew password: \nConfirm: \nPassword successfully updated!\n"
	c.Assert(buf.String(), Equals, expected)
}

func (s *S) TestChangePasswordWrongConfirmation(c *C) {
	var buf bytes.Buffer
	stdin := strings.NewReader("gopher\nblood\nsugar\n")
	context := Context{
		Stdin:  stdin,
		Stdout: &buf,
		Stderr: &buf,
	}
	command := changePassword{}
	err := command.Run(&context, nil)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "New password and password confirmation didn't match.")
}

func (s *S) TestChangePasswordInfo(c *C) {
	expected := Info{
		Name:  "change-password",
		Usage: "change-password",
		Desc:  "Change your password.",
	}
	command := changePassword{}
	c.Assert(command.Info(), DeepEquals, &expected)
}

func (s *S) TestChangePasswordIsACommand(c *C) {
	var _ Command = &changePassword{}
}

func (s *S) TestChangePasswordIsAnInfoer(c *C) {
	var _ Infoer = &changePassword{}
}

func (s *S) TestPasswordFromReaderUsingFile(c *C) {
	tmpdir, err := filepath.EvalSymlinks(os.TempDir())
	filename := path.Join(tmpdir, "password-reader.txt")
	c.Assert(err, IsNil)
	file, err := os.Create(filename)
	c.Assert(err, IsNil)
	defer os.Remove(filename)
	file.WriteString("hello")
	file.Seek(0, 0)
	password, err := passwordFromReader(file)
	c.Assert(err, IsNil)
	c.Assert(password, Equals, "hello")
}

func (s *S) TestPasswordFromReaderUsingStringsReader(c *C) {
	reader := strings.NewReader("abcd\n")
	password, err := passwordFromReader(reader)
	c.Assert(err, IsNil)
	c.Assert(password, Equals, "abcd")
}
