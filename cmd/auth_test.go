package cmd

import (
	"bytes"
	. "launchpad.net/gocheck"
	"net/http"
	"os"
	"syscall"
)

func patchStdin(c *C, content []byte) {
	f, err := os.OpenFile("/tmp/passwdfile.txt", syscall.O_RDWR|syscall.O_NDELAY|syscall.O_CREAT|syscall.O_TRUNC, 0600)
	c.Assert(err, IsNil)
	n, err := f.Write(content)
	c.Assert(err, IsNil)
	c.Assert(n, Equals, len(content))
	ret, err := f.Seek(0, 0)
	c.Assert(err, IsNil)
	c.Assert(ret, Equals, int64(0))
	os.Stdin = f
}

func unpathStdin() {
	os.Stdin = os.NewFile(uintptr(syscall.Stdin), "/dev/stdin")
}

func (s *S) TestLogin(c *C) {
	patchStdin(c, []byte("chico\n"))
	defer unpathStdin()
	expected := "Password: \nSuccessfully logged!\n"
	context := Context{[]string{}, []string{"foo@foo.com"}, manager.Stdout, manager.Stderr}
	client := NewClient(&http.Client{Transport: &transport{msg: `{"token": "sometoken"}`, status: http.StatusOK}})
	command := Login{}
	err := command.Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(manager.Stdout.(*bytes.Buffer).String(), Equals, expected)

	token, err := ReadToken()
	c.Assert(token, Equals, "sometoken")
}

func (s *S) TestLoginShouldReturnErrorIfThePasswordIsNotGiven(c *C) {
	patchStdin(c, []byte("\n"))
	defer unpathStdin()
	expected := "Password: \nYou must provide the password!\n"
	context := Context{[]string{}, []string{"foo@foo.com"}, manager.Stdout, manager.Stderr}
	command := Login{}
	err := command.Run(&context, nil)
	c.Assert(err, NotNil)
	c.Assert(manager.Stdout.(*bytes.Buffer).String(), Equals, expected)
}

func (s *S) TestAddKey(c *C) {
	expected := "Key added with success!\n"
	context := Context{[]string{}, []string{}, manager.Stdout, manager.Stderr}
	client := NewClient(&http.Client{Transport: &transport{msg: "", status: http.StatusOK}})
	command := AddKeyCommand{}
	err := command.Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(manager.Stdout.(*bytes.Buffer).String(), Equals, expected)
}

func (s *S) TestRemoveKey(c *C) {
	expected := "Key removed with success!\n"
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
	expected := `Team "core" created with success!` + "\n"
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

func (s *S) TestUserCreate(c *C) {
	patchStdin(c, []byte("bar123\n"))
	defer unpathStdin()
	expected := "Password: \n" + `User "foo@foo.com" created with success!` + "\n"
	context := Context{[]string{}, []string{"foo@foo.com"}, manager.Stdout, manager.Stderr}
	client := NewClient(&http.Client{Transport: &transport{msg: "", status: http.StatusCreated}})
	command := UserCreate{}
	err := command.Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(manager.Stdout.(*bytes.Buffer).String(), Equals, expected)
}

func (s *S) TestUserCreateShouldReturnErrorIfThePasswordIsNotGiven(c *C) {
	patchStdin(c, []byte("\n"))
	defer unpathStdin()
	expected := "Password: \nYou must provide the password!\n"
	context := Context{[]string{}, []string{"foo@foo.com"}, manager.Stdout, manager.Stderr}
	command := UserCreate{}
	err := command.Run(&context, nil)
	c.Assert(err, NotNil)
	c.Assert(manager.Stdout.(*bytes.Buffer).String(), Equals, expected)
}

func (s *S) TestGetPassword(c *C) {
	patchStdin(c, []byte("chico\n"))
	defer unpathStdin()
	pass := getPassword(os.Stdin.Fd())
	c.Assert(pass, Equals, "chico")
}

func (s *S) TestGetPasswordShouldRemoveAllNewLineCharactersFromTheEndOfThePassword(c *C) {
	patchStdin(c, []byte("chico\n\n\n"))
	defer unpathStdin()
	pass := getPassword(os.Stdin.Fd())
	c.Assert(pass, Equals, "chico")
}

func (s *S) TestGetPasswordShouldRemoveCarriageReturnCharacterFromTheEndOfThePassword(c *C) {
	patchStdin(c, []byte("opeth\r\n"))
	defer unpathStdin()
	pass := getPassword(os.Stdin.Fd())
	c.Assert(pass, Equals, "opeth")
}

func (s *S) TestGetPasswordWithEmptyPassword(c *C) {
	patchStdin(c, []byte("\n"))
	defer unpathStdin()
	pass := getPassword(os.Stdin.Fd())
	c.Assert(pass, Equals, "")
}
