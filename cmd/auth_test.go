package main

import (
	"bytes"
	. "launchpad.net/gocheck"
	"net/http"
	"os"
)

func (s *S) TestLogin(c *C) {
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
	os.Remove(os.ExpandEnv("${HOME}/.tsuru_token"))
	s.patchStdin(c, []byte("bar123\n"))
	defer s.unpatchStdin()
	expected := "Password: \n" + `Successfully logged!` + "\n"
	context := Context{[]string{}, []string{"foo@foo.com"}, manager.Stdout, manager.Stderr}
	client := NewClient(&http.Client{Transport: &transport{msg: `{"token":"sometoken"}`, status: http.StatusOK}})
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

func (s *S) TestAddKey(c *C) {
	expected := "Key successfully added!\n"
	context := Context{[]string{}, []string{}, manager.Stdout, manager.Stderr}
	client := NewClient(&http.Client{Transport: &transport{msg: "", status: http.StatusOK}})
	command := AddKeyCommand{}
	err := command.Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(manager.Stdout.(*bytes.Buffer).String(), Equals, expected)
}

func (s *S) TestRemoveKey(c *C) {
	expected := "Key successfully removed!\n"
	context := Context{[]string{}, []string{}, manager.Stdout, manager.Stderr}
	client := NewClient(&http.Client{Transport: &transport{msg: "", status: http.StatusOK}})
	command := RemoveKey{}
	err := command.Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(manager.Stdout.(*bytes.Buffer).String(), Equals, expected)
}

func (s *S) TestKey(c *C) {
	expect := map[string]interface{}{
		"add":    &AddKeyCommand{},
		"remove": &RemoveKey{},
	}
	command := Key{}
	c.Assert(command.Subcommands(), DeepEquals, expect)
}

func (s *S) TestLogout(c *C) {
	expected := "Successfully logout!\n"
	context := Context{[]string{}, []string{}, manager.Stdout, manager.Stderr}
	command := Logout{}
	err := command.Run(&context, nil)
	c.Assert(err, IsNil)
	c.Assert(manager.Stdout.(*bytes.Buffer).String(), Equals, expected)

	token, err := ReadToken()
	c.Assert(token, Equals, "")
}

func (s *S) TestTeam(c *C) {
	expect := map[string]interface{}{
		"add-user":    &TeamAddUser{},
		"remove-user": &TeamRemoveUser{},
		"create":      &TeamCreate{},
	}
	command := Team{}
	c.Assert(command.Subcommands(), DeepEquals, expect)
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

func (s *S) TestUser(c *C) {
	expect := map[string]interface{}{
		"create": &UserCreate{},
	}
	command := User{}
	c.Assert(command.Subcommands(), DeepEquals, expect)
}

func (s *S) TestUserCreateShouldNotDependOnTsuruTokenFile(c *C) {
	os.Remove(os.ExpandEnv("${HOME}/.tsuru_token"))
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
