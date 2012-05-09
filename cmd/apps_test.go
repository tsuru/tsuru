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
