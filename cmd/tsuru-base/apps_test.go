// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tsuru

import (
	"bytes"
	"encoding/json"
	"github.com/globocom/tsuru/cmd"
	"github.com/globocom/tsuru/cmd/testing"
	"launchpad.net/gnuflag"
	"launchpad.net/gocheck"
	"net/http"
)

var appflag = &gnuflag.Flag{
	Name:     "app",
	Usage:    "The name of the app.",
	Value:    nil,
	DefValue: "",
}

var appshortflag = &gnuflag.Flag{
	Name:     "a",
	Usage:    "The name of the app.",
	Value:    nil,
	DefValue: "",
}

func (s *S) TestAppInfo(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	result := `{"name":"app1","cname":"","ip":"myapp.tsuru.io","platform":"php","repository":"git@git.com:php.git","state":"dead", "units":[{"Ip":"10.10.10.10","Name":"app1/0","State":"started"}, {"Ip":"9.9.9.9","Name":"app1/1","State":"started"}, {"Ip":"","Name":"app1/2","State":"pending"}],"teams":["tsuruteam","crane"]}`
	expected := `Application: app1
Repository: git@git.com:php.git
Platform: php
Teams: tsuruteam, crane
Address: myapp.tsuru.io
Units:
+--------+---------+
| Unit   | State   |
+--------+---------+
| app1/0 | started |
| app1/1 | started |
| app1/2 | pending |
+--------+---------+

`
	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
	}
	client := cmd.NewClient(&http.Client{Transport: &testing.Transport{Message: result, Status: http.StatusOK}}, nil, manager)
	command := AppInfo{}
	command.Flags().Parse(true, []string{"--app", "app1"})
	err := command.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, expected)
}

func (s *S) TestAppInfoNoUnits(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	result := `{"name":"app1","ip":"app1.tsuru.io","platform":"php","repository":"git@git.com:php.git","state":"dead","units":[],"teams":["tsuruteam","crane"]}`
	expected := `Application: app1
Repository: git@git.com:php.git
Platform: php
Teams: tsuruteam, crane
Address: app1.tsuru.io

`
	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
	}
	client := cmd.NewClient(&http.Client{Transport: &testing.Transport{Message: result, Status: http.StatusOK}}, nil, manager)
	command := AppInfo{}
	command.Flags().Parse(true, []string{"--app", "app1"})
	err := command.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, expected)
}

func (s *S) TestAppInfoEmptyUnit(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	result := `{"name":"app1","cname":"","ip":"myapp.tsuru.io","platform":"php","repository":"git@git.com:php.git","state":"dead", "units":[{"Name":"","State":""}],"teams":["tsuruteam","crane"]}`
	expected := `Application: app1
Repository: git@git.com:php.git
Platform: php
Teams: tsuruteam, crane
Address: myapp.tsuru.io

`
	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
	}
	client := cmd.NewClient(&http.Client{Transport: &testing.Transport{Message: result, Status: http.StatusOK}}, nil, manager)
	command := AppInfo{}
	command.Flags().Parse(true, []string{"--app", "app1"})
	err := command.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, expected)
}

func (s *S) TestAppInfoWithoutArgs(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	result := `{"name":"secret","ip":"secret.tsuru.io","platform":"ruby","repository":"git@git.com:php.git","state":"dead","units":[{"Ip":"10.10.10.10","Name":"secret/0","State":"started"}, {"Ip":"9.9.9.9","Name":"secret/1","State":"pending"}],"Teams":["tsuruteam","crane"]}`
	expected := `Application: secret
Repository: git@git.com:php.git
Platform: ruby
Teams: tsuruteam, crane
Address: secret.tsuru.io
Units:
+----------+---------+
| Unit     | State   |
+----------+---------+
| secret/0 | started |
| secret/1 | pending |
+----------+---------+

`
	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
	}
	trans := &testing.ConditionalTransport{
		Transport: testing.Transport{Message: result, Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			return req.URL.Path == "/apps/secret" && req.Method == "GET"
		},
	}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, manager)
	fake := FakeGuesser{name: "secret"}
	guessCommand := GuessingCommand{G: &fake}
	command := AppInfo{GuessingCommand: guessCommand}
	command.Flags().Parse(true, nil)
	err := command.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, expected)
}

