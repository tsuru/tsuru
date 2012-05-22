package cmd

import (
	"bytes"
	. "launchpad.net/gocheck"
	"net/http"
)

func (s *S) TestAppList(c *C) {
	result := `[{"Ip":"","Name":"app1","Framework":"","State":"","Teams":[{"Name":"tsuruteam","Users":[{"Email":"whydidifall@thewho.com","Password":"123","Tokens":null,"Keys":null}]}]}]`
	expected := `+-------------+-------+----+
| Application | State | Ip |
+-------------+-------+----+
| app1        |       |    |
+-------------+-------+----+
`
	context := Context{[]string{}, []string{}, manager.Stdout, manager.Stderr}
	client := NewClient(&http.Client{Transport: &transport{msg: result, status: http.StatusOK}})
	command := AppList{}
	err := command.Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(manager.Stdout.(*bytes.Buffer).String(), Equals, expected)
}

func (s *S) TestAppCreate(c *C) {
	result := `{"status":"success", "repository_url":"git@tsuru.plataformas.glb.com:ble.git"}`
	expected := `App "ble" created with success!
Your repository for "ble" project is "git@tsuru.plataformas.glb.com:ble.git"` + "\n"
	context := Context{[]string{}, []string{"ble", "django"}, manager.Stdout, manager.Stderr}
	client := NewClient(&http.Client{Transport: &transport{msg: result, status: http.StatusOK}})
	command := AppCreate{}
	err := command.Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(manager.Stdout.(*bytes.Buffer).String(), Equals, expected)
}

func (s *S) TestAppRemove(c *C) {
	expected := `App "ble" removed with success!` + "\n"
	context := Context{[]string{}, []string{"ble"}, manager.Stdout, manager.Stderr}
	client := NewClient(&http.Client{Transport: &transport{msg: "", status: http.StatusOK}})
	command := AppRemove{}
	err := command.Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(manager.Stdout.(*bytes.Buffer).String(), Equals, expected)
}

func (s *S) TestAppAddTeam(c *C) {
	expected := `Team "cobrateam" was added to the "games" app` + "\n"
	context := Context{[]string{}, []string{"games", "cobrateam"}, manager.Stdout, manager.Stderr}
	command := AppAddTeam{}
	client := NewClient(&http.Client{Transport: &transport{msg: "", status: http.StatusOK}})
	err := command.Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(manager.Stdout.(*bytes.Buffer).String(), Equals, expected)
}

func (s *S) TestAppRemoveTeam(c *C) {
	expected := `Team "cobrateam" was removed from the "games" app` + "\n"
	context := Context{[]string{}, []string{"games", "cobrateam"}, manager.Stdout, manager.Stderr}
	command := AppRemoveTeam{}
	client := NewClient(&http.Client{Transport: &transport{msg: "", status: http.StatusOK}})
	err := command.Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(manager.Stdout.(*bytes.Buffer).String(), Equals, expected)
}

func (s *S) TestApp(c *C) {
	expect := map[string]interface{}{
		"add-team":    &AppAddTeam{},
		"remove-team": &AppRemoveTeam{},
		"create":      &AppCreate{},
		"remove":      &AppRemove{},
		"list":        &AppList{},
	}
	command := App{}
	c.Assert(command.Subcommands(), DeepEquals, expect)
}
