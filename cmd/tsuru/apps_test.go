package main

import (
	"bytes"
	"github.com/timeredbull/tsuru/cmd"
	. "launchpad.net/gocheck"
	"net/http"
	"strings"
)

func (s *S) TestAppList(c *C) {
	result := `[{"Name":"app1","Framework":"","State":"", "Units":[{"Ip":"10.10.10.10"}],"Teams":[{"Name":"tsuruteam","Users":[{"Email":"whydidifall@thewho.com","Password":"123","Tokens":null,"Keys":null}]}]}]`
	expected := `+-------------+-------+-------------+
| Application | State | Ip          |
+-------------+-------+-------------+
| app1        |       | 10.10.10.10 |
+-------------+-------+-------------+
`
	context := cmd.Context{
		Cmds:   []string{},
		Args:   []string{},
		Stdout: manager.Stdout,
		Stderr: manager.Stderr,
	}
	client := cmd.NewClient(&http.Client{Transport: &transport{msg: result, status: http.StatusOK}})
	command := AppList{}
	err := command.Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(manager.Stdout.(*bytes.Buffer).String(), Equals, expected)
}

func (s *S) TestAppListInfo(c *C) {
	expected := &cmd.Info{
		Name:    "app-list",
		Usage:   "app-list",
		Desc:    "list all your apps.",
		MinArgs: 0,
	}
	c.Assert((&AppList{}).Info(), DeepEquals, expected)
}

func (s *S) TestAppCreateInfo(c *C) {
	expected := &cmd.Info{
		Name:    "app-create",
		Usage:   "app-create <appname> <framework>",
		Desc:    "create a new app.",
		MinArgs: 2,
	}
	c.Assert((&AppCreate{}).Info(), DeepEquals, expected)
}

func (s *S) TestAppCreate(c *C) {
	result := `{"status":"success", "repository_url":"git@tsuru.plataformas.glb.com:ble.git"}`
	expected := `App "ble" successfully created!
Your repository for "ble" project is "git@tsuru.plataformas.glb.com:ble.git"` + "\n"
	context := cmd.Context{
		Cmds:   []string{},
		Args:   []string{"ble", "django"},
		Stdout: manager.Stdout,
		Stderr: manager.Stderr,
	}
	client := cmd.NewClient(&http.Client{Transport: &transport{msg: result, status: http.StatusOK}})
	command := AppCreate{}
	err := command.Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(manager.Stdout.(*bytes.Buffer).String(), Equals, expected)
}

func (s *S) TestAppCreateWithInvalidFramework(c *C) {
	context := cmd.Context{
		Cmds:   []string{},
		Args:   []string{"invalidapp", "lombra"},
		Stdout: manager.Stdout,
		Stderr: manager.Stderr,
	}
	client := cmd.NewClient(&http.Client{Transport: &transport{msg: "", status: http.StatusInternalServerError}})
	command := AppCreate{}
	err := command.Run(&context, client)
	c.Assert(err, NotNil)
	c.Assert(manager.Stdout.(*bytes.Buffer).String(), Equals, "")
}

func (s *S) TestAppRemove(c *C) {
	expected := `App "ble" successfully removed!` + "\n"
	context := cmd.Context{
		Cmds:   []string{},
		Args:   []string{"ble"},
		Stdout: manager.Stdout,
		Stderr: manager.Stderr,
	}
	client := cmd.NewClient(&http.Client{Transport: &transport{msg: "", status: http.StatusOK}})
	command := AppRemove{}
	err := command.Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(manager.Stdout.(*bytes.Buffer).String(), Equals, expected)
}

func (s *S) TestAppRemoveInfo(c *C) {
	expected := &cmd.Info{
		Name:    "app-remove",
		Usage:   "app-remove <appname>",
		Desc:    "removes an app.",
		MinArgs: 1,
	}
	c.Assert((&AppRemove{}).Info(), DeepEquals, expected)
}

func (s *S) TestAppGrant(c *C) {
	expected := `Team "cobrateam" was added to the "games" app` + "\n"
	context := cmd.Context{
		Cmds:   []string{},
		Args:   []string{"games", "cobrateam"},
		Stdout: manager.Stdout,
		Stderr: manager.Stderr,
	}
	command := AppGrant{}
	client := cmd.NewClient(&http.Client{Transport: &transport{msg: "", status: http.StatusOK}})
	err := command.Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(manager.Stdout.(*bytes.Buffer).String(), Equals, expected)
}

