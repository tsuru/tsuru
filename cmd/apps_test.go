package cmd

import (
	"bytes"
	. "launchpad.net/gocheck"
	"net/http"
)

func (s *S) TestAppsRun(c *C) {
	result := `[{"Ip":"","Name":"app1","Framework":"","State":"","Teams":[{"Name":"tsuruteam","Users":[{"Email":"whydidifall@thewho.com","Password":"123","Tokens":null,"Keys":null}]}]}]`
	expected := `+-------------+-------+----+
| Application | State | Ip |
+-------------+-------+----+
| app1        |       |    |
+-------------+-------+----+`
	context := Context{[]string{}, manager.Stdout, manager.Stderr}
	client := NewClient(&http.Client{Transport: &transport{msg: result, status: http.StatusOK}})
	command := AppsCommand{}
	err := command.Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(manager.Stdout.(*bytes.Buffer).String(), Equals, expected)
}

func (s *S) TestShowApps(c *C) {
	result := []byte(`[{"Ip":"","Name":"app1","Framework":"","State":"","Teams":[{"Name":"tsuruteam","Users":[{"Email":"whydidifall@thewho.com","Password":"123","Tokens":null,"Keys":null}]}]}]`)
	expected := `+-------------+-------+----+
| Application | State | Ip |
+-------------+-------+----+
| app1        |       |    |
+-------------+-------+----+`
	context := Context{[]string{}, manager.Stdout, manager.Stderr}
	err := AppsCommand{}.Show(result, &context)
	c.Assert(err, IsNil)
	c.Assert(manager.Stdout.(*bytes.Buffer).String(), Equals, expected)
}

func (s *S) TestCreateApp(c *C) {
	result := `{"status":"success", "repository_url":"git@tsuru.plataformas.glb.com:ble.git"}`
	expected := `Creating application: ble
Your repository for "ble" project is "git@tsuru.plataformas.glb.com:ble.git"
Ok!`
	context := Context{[]string{"ble", "django"}, manager.Stdout, manager.Stderr}
	client := NewClient(&http.Client{Transport: &transport{msg: result, status: http.StatusOK}})
	command := CreateAppCommand{}
	err := command.Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(manager.Stdout.(*bytes.Buffer).String(), Equals, expected)
}

func (s *S) TestDeleteApp(c *C) {
	expected := "App ble delete with success!"
	context := Context{[]string{"ble"}, manager.Stdout, manager.Stderr}
	client := NewClient(&http.Client{Transport: &transport{msg: "", status: http.StatusOK}})
	command := DeleteAppCommand{}
	err := command.Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(manager.Stdout.(*bytes.Buffer).String(), Equals, expected)
}

func (s *S) TestAppAddTeam(c *C) {
	expected := `Team "cobrateam" was added to the "games" app` + "\n"
	context := Context{[]string{"games", "cobrateam"}, manager.Stdout, manager.Stderr}
	command := AppAddTeam{}
	client := NewClient(&http.Client{Transport: &transport{msg: "", status: http.StatusOK}})
	err := command.Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(manager.Stdout.(*bytes.Buffer).String(), Equals, expected)
}

func (s *S) TestAppRemoveTeam(c *C) {
	expected := `Team "cobrateam" was removed from the "games" app` + "\n"
	context := Context{[]string{"games", "cobrateam"}, manager.Stdout, manager.Stderr}
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
	}
	command := App{}
	c.Assert(command.Subcommands(), DeepEquals, expect)
}
