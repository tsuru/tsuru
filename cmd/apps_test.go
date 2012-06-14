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

func (s *S) TestAppRun(c *C) {
	expected := "http.go		http_test.go"
	context := Context{[]string{}, []string{"ble", "ls"}, manager.Stdout, manager.Stderr}
	trans := &conditionalTransport{
		transport{
			msg: "http.go		http_test.go",
			status: http.StatusOK,
		},
		func(req *http.Request) bool {
			b := make([]byte, 2)
			req.Body.Read(b)
			return req.URL.Path == "/apps/ble/run" && string(b) == "ls"
		},
	}
	client := NewClient(&http.Client{Transport: trans})
	err := (&AppRun{}).Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(manager.Stdout.(*bytes.Buffer).String(), Equals, expected)
}

func (s *S) TestAppRunShouldUseAllSubsequentArgumentsAsArgumentsToTheGivenCommand(c *C) {
	expected := "-rw-r--r--  1 f  staff  119 Apr 26 18:23 http.go"
	context := Context{[]string{}, []string{"ble", "ls", "-l"}, manager.Stdout, manager.Stderr}
	trans := &conditionalTransport{
		transport{
			msg:    "-rw-r--r--  1 f  staff  119 Apr 26 18:23 http.go",
			status: http.StatusOK,
		},
		func(req *http.Request) bool {
			b := make([]byte, 5)
			req.Body.Read(b)
			return req.URL.Path == "/apps/ble/run" && string(b) == "ls -l"
		},
	}
	client := NewClient(&http.Client{Transport: trans})
	err := (&AppRun{}).Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(manager.Stdout.(*bytes.Buffer).String(), Equals, expected)
}

func (s *S) TestInfoAppRun(c *C) {
	desc := `run a command in all instances of the app, and prints the output.
Notice that you may need quotes to run your command if you want to deal with
input and outputs redirects, and pipes.
`
	expected := &Info{
		Name:    "run",
		Usage:   `app run appname command commandarg1 commandarg2 ... commandargn`,
		Desc:    desc,
		MinArgs: 1,
	}
	command := AppRun{}
	c.Assert(command.Info(), DeepEquals, expected)
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
		"run":         &AppRun{},
	}
	command := App{}
	c.Assert(command.Subcommands(), DeepEquals, expect)
}
