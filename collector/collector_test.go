package main

import (
	"github.com/timeredbull/commandmocker"
	"github.com/timeredbull/tsuru/api/app"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"os"
	"path/filepath"
)

func getOutput() *output {
	return &output{
		Services: map[string]Service{
			"umaappqq": Service{
				Units: map[string]app.Unit{
					"umaappqq/0": app.Unit{
						AgentState: "started",
						Machine:    1,
					},
				},
			},
		},
		Machines: map[int]interface{}{
			0: map[interface{}]interface{}{
				"dns-name":       "192.168.0.10",
				"instance-id":    "i-00000zz6",
				"instance-state": "running",
				"agent-state":    "running",
			},
			1: map[interface{}]interface{}{
				"dns-name":       "192.168.0.11",
				"instance-id":    "i-00000zz7",
				"instance-state": "running",
				"agent-state":    "running",
			},
		},
	}
}

func getApp(c *C) *app.App {
	a := &app.App{Name: "umaappqq", State: "STOPPED"}
	err := a.Create()
	c.Assert(err, IsNil)
	return a
}

func (s *S) TestUpdate(c *C) {
	a := getApp(c)
	out := getOutput()
	update(out)

	err := a.Get()
	c.Assert(err, IsNil)
	c.Assert(a.State, Equals, "started")
	c.Assert(a.Units[0].Ip, Equals, "192.168.0.11")
	c.Assert(a.Units[0].Machine, Equals, 1)
	c.Assert(a.Units[0].InstanceState, Equals, "running")
	c.Assert(a.Units[0].MachineAgentState, Equals, "running")
	c.Assert(a.Units[0].AgentState, Equals, "started")
	c.Assert(a.Units[0].InstanceId, Equals, "i-00000zz7")

	a.Destroy()
}

func (s *S) TestUpdateWithMultipleUnits(c *C) {
	a := getApp(c)
	out := getOutput()
	u := app.Unit{AgentState: "started", Machine: 2}
	out.Services["umaappqq"].Units["umaappqq/1"] = u
	out.Machines[2] = map[interface{}]interface{}{
		"dns-name":       "192.168.0.12",
		"instance-id":    "i-00000zz8",
		"instance-state": "running",
		"agent-state":    "running",
	}
	update(out)
	err := a.Get()
	c.Assert(err, IsNil)
	c.Assert(len(a.Units), Equals, 2)
	for _, u = range a.Units {
		if u.Machine == 2 {
			break
		}
	}
	c.Assert(u.Ip, Equals, "192.168.0.12")
	c.Assert(u.InstanceState, Equals, "running")
	c.Assert(u.AgentState, Equals, "started")
	c.Assert(u.MachineAgentState, Equals, "running")
}

func (s *S) TestUpdateWithDownMachine(c *C) {
	a := app.App{Name: "barduscoapp", State: "STOPPED"}
	err := a.Create()
	c.Assert(err, IsNil)
	file, _ := os.Open(filepath.Join("testdata", "broken-output.yaml"))
	jujuOutput, _ := ioutil.ReadAll(file)
	file.Close()
	out := parse(jujuOutput)
	update(out)
	err = a.Get()
	c.Assert(err, IsNil)
	c.Assert(a.State, Equals, "creating")
}

func (s *S) TestUpdateTwice(c *C) {
	a := getApp(c)
	defer a.Destroy()
	out := getOutput()
	update(out)
	err := a.Get()
	c.Assert(err, IsNil)
	c.Assert(a.State, Equals, "started")
	c.Assert(a.Units[0].Ip, Equals, "192.168.0.11")
	c.Assert(a.Units[0].Machine, Equals, 1)
	c.Assert(a.Units[0].InstanceState, Equals, "running")
	c.Assert(a.Units[0].MachineAgentState, Equals, "running")
	c.Assert(a.Units[0].AgentState, Equals, "started")
	update(out)
	err = a.Get()
	c.Assert(len(a.Units), Equals, 1)
}

