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
