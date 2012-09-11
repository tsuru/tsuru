package app

import (
	"github.com/timeredbull/commandmocker"
	"github.com/timeredbull/tsuru/fs"
	"github.com/timeredbull/tsuru/fs/testing"
	. "launchpad.net/gocheck"
	"os"
	"path"
)

func (s *S) TestRewriteEnvMessage(c *C) {
	dir, err := commandmocker.Add("juju", output)
	c.Assert(err, IsNil)
	defer commandmocker.Remove(dir)
	app := App{
		Name:  "time",
		Teams: []string{s.team.Name},
		Units: []Unit{
			Unit{AgentState: "started", MachineAgentState: "running", InstanceState: "running"},
		},
	}
	msg := message{
		app:     &app,
		success: make(chan bool),
	}
	env <- msg
	c.Assert(<-msg.success, Equals, true)
	c.Assert(commandmocker.Ran(dir), Equals, true)
}

func (s *S) TestDoesNotSendInTheSuccessChannelIfItIsNil(c *C) {
	defer func() {
		r := recover()
		c.Assert(r, IsNil)
	}()
	dir, err := commandmocker.Add("juju", output)
	c.Assert(err, IsNil)
	defer commandmocker.Remove(dir)
	app := App{
		Name:      "rainmaker",
		Framework: "",
		Teams:     []string{s.team.Name},
		ec2Auth:   &fakeAuthorizer{},
	}
	err = createApp(&app)
	c.Assert(err, IsNil)
	msg := message{
		app: &app,
	}
	env <- msg
}

func (s *S) TestEnvironConfPath(c *C) {
	expected := path.Join(os.ExpandEnv("${HOME}"), ".juju", "environments.yaml")
	c.Assert(environConfPath, Equals, expected)
}

func (s *S) TestFileSystem(c *C) {
	fsystem = &testing.RecordingFs{}
	c.Assert(filesystem(), DeepEquals, fsystem)
	fsystem = nil
	c.Assert(filesystem(), DeepEquals, fs.OsFs{})
	fsystem = s.rfs
}
