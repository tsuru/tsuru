package local

import (
	"github.com/globocom/commandmocker"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/fs/testing"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"os"
)

func (s *S) TestLXCCreate(c *C) {
	config.Set("local:authorized-key-path", "somepath")
	key := "somekey"
	rfs := &testing.RecordingFs{FileContent: string(key)}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	tmpdir, err := commandmocker.Add("sudo", "$*")
	c.Assert(err, IsNil)
	defer commandmocker.Remove(tmpdir)
	container := container{name: "container"}
	err = container.create()
	c.Assert(err, IsNil)
	c.Assert(commandmocker.Ran(tmpdir), Equals, true)
	expected := "lxc-create -t ubuntu -n container -- -S " + key
	c.Assert(commandmocker.Output(tmpdir), Equals, expected)
}

func (s *S) TestLXCStart(c *C) {
	tmpdir, err := commandmocker.Add("sudo", "$*")
	c.Assert(err, IsNil)
	defer commandmocker.Remove(tmpdir)
	container := container{name: "container"}
	err = container.start()
	c.Assert(err, IsNil)
	c.Assert(commandmocker.Ran(tmpdir), Equals, true)
	expected := "lxc-start --daemon -n container"
	c.Assert(commandmocker.Output(tmpdir), Equals, expected)
}

func (s *S) TestLXCStop(c *C) {
	tmpdir, err := commandmocker.Add("sudo", "$*")
	c.Assert(err, IsNil)
	defer commandmocker.Remove(tmpdir)
	container := container{name: "container"}
	err = container.stop()
	c.Assert(err, IsNil)
	c.Assert(commandmocker.Ran(tmpdir), Equals, true)
	expected := "lxc-stop -n container"
	c.Assert(commandmocker.Output(tmpdir), Equals, expected)
}

func (s *S) TestLXCDestroy(c *C) {
	tmpdir, err := commandmocker.Add("sudo", "$*")
	c.Assert(err, IsNil)
	defer commandmocker.Remove(tmpdir)
	container := container{name: "container"}
	err = container.destroy()
	c.Assert(err, IsNil)
	c.Assert(commandmocker.Ran(tmpdir), Equals, true)
	expected := "lxc-destroy -n container"
	c.Assert(commandmocker.Output(tmpdir), Equals, expected)
}

func (s *S) TestContainerIP(c *C) {
	file, _ := os.Open("testdata/dnsmasq.leases")
	data, err := ioutil.ReadAll(file)
	c.Assert(err, IsNil)
	rfs := &testing.RecordingFs{FileContent: string(data)}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	cont := container{name: "vm1"}
	c.Assert(cont.ip(), Equals, "10.10.10.10")
	cont = container{name: "notfound"}
	c.Assert(cont.ip(), Equals, "")
}

func (s *S) TestGetAuthorizedKey(c *C) {
	config.Set("local:authorized-key-path", "somepath")
	expected := "somekey"
	rfs := &testing.RecordingFs{FileContent: string(expected)}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	key, err := authorizedKey()
	c.Assert(err, IsNil)
	c.Assert(key, Equals, expected)
}
