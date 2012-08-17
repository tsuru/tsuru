package cmd

import (
	"bytes"
	"github.com/timeredbull/tsuru/fs/testing"
	. "launchpad.net/gocheck"
	"net/http"
)

func (s *S) TestLogin(c *C) {
	fsystem = &testing.RecordingFs{FileContent: "old-token"}
	defer func() {
		fsystem = nil
	}()
	s.patchStdin(c, []byte("chico\n"))
	defer s.unpatchStdin()
	expected := "Password: \nSuccessfully logged!\n"
	context := Context{[]string{}, []string{"foo@foo.com"}, manager.Stdout, manager.Stderr}
	client := NewClient(&http.Client{Transport: &transport{msg: `{"token": "sometoken"}`, status: http.StatusOK}})
	command := Login{}
	err := command.Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(manager.Stdout.(*bytes.Buffer).String(), Equals, expected)
	token, err := ReadToken()
	c.Assert(err, IsNil)
	c.Assert(token, Equals, "sometoken")
}

func (s *S) TestLoginShouldNotDependOnTsuruTokenFile(c *C) {
	fsystem = &testing.FailureFs{}
	defer func() {
		fsystem = nil
	}()
	s.patchStdin(c, []byte("bar123\n"))
	defer s.unpatchStdin()
	expected := "Password: \n" + `Successfully logged!` + "\n"
	context := Context{[]string{}, []string{"foo@foo.com"}, manager.Stdout, manager.Stderr}
	client := NewClient(&http.Client{Transport: &transport{msg: `{"token":"anothertoken"}`, status: http.StatusOK}})
	command := Login{}
	err := command.Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(manager.Stdout.(*bytes.Buffer).String(), Equals, expected)
}

func (s *S) TestLoginShouldReturnErrorIfThePasswordIsNotGiven(c *C) {
	s.patchStdin(c, []byte("\n"))
	defer s.unpatchStdin()
	expected := "Password: \nYou must provide the password!\n"
	context := Context{[]string{}, []string{"foo@foo.com"}, manager.Stdout, manager.Stderr}
	command := Login{}
	err := command.Run(&context, nil)
	c.Assert(err, NotNil)
	c.Assert(manager.Stdout.(*bytes.Buffer).String(), Equals, expected)
}

func (s *S) TestLogout(c *C) {
	rfs := &testing.RecordingFs{}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	expected := "Successfully logout!\n"
	context := Context{[]string{}, []string{}, manager.Stdout, manager.Stderr}
	command := Logout{}
	err := command.Run(&context, nil)
	c.Assert(err, IsNil)
	c.Assert(manager.Stdout.(*bytes.Buffer).String(), Equals, expected)
	tokenPath, err := joinWithUserDir(".tsuru_token")
	c.Assert(err, IsNil)
	c.Assert(rfs.HasAction("remove "+tokenPath), Equals, true)
}

func (s *S) TestLogoutWhenNotLoggedIn(c *C) {
	fsystem = &testing.FailureFs{}
	defer func() {
		fsystem = nil
	}()
	context := Context{[]string{}, []string{}, manager.Stdout, manager.Stderr}
	command := Logout{}
	err := command.Run(&context, nil)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "You're not logged in!")
}

func (s *S) TestTeamAddUser(c *C) {
	expected := `User "andorito" was added to the "cobrateam" team` + "\n"
	context := Context{[]string{}, []string{"cobrateam", "andorito"}, manager.Stdout, manager.Stderr}
	command := TeamUserAdd{}
	client := NewClient(&http.Client{Transport: &transport{msg: "", status: http.StatusOK}})
	err := command.Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(manager.Stdout.(*bytes.Buffer).String(), Equals, expected)
}

func (s *S) TestTeamAddUserInfo(c *C) {
	expected := &Info{
		Name:    "team-user-add",
		Usage:   "team-user-add <teamname> <useremail>",
		Desc:    "adds a user to a team.",
		MinArgs: 2,
	}
	c.Assert((&TeamUserAdd{}).Info(), DeepEquals, expected)
}

func (s *S) TestTeamRemoveUser(c *C) {
	expected := `User "andorito" was removed from the "cobrateam" team` + "\n"
	context := Context{[]string{}, []string{"cobrateam", "andorito"}, manager.Stdout, manager.Stderr}
	command := TeamUserRemove{}
	client := NewClient(&http.Client{Transport: &transport{msg: "", status: http.StatusOK}})
	err := command.Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(manager.Stdout.(*bytes.Buffer).String(), Equals, expected)
}