func (s *S) TestUpdateWithMultipleApps(c *C) {
	appDicts := []map[string]string{
		map[string]string{
			"name": "andrewzito3",
			"ip":   "10.10.10.163",
		},
		map[string]string{
			"name": "flaviapp",
			"ip":   "10.10.10.208",
		},
		map[string]string{
			"name": "mysqlapi",
			"ip":   "10.10.10.131",
		},
		map[string]string{
			"name": "teste_api_semantica",
			"ip":   "10.10.10.189",
		},
		map[string]string{
			"name": "xikin",
			"ip":   "10.10.10.168",
		},
	}
	apps := make([]app.App, len(appDicts))
	for i, appDict := range appDicts {
		a := app.App{Name: appDict["name"]}
		err := a.Create()
		c.Assert(err, IsNil)
		apps[i] = a
	}
	jujuOutput, err := ioutil.ReadFile(filepath.Join("testdata", "multiple-apps.yaml"))
	c.Assert(err, IsNil)
	data := parse(jujuOutput)
	update(data)
	for _, appDict := range appDicts {
		a := app.App{Name: appDict["name"]}
		err := a.Get()
		c.Assert(err, IsNil)
		c.Assert(a.Units[0].Ip, Equals, appDict["ip"])
	}
}

func (s *S) TestParser(c *C) {
	file, _ := os.Open(filepath.Join("testdata", "output.yaml"))
	jujuOutput, _ := ioutil.ReadAll(file)
	file.Close()
	expected := getOutput()
	c.Assert(parse(jujuOutput), DeepEquals, expected)
}

func (s *S) TestCollect(c *C) {
	tmpdir, err := commandmocker.Add("juju", "$*")
	c.Assert(err, IsNil)
	defer commandmocker.Remove(tmpdir)
	out, err := collect()
	c.Assert(err, IsNil)
	c.Assert(string(out), Equals, "status")
}

func (s *S) TestAppStatusMachineAgentPending(c *C) {
	u := app.Unit{MachineAgentState: "pending"}
	st := appState(&u)
	c.Assert(st, Equals, "creating")
}

func (s *S) TestAppStatusInstanceStatePending(c *C) {
	u := app.Unit{InstanceState: "pending"}
	st := appState(&u)
	c.Assert(st, Equals, "creating")
}

func (s *S) TestAppStatusInstanceStateError(c *C) {
	u := app.Unit{InstanceState: "error"}
	st := appState(&u)
	c.Assert(st, Equals, "error")
}

func (s *S) TestAppStatusAgentStatePending(c *C) {
	u := app.Unit{AgentState: "pending", InstanceState: ""}
	st := appState(&u)
	c.Assert(st, Equals, "creating")
}

func (s *S) TestAppStatusAgentAndInstanceRunning(c *C) {
	u := app.Unit{AgentState: "started", InstanceState: "running", MachineAgentState: "running"}
	st := appState(&u)
	c.Assert(st, Equals, "started")
}

func (s *S) TestAppStatusMachineAgentRunningAndInstanceAndAgentPending(c *C) {
	u := app.Unit{AgentState: "pending", InstanceState: "running", MachineAgentState: "running"}
	st := appState(&u)
	c.Assert(st, Equals, "installing")
}

func (s *S) TestAppStatusInstancePending(c *C) {
	u := app.Unit{AgentState: "not-started", InstanceState: "pending"}
	st := appState(&u)
	c.Assert(st, Equals, "creating")
}

func (s *S) TestAppStatusInstancePendingWhenMachineStateIsRunning(c *C) {
	u := app.Unit{AgentState: "not-started", MachineAgentState: "running"}
	st := appState(&u)
	c.Assert(st, Equals, "creating")
}

func (s *S) TestAppStatePending(c *C) {
	u := app.Unit{MachineAgentState: "some-state", AgentState: "some-state", InstanceState: "some-other-state"}
	st := appState(&u)
	c.Assert(st, Equals, "pending")
}
