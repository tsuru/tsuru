package app

import (
	"github.com/timeredbull/commandmocker"
	"github.com/timeredbull/tsuru/config"
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
	app, err := NewApp("rainmaker", "", []string{s.team.Name})
	c.Assert(err, IsNil)
	msg := message{
		app: &app,
	}
	env <- msg
}

func (s *S) TestnewJujuEnv(c *C) {
	ec2, err := config.GetString("juju:ec2")
	c.Assert(err, IsNil)
	s3, err := config.GetString("juju:s3")
	c.Assert(err, IsNil)
	jujuOrigin, err := config.GetString("juju:origin")
	c.Assert(err, IsNil)
	series, err := config.GetString("juju:series")
	c.Assert(err, IsNil)
	imageId, err := config.GetString("juju:image-id")
	c.Assert(err, IsNil)
	instaceType, err := config.GetString("juju:instance-type")
	c.Assert(err, IsNil)
	expected := jujuEnv{
		Ec2:           ec2,
		S3:            s3,
		JujuOrigin:    jujuOrigin,
		Type:          "ec2",
		AdminSecret:   "101112131415161718191a1b1c1d1e1f",
		ControlBucket: "juju-101112131415161718191a1b1c1d1e1f",
		Series:        series,
		ImageId:       imageId,
		InstanceType:  instaceType,
		AccessKey:     "access",
		SecretKey:     "secret",
	}
	result, err := newJujuEnv("access", "secret")
	c.Assert(err, IsNil)
	c.Assert(result, DeepEquals, expected)
}

func (s *S) TestNewEnviron(c *C) {
	expected := map[string]map[string]jujuEnv{}
	result := map[string]map[string]jujuEnv{}
	expected["environments"] = map[string]jujuEnv{}
	nameEnv, err := newJujuEnv("access", "secret")
	expected["environments"]["name"] = nameEnv
	rfs := &testing.RecordingFs{}
	file, err := rfs.Open("/dev/urandom")
	file.Write([]byte{16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31})
	fsystem = rfs
	defer func() {
		fsystem = s.rfs
	}()
	a := App{
		Name: "name",
		KeystoneEnv: keystoneEnv{
			AccessKey: "access",
			secretKey: "secret",
		},
	}
	err = newEnviron(&a)
	c.Assert(err, IsNil)
	c.Assert(rfs.HasAction("openfile "+environConfPath+" with mode 0600"), Equals, true)
	file, err = rfs.Open(environConfPath)
	c.Assert(err, IsNil)
	content, err := ioutil.ReadAll(file)
	c.Assert(err, IsNil)
	goyaml.Unmarshal(content, &result)
	c.Assert(result, DeepEquals, expected)
}

func (s *S) TestNewEnvironShouldKeepExistentsEnvirons(c *C) {
	expected := map[string]map[string]jujuEnv{}
	result := map[string]map[string]jujuEnv{}
	initial := map[string]map[string]jujuEnv{}
	initial["environments"] = map[string]jujuEnv{}
	fooEnv, err := newJujuEnv("foo", "foo")
	c.Assert(err, IsNil)
	initial["environments"]["foo"] = fooEnv
	expected["environments"] = map[string]jujuEnv{}
	expected["environments"]["foo"] = fooEnv
	nameEnv, err := newJujuEnv("access", "secret")
	c.Assert(err, IsNil)
	expected["environments"]["name"] = nameEnv
	data, err := goyaml.Marshal(&initial)
	c.Assert(err, IsNil)
	rfs := &testing.RecordingFs{FileContent: string(data)}
	file, err := rfs.Open("/dev/urandom")
	file.Write([]byte{16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31})
	fsystem = rfs
	defer func() {
		fsystem = s.rfs
	}()
	a := App{
		Name: "name",
		KeystoneEnv: keystoneEnv{
			AccessKey: "access",
			secretKey: "secret",
		},
	}
	err = newEnviron(&a)
	c.Assert(err, IsNil)
	c.Assert(rfs.HasAction("openfile "+environConfPath+" with mode 0600"), Equals, true)
	file, err = rfs.Open(environConfPath)
	c.Assert(err, IsNil)
	content, err := ioutil.ReadAll(file)
	c.Assert(err, IsNil)
	goyaml.Unmarshal(content, &result)
	c.Assert(result, DeepEquals, expected)
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
