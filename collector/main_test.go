package main

import (
	"github.com/timeredbull/tsuru/api/app"
	"github.com/timeredbull/tsuru/db"
	"labix.org/v2/mgo/bson"
	. "launchpad.net/gocheck"
)

var pendingApps = []string{"as_i_rise", "the_infanta"}
var runningApps = []string{"caravan", "bu2b", "carnies"}

func createApp(name, agentState, machineAgentState, instanceState string) {
	a := app.App{
		Name: name,
		Units: []app.Unit{
			app.Unit{
				AgentState:        agentState,
				MachineAgentState: machineAgentState,
				InstanceState:     instanceState,
			},
		},
	}
	err := a.Create()
	if err != nil {
		panic(err)
	}
}

func createApps() {
	for _, name := range pendingApps {
		createApp(name, "not-started", "pending", "pending")
	}
	for _, name := range runningApps {
		createApp(name, "started", "running", "running")
	}
}

func destroyApps() {
	allApps := append(pendingApps, runningApps...)
	db.Session.Apps().Remove(bson.M{"name": bson.M{"$in": allApps}})
}

func (s *S) TestGetApps(c *C) {
	createApps()
	defer destroyApps()
	apps := getApps()
	names := make([]string, len(apps))
	for i, app := range apps {
		names[i] = app.Name
	}
	c.Assert(names, DeepEquals, pendingApps)
}