func (s *S) TestAppInfoCName(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	result := `{"name":"app1","ip":"myapp.tsuru.io","cname":"yourapp.tsuru.io","platform":"php","repository":"git@git.com:php.git","state":"dead","units":[{"Ip":"10.10.10.10","Name":"app1/0","State":"started"}, {"Ip":"9.9.9.9","Name":"app1/1","State":"started"}, {"Ip":"","Name":"app1/2","State":"pending"}],"Teams":["tsuruteam","crane"]}`
	expected := `Application: app1
Repository: git@git.com:php.git
Platform: php
Teams: tsuruteam, crane
Address: yourapp.tsuru.io
Units:
+--------+---------+
| Unit   | State   |
+--------+---------+
| app1/0 | started |
| app1/1 | started |
| app1/2 | pending |
+--------+---------+

`
	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
	}
	client := cmd.NewClient(&http.Client{Transport: &testing.Transport{Message: result, Status: http.StatusOK}}, nil, manager)
	command := AppInfo{}
	command.Flags().Parse(true, []string{"--app", "app1"})
	err := command.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, expected)
}

func (s *S) TestAppInfoInfo(c *gocheck.C) {
	expected := &cmd.Info{
		Name:  "app-info",
		Usage: "app-info [--app appname]",
		Desc: `show information about your app.

If you don't provide the app name, tsuru will try to guess it.`,
		MinArgs: 0,
	}
	c.Assert((&AppInfo{}).Info(), gocheck.DeepEquals, expected)
}

func (s *S) TestAppInfoFlags(c *gocheck.C) {
	command := AppInfo{}
	flagset := command.Flags()
	flag := flagset.Lookup("app")
	flag.Value = nil
	c.Assert(flag, gocheck.DeepEquals, appflag)
}

func (s *S) TestAppGrant(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	expected := `Team "cobrateam" was added to the "games" app` + "\n"
	context := cmd.Context{
		Args:   []string{"cobrateam"},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	command := AppGrant{}
	command.Flags().Parse(true, []string{"--app", "games"})
	client := cmd.NewClient(&http.Client{Transport: &testing.Transport{Message: "", Status: http.StatusOK}}, nil, manager)
	err := command.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, expected)
}

func (s *S) TestAppGrantWithoutFlag(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	expected := `Team "cobrateam" was added to the "fights" app` + "\n"
	context := cmd.Context{
		Args:   []string{"cobrateam"},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	fake := &FakeGuesser{name: "fights"}
	command := AppGrant{GuessingCommand: GuessingCommand{G: fake}}
	command.Flags().Parse(true, nil)
	client := cmd.NewClient(&http.Client{Transport: &testing.Transport{Message: "", Status: http.StatusOK}}, nil, manager)
	err := command.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, expected)
}

func (s *S) TestAppGrantInfo(c *gocheck.C) {
	expected := &cmd.Info{
		Name:  "app-grant",
		Usage: "app-grant <teamname> [--app appname]",
		Desc: `grants access to an app to a team.

If you don't provide the app name, tsuru will try to guess it.`,
		MinArgs: 1,
	}
	c.Assert((&AppGrant{}).Info(), gocheck.DeepEquals, expected)
}

func (s *S) TestAppRevoke(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	expected := `Team "cobrateam" was removed from the "games" app` + "\n"
	context := cmd.Context{
		Args:   []string{"cobrateam"},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	command := AppRevoke{}
	command.Flags().Parse(true, []string{"--app", "games"})
	client := cmd.NewClient(&http.Client{Transport: &testing.Transport{Message: "", Status: http.StatusOK}}, nil, manager)
	err := command.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, expected)
}

func (s *S) TestAppRevokeWithoutFlag(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	expected := `Team "cobrateam" was removed from the "fights" app` + "\n"
	context := cmd.Context{
		Args:   []string{"cobrateam"},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	fake := &FakeGuesser{name: "fights"}
	command := AppRevoke{GuessingCommand: GuessingCommand{G: fake}}
	command.Flags().Parse(true, nil)
	client := cmd.NewClient(&http.Client{Transport: &testing.Transport{Message: "", Status: http.StatusOK}}, nil, manager)
	err := command.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, expected)
}

