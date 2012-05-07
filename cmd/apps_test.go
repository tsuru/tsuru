package cmd

import (
	"bytes"
	"encoding/json"
	"github.com/timeredbull/tsuru/api/app"
	"github.com/timeredbull/tsuru/api/auth"
	. "launchpad.net/gocheck"
)

func (s *S) TestShowApps(c *C) {
	user := &auth.User{Email: "whydidifall@thewho.com", Password: "123"}
	team := auth.Team{Name: "tsuruteam", Users: []*auth.User{user}}
	app1 := app.App{Name: "app1", Teams: []auth.Team{team}}
	appsList := []app.App{app1}
	result, err := json.Marshal(appsList)
	c.Assert(err, IsNil)

	context := Context{[]string{}, manager.Stdout, manager.Stderr}
	err = AppsCommand{}.Show(result, &context)
	c.Assert(err, IsNil)
	table := NewTable()
	table.Headers = Row{"Application", "State", "Ip"}
	table.AddRow(Row{"app1", "", ""})
	c.Assert(manager.Stdout.(*bytes.Buffer).String(), Equals, table.String())
}
