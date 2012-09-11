package app

import (
	"bytes"
	"github.com/timeredbull/tsuru/config"
	"github.com/timeredbull/tsuru/fs/testing"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/goyaml"
	"syscall"
)

func (s *S) TestnewJujuEnvConf(c *C) {
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
	result, err := newJujuEnvConf("access", "secret")
	c.Assert(err, IsNil)
	c.Assert(result, DeepEquals, expected)
}

func (s *S) TestNewEnviron(c *C) {
	expected := map[string]map[string]jujuEnv{}
	result := map[string]map[string]jujuEnv{}
	expected["environments"] = map[string]jujuEnv{}
	nameEnv, err := newJujuEnvConf("access", "secret")
	expected["environments"]["name"] = nameEnv
	rfs := &testing.RecordingFs{}
	file, err := rfs.Open("/dev/urandom")
	defer file.Close()
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
	err = newEnvironConf(&a)
	c.Assert(err, IsNil)
	c.Assert(rfs.HasAction("openfile "+environConfPath+" with mode 0600"), Equals, true)
	file, err = rfs.Open(environConfPath)
	defer file.Close()
	c.Assert(err, IsNil)
	content, err := ioutil.ReadAll(file)
	c.Assert(err, IsNil)
	goyaml.Unmarshal(content, &result)
	c.Assert(result, DeepEquals, expected)
}

func (s *S) TestNewEnvironShouldKeepExistentsEnvirons(c *C) {
	expected := map[string]map[string]jujuEnv{}
	initial := map[string]map[string]jujuEnv{}
	initial["environments"] = map[string]jujuEnv{}
	fooEnv, err := newJujuEnvConf("foo", "foo")
	c.Assert(err, IsNil)
	initial["environments"]["foo"] = fooEnv
	expected["environments"] = map[string]jujuEnv{}
	expected["environments"]["foo"] = fooEnv
	nameEnv, err := newJujuEnvConf("access", "secret")
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
	var result map[string]map[string]jujuEnv
	err = newEnvironConf(&a)
	c.Assert(err, IsNil)
	c.Assert(rfs.HasAction("openfile "+environConfPath+" with mode 0600"), Equals, true)
	file, err = rfs.Open(environConfPath)
	c.Assert(err, IsNil)
	content, err := ioutil.ReadAll(file)
	c.Assert(err, IsNil)
	// Issue #127.
	c.Assert(bytes.Count(content, []byte("environments:")), Equals, 1)
	err = goyaml.Unmarshal(content, &result)
	c.Assert(err, IsNil)
	c.Assert(result, DeepEquals, expected)
}

func (s *S) TestRemoveEnviron(c *C) {
	expected := map[string]map[string]jujuEnv{}
	expected["environments"] = map[string]jujuEnv{}
	env1, err := newJujuEnvConf("access", "secret")
	expected["environments"]["env1"] = env1
	env2, err := newJujuEnvConf("access", "secret")
	expected["environments"]["env2"] = env2
	rfs := &testing.RecordingFs{}
	file, err := rfs.OpenFile(environConfPath, syscall.O_RDWR, 0600)
	file.Write([]byte{16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31})
	data, err := goyaml.Marshal(&expected)
	c.Assert(err, IsNil)
	_, err = file.Write(data)
	c.Assert(err, IsNil)
	fsystem = rfs
	defer func() {
		fsystem = s.rfs
	}()
	a := App{
		Name: "env2",
		KeystoneEnv: keystoneEnv{
			AccessKey: "access",
			secretKey: "secret",
		},
	}
	err = removeEnvironConf(&a)
	c.Assert(err, IsNil)
	c.Assert(rfs.HasAction("openfile "+environConfPath+" with mode 0600"), Equals, true)
	delete(expected["environments"], "env2")
	file, err = rfs.Open(environConfPath)
	c.Assert(err, IsNil)
	content, err := ioutil.ReadAll(file)
	c.Assert(err, IsNil)
	result := map[string]map[string]jujuEnv{}
	goyaml.Unmarshal(content, &result)
	c.Assert(result, DeepEquals, expected)
}
