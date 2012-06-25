package app_cli

import (
	"bytes"
	. "launchpad.net/gocheck"
	"net/http"
)

func (s *S) TestAppList(c *C) {
	result := `[{"Name":"app1","Framework":"","State":"", "Units":[{"Ip":"10.10.10.10"}],"Teams":[{"Name":"tsuruteam","Users":[{"Email":"whydidifall@thewho.com","Password":"123","Tokens":null,"Keys":null}]}]}]`
	expected := `+-------------+-------+-------------+
| Application | State | Ip          |
+-------------+-------+-------------+
| app1        |       | 10.10.10.10 |
+-------------+-------+-------------+
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
	expected := `App "ble" successfully created!
Your repository for "ble" project is "git@tsuru.plataformas.glb.com:ble.git"` + "\n"
	context := Context{[]string{}, []string{"ble", "django"}, manager.Stdout, manager.Stderr}
	client := NewClient(&http.Client{Transport: &transport{msg: result, status: http.StatusOK}})
	command := AppCreate{}
	err := command.Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(manager.Stdout.(*bytes.Buffer).String(), Equals, expected)
}

func (s *S) TestAppRemove(c *C) {
	expected := `App "ble" successfully removed!` + "\n"
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

func (s *S) TestAppLog(c *C) {
	result := `[{"Date":"2012-06-20T11:17:22.75-03:00","Message":"creating app lost"},{"Date":"2012-06-20T11:17:22.753-03:00","Message":"app lost successfully created"}]`
	expected := `2012-06-20 11:17:22.75 -0300 BRT - creating app lost
2012-06-20 11:17:22.753 -0300 BRT - app lost successfully created
`
	context := Context{[]string{}, []string{"appName"}, manager.Stdout, manager.Stderr}
	command := AppLog{}
	client := NewClient(&http.Client{Transport: &transport{msg: result, status: http.StatusOK}})
	err := command.Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(manager.Stdout.(*bytes.Buffer).String(), Equals, expected)
}

func (s *S) TestAppLogShouldReturnNilIfHasNoContent(c *C) {
	context := Context{[]string{}, []string{"appName"}, manager.Stdout, manager.Stderr}
	command := AppLog{}
	client := NewClient(&http.Client{Transport: &transport{msg: "", status: http.StatusNoContent}})
	err := command.Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(manager.Stdout.(*bytes.Buffer).String(), Equals, "")
}

func (s *S) TestApp(c *C) {
	expect := map[string]interface{}{
		"add-team":    &AppAddTeam{},
		"remove-team": &AppRemoveTeam{},
		"create":      &AppCreate{},
		"remove":      &AppRemove{},
		"list":        &AppList{},
		"run":         &AppRun{},
		"log":         &AppLog{},
	}
	command := App{}
	c.Assert(command.Subcommands(), DeepEquals, expect)
}