func (s *S) TestAppRevokeInfo(c *gocheck.C) {
	expected := &cmd.Info{
		Name:  "app-revoke",
		Usage: "app-revoke <teamname> [--app appname]",
		Desc: `revokes access to an app from a team.

If you don't provide the app name, tsuru will try to guess it.`,
		MinArgs: 1,
	}
	c.Assert((&AppRevoke{}).Info(), gocheck.DeepEquals, expected)
}

func (s *S) TestAppList(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	result := `[{"ip":"10.10.10.10","name":"app1","ready":true,"units":[{"Name":"app1/0","State":"started"}]}]`
	expected := `+-------------+-------------------------+-------------+--------+
| Application | Units State Summary     | Address     | Ready? |
+-------------+-------------------------+-------------+--------+
| app1        | 1 of 1 units in-service | 10.10.10.10 | Yes    |
+-------------+-------------------------+-------------+--------+
`
	context := cmd.Context{
		Args:   []string{},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	client := cmd.NewClient(&http.Client{Transport: &testing.Transport{Message: result, Status: http.StatusOK}}, nil, manager)
	command := AppList{}
	err := command.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, expected)
}

func (s *S) TestAppListDisplayAppsInAlphabeticalOrder(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	result := `[{"ip":"10.10.10.11","name":"sapp","ready":true,"units":[{"Name":"sapp1/0","State":"started"}]},{"ip":"10.10.10.10","name":"app1","ready":true,"units":[{"Name":"app1/0","State":"started"}]}]`
	expected := `+-------------+-------------------------+-------------+--------+
| Application | Units State Summary     | Address     | Ready? |
+-------------+-------------------------+-------------+--------+
| app1        | 1 of 1 units in-service | 10.10.10.10 | Yes    |
| sapp        | 1 of 1 units in-service | 10.10.10.11 | Yes    |
+-------------+-------------------------+-------------+--------+
`
	context := cmd.Context{
		Args:   []string{},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	client := cmd.NewClient(&http.Client{Transport: &testing.Transport{Message: result, Status: http.StatusOK}}, nil, manager)
	command := AppList{}
	err := command.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, expected)
}

func (s *S) TestAppListUnitIsntStarted(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	result := `[{"ip":"10.10.10.10","name":"app1","ready":true,"units":[{"Name":"app1/0","State":"pending"}]}]`
	expected := `+-------------+-------------------------+-------------+--------+
| Application | Units State Summary     | Address     | Ready? |
+-------------+-------------------------+-------------+--------+
| app1        | 0 of 1 units in-service | 10.10.10.10 | Yes    |
+-------------+-------------------------+-------------+--------+
`
	context := cmd.Context{
		Args:   []string{},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	client := cmd.NewClient(&http.Client{Transport: &testing.Transport{Message: result, Status: http.StatusOK}}, nil, manager)
	command := AppList{}
	err := command.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, expected)
}

func (s *S) TestAppListUnitWithoutName(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	result := `[{"ip":"10.10.10.10","name":"app1","ready":true,"units":[{"Name":"","State":"pending"}]}]`
	expected := `+-------------+-------------------------+-------------+--------+
| Application | Units State Summary     | Address     | Ready? |
+-------------+-------------------------+-------------+--------+
| app1        | 0 of 0 units in-service | 10.10.10.10 | Yes    |
+-------------+-------------------------+-------------+--------+
`
	context := cmd.Context{
		Args:   []string{},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	client := cmd.NewClient(&http.Client{Transport: &testing.Transport{Message: result, Status: http.StatusOK}}, nil, manager)
	command := AppList{}
	err := command.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, expected)
}

func (s *S) TestAppListNotReady(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	result := `[{"ip":"10.10.10.10","name":"app1","ready":false,"units":[{"Name":"","State":"pending"}]}]`
	expected := `+-------------+-------------------------+-------------+--------+
| Application | Units State Summary     | Address     | Ready? |
+-------------+-------------------------+-------------+--------+
| app1        | 0 of 0 units in-service | 10.10.10.10 | No     |
+-------------+-------------------------+-------------+--------+
`
	context := cmd.Context{
		Args:   []string{},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	client := cmd.NewClient(&http.Client{Transport: &testing.Transport{Message: result, Status: http.StatusOK}}, nil, manager)
	command := AppList{}
	err := command.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, expected)
}

