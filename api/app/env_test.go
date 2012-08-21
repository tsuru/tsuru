package app

import (
	"github.com/timeredbull/commandmocker"
	"github.com/timeredbull/tsuru/fs"
	"github.com/timeredbull/tsuru/fs/testing"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/goyaml"
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
	msg := Message{
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
	app := App{Name: "rainmaker", Teams: []string{s.team.Name}}
	err = app.Create()
	c.Assert(err, IsNil)
	msg := Message{
		app: &app,
	}
	env <- msg
}

func (s *S) TestNewEnviron(c *C) {
	expected := map[string]map[string]JujuEnv{}
	result := map[string]map[string]JujuEnv{}
	expected["environments"] = map[string]JujuEnv{}
	expected["environments"]["name"] = JujuEnv{Access: "access", Secret: "secret"}
	rfs := &testing.RecordingFs{}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	err := NewEnviron("name", "access", "secret")
	c.Assert(err, IsNil)
	c.Assert(rfs.HasAction("openfile "+EnvironConfPath+" with mode 0600"), Equals, true)
	file, err := rfs.Open(EnvironConfPath)
	c.Assert(err, IsNil)
	content, err := ioutil.ReadAll(file)
	c.Assert(err, IsNil)
	goyaml.Unmarshal(content, &result)
	c.Assert(result, DeepEquals, expected)
}

func (s *S) TestNewEnvironShouldKeepExistentsEnvirons(c *C) {
	expected := map[string]map[string]JujuEnv{}
	result := map[string]map[string]JujuEnv{}
	initial := map[string]map[string]JujuEnv{}
	initial["environments"] = map[string]JujuEnv{}
	initial["environments"]["foo"] = JujuEnv{Access: "foo", Secret: "foo"}
	expected["environments"] = map[string]JujuEnv{}
	expected["environments"]["foo"] = JujuEnv{Access: "foo", Secret: "foo"}
	expected["environments"]["name"] = JujuEnv{Access: "access", Secret: "secret"}
	data, err := goyaml.Marshal(&initial)
	c.Assert(err, IsNil)
	rfs := &testing.RecordingFs{FileContent: string(data)}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	err = NewEnviron("name", "access", "secret")
	c.Assert(err, IsNil)
	c.Assert(rfs.HasAction("openfile "+EnvironConfPath+" with mode 0600"), Equals, true)
	file, err := rfs.Open(EnvironConfPath)
	c.Assert(err, IsNil)
	content, err := ioutil.ReadAll(file)
	c.Assert(err, IsNil)
	goyaml.Unmarshal(content, &result)
	c.Assert(result, DeepEquals, expected)
}

func (s *S) TestEnvironConfPath(c *C) {
	expected := path.Join(os.ExpandEnv("${HOME}"), ".juju", "environments.yml")
	c.Assert(EnvironConfPath, Equals, expected)
}

func (s *S) TestFileSystem(c *C) {
	fsystem = &testing.RecordingFs{}
	c.Assert(filesystem(), DeepEquals, fsystem)
	fsystem = nil
	c.Assert(filesystem(), DeepEquals, fs.OsFs{})
}