func (s *S) TestTeamRemoveUserInfo(c *C) {
	expected := &Info{
		Name:    "team-user-remove",
		Usage:   "team-user-remove <teamname> <useremail>",
		Desc:    "removes a user from a team.",
		MinArgs: 2,
	}
	c.Assert((&TeamUserRemove{}).Info(), DeepEquals, expected)
}

func (s *S) TestTeamCreate(c *C) {
	expected := `Team "core" successfully created!` + "\n"
	context := Context{[]string{}, []string{"core"}, manager.Stdout, manager.Stderr}
	client := NewClient(&http.Client{Transport: &transport{msg: "", status: http.StatusCreated}})
	command := TeamCreate{}
	err := command.Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(manager.Stdout.(*bytes.Buffer).String(), Equals, expected)
}

func (s *S) TestTeamCreateInfo(c *C) {
	expected := &Info{
		Name:    "team-create",
		Usage:   "team-create <teamname>",
		Desc:    "creates a new team.",
		MinArgs: 1,
	}
	c.Assert((&TeamCreate{}).Info(), DeepEquals, expected)
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
	client := NewClient(&http.Client{Transport: trans})
	err := (&TeamList{}).Run(&Context{[]string{}, []string{}, manager.Stdout, manager.Stderr}, client)
	c.Assert(err, IsNil)
	c.Assert(called, Equals, true)
	c.Assert(manager.Stdout.(*bytes.Buffer).String(), Equals, expected)
}

func (s *S) TestTeamListRunWithNoContent(c *C) {
	client := NewClient(&http.Client{Transport: &transport{msg: "", status: http.StatusNoContent}})
	err := (&TeamList{}).Run(&Context{[]string{}, []string{}, manager.Stdout, manager.Stderr}, client)
	c.Assert(err, IsNil)
	c.Assert(manager.Stdout.(*bytes.Buffer).String(), Equals, "")
}

func (s *S) TestTeamListInfo(c *C) {
	expected := &Info{
		Name:    "team-list",
		Usage:   "team-list",
		Desc:    "List all teams that you are member.",
		MinArgs: 0,
	}
	c.Assert((&TeamList{}).Info(), DeepEquals, expected)
}

func (s *S) TestTeamListIsACommand(c *C) {
	var command Command
	c.Assert(&TeamList{}, Implements, &command)
}

func (s *S) TeamTeamListIsAnInfoer(c *C) {
	var infoer Infoer
	c.Assert(&TeamList{}, Implements, &infoer)
}

func (s *S) TestUserCreateShouldNotDependOnTsuruTokenFile(c *C) {
	fsystem = &testing.FailureFs{}
	defer func() {
		fsystem = nil
	}()
	s.patchStdin(c, []byte("bar123\n"))
	defer s.unpatchStdin()
	expected := "Password: \n" + `User "foo@foo.com" successfully created!` + "\n"
	context := Context{[]string{}, []string{"foo@foo.com"}, manager.Stdout, manager.Stderr}
	client := NewClient(&http.Client{Transport: &transport{msg: "", status: http.StatusCreated}})
	command := UserCreate{}
	err := command.Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(manager.Stdout.(*bytes.Buffer).String(), Equals, expected)
}

func (s *S) TestUserCreate(c *C) {
	s.patchStdin(c, []byte("bar123\n"))
	defer s.unpatchStdin()
	expected := "Password: \n" + `User "foo@foo.com" successfully created!` + "\n"
	context := Context{[]string{}, []string{"foo@foo.com"}, manager.Stdout, manager.Stderr}
	client := NewClient(&http.Client{Transport: &transport{msg: "", status: http.StatusCreated}})
	command := UserCreate{}
	err := command.Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(manager.Stdout.(*bytes.Buffer).String(), Equals, expected)
}

func (s *S) TestUserCreateShouldReturnErrorIfThePasswordIsNotGiven(c *C) {
	s.patchStdin(c, []byte("\n"))
	defer s.unpatchStdin()
	expected := "Password: \nYou must provide the password!\n"
	context := Context{[]string{}, []string{"foo@foo.com"}, manager.Stdout, manager.Stderr}
	command := UserCreate{}
	err := command.Run(&context, nil)
	c.Assert(err, NotNil)
	c.Assert(manager.Stdout.(*bytes.Buffer).String(), Equals, expected)
}

func (s *S) TestUserCreateInfo(c *C) {
	expected := &Info{
		Name:    "user-create",
		Usage:   "user-create <email>",
		Desc:    "creates a user.",
		MinArgs: 1,
	}
	c.Assert((&UserCreate{}).Info(), DeepEquals, expected)
}
