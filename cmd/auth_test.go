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

func (s *S) TestAddUserIsSubcommandOfTeam(c *C) {
	t := Team{}
	subc, ok := t.Subcommands()["add-user"]
	c.Assert(ok, Equals, true)
	c.Assert(subc, FitsTypeOf, &TeamAddUser{})
}

func (s *S) TestRemoveUserIsASubcommandOfTeam(c *C) {
	t := Team{}
	subc, ok := t.Subcommands()["remove-user"]
	c.Assert(ok, Equals, true)
	c.Assert(subc, FitsTypeOf, &TeamRemoveUser{})
}

func (s *S) TestCreateUsASubcommandOfTeam(c *C) {
	t := Team{}
	subc, ok := t.Subcommands()["create"]
	c.Assert(ok, Equals, true)
	c.Assert(subc, FitsTypeOf, &TeamCreate{})
}

func (s *S) TestTeamAddUser(c *C) {
	expected := `User "andorito" was added to the "cobrateam" team` + "\n"
	context := Context{[]string{}, []string{"cobrateam", "andorito"}, manager.Stdout, manager.Stderr}
	command := TeamAddUser{}
	client := NewClient(&http.Client{Transport: &transport{msg: "", status: http.StatusOK}})
	err := command.Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(manager.Stdout.(*bytes.Buffer).String(), Equals, expected)
}

func (s *S) TestTeamRemoveUser(c *C) {
	expected := `User "andorito" was removed from the "cobrateam" team` + "\n"
	context := Context{[]string{}, []string{"cobrateam", "andorito"}, manager.Stdout, manager.Stderr}
	command := TeamRemoveUser{}
	client := NewClient(&http.Client{Transport: &transport{msg: "", status: http.StatusOK}})
	err := command.Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(manager.Stdout.(*bytes.Buffer).String(), Equals, expected)
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

func (s *S) TeatTeamListInfo(c *C) {
	expected := &Info{
		Name:    "list",
		Usage:   "team list",
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

func (s *S) TestTeamListIsASubCommandOfTeam(c *C) {
	t := Team{}
	subc, ok := t.Subcommands()["list"]
	c.Assert(ok, Equals, true)
	c.Assert(subc, FitsTypeOf, &TeamList{})
}

func (s *S) TestUser(c *C) {
	expect := map[string]interface{}{
		"create": &UserCreate{},
	}
	command := User{}
	c.Assert(command.Subcommands(), DeepEquals, expect)
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
