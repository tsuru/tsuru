package local

import (
	"github.com/globocom/commandmocker"
	. "launchpad.net/gocheck"
)

func (s *S) TestCreate(c *C) {
	tmpdir, err := commandmocker.Add("sudo", "$*")
	c.Assert(err, IsNil)
	defer commandmocker.Remove(tmpdir)
	container := container{name: "container"}
	err = container.create()
	c.Assert(err, IsNil)
	c.Assert(commandmocker.Ran(tmpdir), Equals, true)
	expected := "lxc-create -t ubuntu -n container"
	c.Assert(commandmocker.Output(tmpdir), Equals, expected)
}

func (s *S) TestStart(c *C) {
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

func (s *S) TestStop(c *C) {
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

func (s *S) TestDestroy(c *C) {
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