func (s *S) TestAppGrantInfo(c *C) {
	expected := &cmd.Info{
		Name:    "app-grant",
		Usage:   "app-grant <appname> <teamname>",
		Desc:    "grants access to an app to a team.",
		MinArgs: 2,
	}
	c.Assert((&AppGrant{}).Info(), DeepEquals, expected)
}

func (s *S) TestAppRevoke(c *C) {
	expected := `Team "cobrateam" was removed from the "games" app` + "\n"
	context := cmd.Context{
		Cmds:   []string{},
		Args:   []string{"games", "cobrateam"},
		Stdout: manager.Stdout,
		Stderr: manager.Stderr,
	}
	command := AppRevoke{}
	client := cmd.NewClient(&http.Client{Transport: &transport{msg: "", status: http.StatusOK}})
	err := command.Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(manager.Stdout.(*bytes.Buffer).String(), Equals, expected)
}

func (s *S) TestAppRevokeInfo(c *C) {
	expected := &cmd.Info{
		Name:    "app-revoke",
		Usage:   "app-revoke <appname> <teamname>",
		Desc:    "revokes access to an app from a team.",
		MinArgs: 2,
	}
	c.Assert((&AppRevoke{}).Info(), DeepEquals, expected)
}

func (s *S) TestAppLog(c *C) {
	result := `[{"Date":"2012-06-20T11:17:22.75-03:00","Message":"creating app lost"},{"Date":"2012-06-20T11:17:22.753-03:00","Message":"app lost successfully created"}]`
	expected := `2012-06-20 11:17:22.75 -0300 BRT - creating app lost
2012-06-20 11:17:22.753 -0300 BRT - app lost successfully created
`
	context := cmd.Context{
		Cmds:   []string{},
		Args:   []string{"appName"},
		Stdout: manager.Stdout,
		Stderr: manager.Stderr,
	}
	command := AppLog{}
	client := cmd.NewClient(&http.Client{Transport: &transport{msg: result, status: http.StatusOK}})
	err := command.Run(&context, client)
	c.Assert(err, IsNil)
	got := manager.Stdout.(*bytes.Buffer).String()
	got = strings.Replace(got, "-0300 -0300", "-0300 BRT", -1)
	c.Assert(got, Equals, expected)
}

func (s *S) TestAppLogShouldReturnNilIfHasNoContent(c *C) {
	context := cmd.Context{
		Cmds:   []string{},
		Args:   []string{"appName"},
		Stdout: manager.Stdout,
		Stderr: manager.Stderr,
	}
	command := AppLog{}
	client := cmd.NewClient(&http.Client{Transport: &transport{msg: "", status: http.StatusNoContent}})
	err := command.Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(manager.Stdout.(*bytes.Buffer).String(), Equals, "")
}

func (s *S) TestAppLogInfo(c *C) {
	expected := &cmd.Info{
		Name:    "log",
		Usage:   "log <appname>",
		Desc:    "show logs for an app.",
		MinArgs: 1,
	}
	c.Assert((&AppLog{}).Info(), DeepEquals, expected)
}

func (s *S) TestAppRestart(c *C) {
	var called bool
	context := cmd.Context{
		Cmds:   []string{},
		Args:   []string{"handful_of_nothing"},
		Stdout: manager.Stdout,
		Stderr: manager.Stderr,
	}
	trans := &conditionalTransport{
		transport{
			msg:    "Restarted",
			status: http.StatusOK,
		},
		func(req *http.Request) bool {
			called = true
			return req.URL.Path == "/apps/handful_of_nothing/restart" && req.Method == "GET"
		},
	}
	client := cmd.NewClient(&http.Client{Transport: trans})
	err := (&AppRestart{}).Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(called, Equals, true)
	c.Assert(manager.Stdout.(*bytes.Buffer).String(), Equals, "Restarted")
}

func (s *S) TestAppRestartInfo(c *C) {
	expected := &cmd.Info{
		Name:    "restart",
		Usage:   "restart <appname>",
		Desc:    "restarts an app.",
		MinArgs: 1,
	}
	c.Assert((&AppRestart{}).Info(), DeepEquals, expected)
}

func (s *S) TestAppRestartIsACommand(c *C) {
	var command cmd.Command
	c.Assert(&AppRestart{}, Implements, &command)
}

func (s *S) TestAppRestartIsAnInfoer(c *C) {
	var infoer cmd.Infoer
	c.Assert(&AppRestart{}, Implements, &infoer)
}