func (s *S) TestAppListCName(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	result := `[{"ip":"10.10.10.10","cname":"app1.tsuru.io","name":"app1","ready":true,"units":[{"Name":"app1/0","State":"started"}]}]`
	expected := `+-------------+-------------------------+---------------+--------+
| Application | Units State Summary     | Address       | Ready? |
+-------------+-------------------------+---------------+--------+
| app1        | 1 of 1 units in-service | app1.tsuru.io | Yes    |
+-------------+-------------------------+---------------+--------+
`
	context := cmd.Context{
		Args:   []string{},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	client := cmd.NewClient(&http.Client{Transport: &testing.Transport{Message: result, Status: http.StatusOK}}, nil, manager)
	command := AppList{}
	err := command.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, expected)
}

func (s *S) TestAppListInfo(c *gocheck.C) {
	expected := &cmd.Info{
		Name:    "app-list",
		Usage:   "app-list",
		Desc:    "list all your apps.",
		MinArgs: 0,
	}
	c.Assert(AppList{}.Info(), gocheck.DeepEquals, expected)
}

func (s *S) TestAppListIsACommand(c *gocheck.C) {
	var _ cmd.Command = AppList{}
}

func (s *S) TestAppRestart(c *gocheck.C) {
	var (
		called         bool
		stdout, stderr bytes.Buffer
	)
	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
	}
	trans := &testing.ConditionalTransport{
		Transport: testing.Transport{Message: "Restarted", Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			called = true
			return req.URL.Path == "/apps/handful_of_nothing/restart" && req.Method == "GET"
		},
	}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, manager)
	command := AppRestart{}
	command.Flags().Parse(true, []string{"--app", "handful_of_nothing"})
	err := command.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(called, gocheck.Equals, true)
	c.Assert(stdout.String(), gocheck.Equals, "Restarted")
}

func (s *S) TestAppRestartWithoutTheFlag(c *gocheck.C) {
	var (
		called         bool
		stdout, stderr bytes.Buffer
	)
	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
	}
	trans := &testing.ConditionalTransport{
		Transport: testing.Transport{Message: "Restarted", Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			called = true
			return req.URL.Path == "/apps/motorbreath/restart" && req.Method == "GET"
		},
	}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, manager)
	fake := &FakeGuesser{name: "motorbreath"}
	command := AppRestart{GuessingCommand: GuessingCommand{G: fake}}
	command.Flags().Parse(true, nil)
	err := command.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(called, gocheck.Equals, true)
	c.Assert(stdout.String(), gocheck.Equals, "Restarted")
}

func (s *S) TestAppRestartInfo(c *gocheck.C) {
	expected := &cmd.Info{
		Name:  "restart",
		Usage: "restart [--app appname]",
		Desc: `restarts an app.

If you don't provide the app name, tsuru will try to guess it.`,
		MinArgs: 0,
	}
	c.Assert((&AppRestart{}).Info(), gocheck.DeepEquals, expected)
}

func (s *S) TestAppRestartIsAFlaggedCommand(c *gocheck.C) {
	var _ cmd.FlaggedCommand = &AppRestart{}
}

func (s *S) TestSetCName(c *gocheck.C) {
	var (
		called         bool
		stdout, stderr bytes.Buffer
	)
	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
		Args:   []string{"death.evergrey.mycompany.com"},
	}
	trans := &testing.ConditionalTransport{
		Transport: testing.Transport{Message: "Restarted", Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			called = true
			var m map[string]string
			err := json.NewDecoder(req.Body).Decode(&m)
			c.Assert(err, gocheck.IsNil)
			return req.URL.Path == "/apps/death/cname" &&
				req.Method == "POST" &&
				m["cname"] == "death.evergrey.mycompany.com"
		},
	}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, manager)
	command := SetCName{}
	command.Flags().Parse(true, []string{"-a", "death"})
	err := command.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(called, gocheck.Equals, true)
	c.Assert(stdout.String(), gocheck.Equals, "cname successfully defined.\n")
}

