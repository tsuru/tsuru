// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tsuru

import (
	"bytes"
	"encoding/json"
	"net/http"

	"github.com/tsuru/tsuru/cmd"
	"github.com/tsuru/tsuru/cmd/testing"
	"launchpad.net/gnuflag"
	"launchpad.net/gocheck"
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
	result := `{"name":"app1","teamowner":"myteam","cname":[""],"ip":"myapp.tsuru.io","platform":"php","repository":"git@git.com:php.git","state":"dead", "units":[{"Ip":"10.10.10.10","Name":"app1/0","Status":"started"}, {"Ip":"9.9.9.9","Name":"app1/1","Status":"started"}, {"Ip":"","Name":"app1/2","Status":"pending"}],"teams":["tsuruteam","crane"], "owner": "myapp_owner", "deploys": 7}`
	expected := `Application: app1
Repository: git@git.com:php.git
Platform: php
Teams: tsuruteam, crane
Address: myapp.tsuru.io
Owner: myapp_owner
Team owner: myteam
Deploys: 7
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
	result := `{"name":"app1","ip":"app1.tsuru.io","teamowner":"myteam","platform":"php","repository":"git@git.com:php.git","state":"dead","units":[],"teams":["tsuruteam","crane"], "owner": "myapp_owner", "deploys": 7}`
	expected := `Application: app1
Repository: git@git.com:php.git
Platform: php
Teams: tsuruteam, crane
Address: app1.tsuru.io
Owner: myapp_owner
Team owner: myteam
Deploys: 7

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
	result := `{"name":"app1","teamowner":"","cname":[""],"ip":"myapp.tsuru.io","platform":"php","repository":"git@git.com:php.git","state":"dead", "units":[{"Name":"","Status":""}],"teams":["tsuruteam","crane"], "owner": "myapp_owner", "deploys": 7}`
	expected := `Application: app1
Repository: git@git.com:php.git
Platform: php
Teams: tsuruteam, crane
Address: myapp.tsuru.io
Owner: myapp_owner
Team owner: 
Deploys: 7

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
	result := `{"name":"secret","teamowner":"myteam","ip":"secret.tsuru.io","platform":"ruby","repository":"git@git.com:php.git","state":"dead","units":[{"Ip":"10.10.10.10","Name":"secret/0","Status":"started"}, {"Ip":"9.9.9.9","Name":"secret/1","Status":"pending"}],"Teams":["tsuruteam","crane"], "owner": "myapp_owner", "deploys": 7}`
	expected := `Application: secret
Repository: git@git.com:php.git
Platform: ruby
Teams: tsuruteam, crane
Address: secret.tsuru.io
Owner: myapp_owner
Team owner: myteam
Deploys: 7
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
	result := `{"name":"app1","teamowner":"myteam","ip":"myapp.tsuru.io","cname":["yourapp.tsuru.io"],"platform":"php","repository":"git@git.com:php.git","state":"dead","units":[{"Ip":"10.10.10.10","Name":"app1/0","Status":"started"}, {"Ip":"9.9.9.9","Name":"app1/1","Status":"started"}, {"Ip":"","Name":"app1/2","Status":"pending"}],"Teams":["tsuruteam","crane"], "owner": "myapp_owner", "deploys": 7}`
	expected := `Application: app1
Repository: git@git.com:php.git
Platform: php
Teams: tsuruteam, crane
Address: yourapp.tsuru.io, myapp.tsuru.io
Owner: myapp_owner
Team owner: myteam
Deploys: 7
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
	result := `[{"ip":"10.10.10.10","name":"app1","ready":true,"units":[{"Name":"app1/0","Status":"started"}]}]`
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
	result := `[{"ip":"10.10.10.11","name":"sapp","ready":true,"units":[{"Name":"sapp1/0","Status":"started"}]},{"ip":"10.10.10.10","name":"app1","ready":true,"units":[{"Name":"app1/0","Status":"started"}]}]`
	expected := `+-------------+-------------------------+-------------+--------+
| Application | Units State Summary     | Address     | Ready? |
+-------------+-------------------------+-------------+--------+
| app1        | 1 of 1 units in-service | 10.10.10.10 | Yes    |
+-------------+-------------------------+-------------+--------+
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

func (s *S) TestAppListUnitIsntAvailable(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	result := `[{"ip":"10.10.10.10","name":"app1","ready":true,"units":[{"Name":"app1/0","Status":"pending"}]}]`
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

func (s *S) TestAppListUnitIsAvailable(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	result := `[{"ip":"10.10.10.10","name":"app1","ready":true,"units":[{"Name":"app1/0","Status":"unreachable"}]}]`
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

func (s *S) TestAppListUnitWithoutName(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	result := `[{"ip":"10.10.10.10","name":"app1","ready":true,"units":[{"Name":"","Status":"pending"}]}]`
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
	result := `[{"ip":"10.10.10.10","name":"app1","ready":false,"units":[{"Name":"","Status":"pending"}]}]`
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
	result := `[{"ip":"10.10.10.10","cname":["app1.tsuru.io"],"name":"app1","ready":true,"units":[{"Name":"app1/0","Status":"started"}]}]`
	expected := `+-------------+-------------------------+---------------+--------+
| Application | Units State Summary     | Address       | Ready? |
+-------------+-------------------------+---------------+--------+
| app1        | 1 of 1 units in-service | app1.tsuru.io | Yes    |
|             |                         | 10.10.10.10   |        |
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
			return req.URL.Path == "/apps/handful_of_nothing/restart" && req.Method == "POST"
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
			return req.URL.Path == "/apps/motorbreath/restart" && req.Method == "POST"
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

func (s *S) TestAddCName(c *gocheck.C) {
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
			var m map[string][]string
			err := json.NewDecoder(req.Body).Decode(&m)
			c.Assert(err, gocheck.IsNil)
			c.Assert(m["cname"], gocheck.DeepEquals, []string{"death.evergrey.mycompany.com"})
			return req.URL.Path == "/apps/death/cname" &&
				req.Method == "POST"
		},
	}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, manager)
	command := AddCName{}
	command.Flags().Parse(true, []string{"-a", "death"})
	err := command.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(called, gocheck.Equals, true)
	c.Assert(stdout.String(), gocheck.Equals, "cname successfully defined.\n")
}

func (s *S) TestAddCNameWithoutTheFlag(c *gocheck.C) {
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
			var m map[string][]string
			err := json.NewDecoder(req.Body).Decode(&m)
			c.Assert(err, gocheck.IsNil)
			c.Assert(m["cname"], gocheck.DeepEquals, []string{"corey.evergrey.mycompany.com"})
			return req.URL.Path == "/apps/corey/cname" &&
				req.Method == "POST"
		},
	}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, manager)
	err := (&AddCName{GuessingCommand{G: fake}}).Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(called, gocheck.Equals, true)
	c.Assert(stdout.String(), gocheck.Equals, "cname successfully defined.\n")
}

func (s *S) TestAddCNameFailure(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
		Args:   []string{"masterplan.evergrey.mycompany.com"},
	}
	trans := &testing.Transport{Message: "Invalid cname", Status: http.StatusPreconditionFailed}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, manager)
	command := AddCName{}
	command.Flags().Parse(true, []string{"-a", "masterplan"})
	err := command.Run(&context, client)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "Invalid cname")
}

func (s *S) TestAddCNameInfo(c *gocheck.C) {
	expected := &cmd.Info{
		Name:    "add-cname",
		Usage:   "add-cname <cname> [<cname> ...] [--app appname]",
		Desc:    `adds a cname for your app.`,
		MinArgs: 1,
	}
	c.Assert((&AddCName{}).Info(), gocheck.DeepEquals, expected)
}

func (s *S) TestAddCNameIsAFlaggedCommand(c *gocheck.C) {
	var _ cmd.FlaggedCommand = &AddCName{}
}

func (s *S) TestRemoveCName(c *gocheck.C) {
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
	command := RemoveCName{}
	command.Flags().Parse(true, []string{"--app", "death"})
	err := command.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(called, gocheck.Equals, true)
	c.Assert(stdout.String(), gocheck.Equals, "cname successfully undefined.\n")
}

func (s *S) TestRemoveCNameWithoutTheFlag(c *gocheck.C) {
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
	err := (&RemoveCName{GuessingCommand{G: fake}}).Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(called, gocheck.Equals, true)
	c.Assert(stdout.String(), gocheck.Equals, "cname successfully undefined.\n")
}

func (s *S) TestRemoveCNameInfo(c *gocheck.C) {
	expected := &cmd.Info{
		Name:    "remove-cname",
		Usage:   "remove-cname <cname> [<cname> ...] [--app appname]",
		Desc:    `removes cnames of your app.`,
		MinArgs: 1,
	}
	c.Assert((&RemoveCName{}).Info(), gocheck.DeepEquals, expected)
}

func (s *S) TestRemoveCNameIsAFlaggedCommand(c *gocheck.C) {
	var _ cmd.FlaggedCommand = &RemoveCName{}
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

func (s *S) TestAppStart(c *gocheck.C) {
	var (
		called         bool
		stdout, stderr bytes.Buffer
	)
	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
	}
	trans := &testing.ConditionalTransport{
		Transport: testing.Transport{Message: "Started", Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			called = true
			return req.URL.Path == "/apps/handful_of_nothing/start" && req.Method == "POST"
		},
	}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, manager)
	command := AppStart{}
	command.Flags().Parse(true, []string{"--app", "handful_of_nothing"})
	err := command.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(called, gocheck.Equals, true)
	c.Assert(stdout.String(), gocheck.Equals, "Started")
}

func (s *S) TestAppStartWithoutTheFlag(c *gocheck.C) {
	var (
		called         bool
		stdout, stderr bytes.Buffer
	)
	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
	}
	trans := &testing.ConditionalTransport{
		Transport: testing.Transport{Message: "Started", Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			called = true
			return req.URL.Path == "/apps/motorbreath/start" && req.Method == "POST"
		},
	}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, manager)
	fake := &FakeGuesser{name: "motorbreath"}
	command := AppStart{GuessingCommand: GuessingCommand{G: fake}}
	command.Flags().Parse(true, nil)
	err := command.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(called, gocheck.Equals, true)
	c.Assert(stdout.String(), gocheck.Equals, "Started")
}

func (s *S) TestAppStartIsAFlaggedCommand(c *gocheck.C) {
	var _ cmd.FlaggedCommand = &AppStart{}
}

func (s *S) TestUnitAvailable(c *gocheck.C) {
	u := &unit{Status: "unreachable"}
	c.Assert(u.Available(), gocheck.Equals, true)
	u = &unit{Status: "started"}
	c.Assert(u.Available(), gocheck.Equals, true)
	u = &unit{Status: "down"}
	c.Assert(u.Available(), gocheck.Equals, false)
}

func (s *S) TestAppStopInfo(c *gocheck.C) {
	expected := &cmd.Info{
		Name:  "stop",
		Usage: "stop [--app appname]",
		Desc: `stops an app.

If you don't provide the app name, tsuru will try to guess it.`,
		MinArgs: 0,
	}
	c.Assert((&AppStop{}).Info(), gocheck.DeepEquals, expected)
}

func (s *S) TestAppStop(c *gocheck.C) {
	var (
		called         bool
		stdout, stderr bytes.Buffer
	)
	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
	}
	trans := &testing.ConditionalTransport{
		Transport: testing.Transport{Message: "Stopped", Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			called = true
			return req.URL.Path == "/apps/handful_of_nothing/stop" && req.Method == "POST"
		},
	}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, manager)
	command := AppStop{}
	command.Flags().Parse(true, []string{"--app", "handful_of_nothing"})
	err := command.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(called, gocheck.Equals, true)
	c.Assert(stdout.String(), gocheck.Equals, "Stopped")
}

func (s *S) TestAppStopWithoutTheFlag(c *gocheck.C) {
	var (
		called         bool
		stdout, stderr bytes.Buffer
	)
	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
	}
	trans := &testing.ConditionalTransport{
		Transport: testing.Transport{Message: "Stopped", Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			called = true
			return req.URL.Path == "/apps/motorbreath/stop" && req.Method == "POST"
		},
	}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, manager)
	fake := &FakeGuesser{name: "motorbreath"}
	command := AppStop{GuessingCommand: GuessingCommand{G: fake}}
	command.Flags().Parse(true, nil)
	err := command.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(called, gocheck.Equals, true)
	c.Assert(stdout.String(), gocheck.Equals, "Stopped")
}

func (s *S) TestAppStopIsAFlaggedCommand(c *gocheck.C) {
	var _ cmd.FlaggedCommand = &AppStop{}
}
