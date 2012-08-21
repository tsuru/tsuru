package main

import (
	"github.com/timeredbull/commandmocker"
	"github.com/timeredbull/tsuru/api/app"
	"github.com/timeredbull/tsuru/db"
	"io/ioutil"
	"labix.org/v2/mgo/bson"
	. "launchpad.net/gocheck"
	"path/filepath"
	"time"
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
	err := db.Session.Apps().Insert(&a)
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

func (s *S) TestJujuCollect(c *C) {
	b, err := ioutil.ReadFile(filepath.Join("testdata", "jujucollect.yaml"))
	c.Assert(err, IsNil)
	tmpdir, err := commandmocker.Add("juju", string(b))
	c.Assert(err, IsNil)
	defer commandmocker.Remove(tmpdir)
	createApps()
	defer destroyApps()
	ch := make(chan time.Time)
	go jujuCollect(ch)
	ch <- time.Now()
	close(ch)
	time.Sleep(1e9)
	var apps []app.App
	err = db.Session.Apps().Find(bson.M{"name": bson.M{"$in": []string{"as_i_rise", "the_infanta"}}}).Sort("name").All(&apps)
	c.Assert(err, IsNil)
	c.Assert(apps[0].Units[1].Ip, Equals, "10.10.10.163")
	c.Assert(apps[1].Units[1].Ip, Equals, "10.10.10.168")
}