func (s *S) TestSetCNameWithoutTheFlag(c *gocheck.C) {
	var (
		called         bool
		stdout, stderr bytes.Buffer
	)
	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
		Args:   []string{"corey.evergrey.mycompany.com"},
	}
	fake := &FakeGuesser{name: "corey"}
	trans := &testing.ConditionalTransport{
		Transport: testing.Transport{Message: "Restarted", Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			called = true
			var m map[string]string
			err := json.NewDecoder(req.Body).Decode(&m)
			c.Assert(err, gocheck.IsNil)
			return req.URL.Path == "/apps/corey/cname" &&
				req.Method == "POST" &&
				m["cname"] == "corey.evergrey.mycompany.com"
		},
	}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, manager)
	err := (&SetCName{GuessingCommand{G: fake}}).Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(called, gocheck.Equals, true)
	c.Assert(stdout.String(), gocheck.Equals, "cname successfully defined.\n")
}

func (s *S) TestSetCNameFailure(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
		Args:   []string{"masterplan.evergrey.mycompany.com"},
	}
	trans := &testing.Transport{Message: "Invalid cname", Status: http.StatusPreconditionFailed}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, manager)
	command := SetCName{}
	command.Flags().Parse(true, []string{"-a", "masterplan"})
	err := command.Run(&context, client)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "Invalid cname")
}

func (s *S) TestSetCNameInfo(c *gocheck.C) {
	expected := &cmd.Info{
		Name:    "set-cname",
		Usage:   "set-cname <cname> [--app appname]",
		Desc:    `defines a cname for your app.`,
		MinArgs: 1,
	}
	c.Assert((&SetCName{}).Info(), gocheck.DeepEquals, expected)
}

func (s *S) TestSetCNameIsAFlaggedCommand(c *gocheck.C) {
	var _ cmd.FlaggedCommand = &SetCName{}
}

func (s *S) TestUnsetCName(c *gocheck.C) {
	var (
		called         bool
		stdout, stderr bytes.Buffer
	)
	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
	}
	trans := &testing.ConditionalTransport{
		Transport: testing.Transport{Message: "Restarted", Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			called = true
			return req.URL.Path == "/apps/death/cname" && req.Method == "DELETE"
		},
	}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, manager)
	command := UnsetCName{}
	command.Flags().Parse(true, []string{"--app", "death"})
	err := command.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(called, gocheck.Equals, true)
	c.Assert(stdout.String(), gocheck.Equals, "cname successfully undefined.\n")
}

func (s *S) TestUnsetCNameWithoutTheFlag(c *gocheck.C) {
	var (
		called         bool
		stdout, stderr bytes.Buffer
	)
	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
	}
	fake := &FakeGuesser{name: "corey"}
	trans := &testing.ConditionalTransport{
		Transport: testing.Transport{Message: "Restarted", Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			called = true
			return req.URL.Path == "/apps/corey/cname" && req.Method == "DELETE"
		},
	}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, manager)
	err := (&UnsetCName{GuessingCommand{G: fake}}).Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(called, gocheck.Equals, true)
	c.Assert(stdout.String(), gocheck.Equals, "cname successfully undefined.\n")
}

func (s *S) TestUnsetCNameInfo(c *gocheck.C) {
	expected := &cmd.Info{
		Name:    "unset-cname",
		Usage:   "unset-cname [--app appname]",
		Desc:    `unsets the current cname of your app.`,
		MinArgs: 0,
	}
	c.Assert((&UnsetCName{}).Info(), gocheck.DeepEquals, expected)
}

func (s *S) TestUnsetCNameIsAFlaggedCommand(c *gocheck.C) {
	var _ cmd.FlaggedCommand = &UnsetCName{}
}

func (s *S) TestAppStartInfo(c *gocheck.C) {
	expected := &cmd.Info{
		Name:  "start",
		Usage: "start [--app appname]",
		Desc: `starts an app.

If you don't provide the app name, tsuru will try to guess it.`,
		MinArgs: 0,
	}
	c.Assert((&AppStart{}).Info(), gocheck.DeepEquals, expected)
}
