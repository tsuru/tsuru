package local

import (
	"github.com/globocom/commandmocker"
	"github.com/globocom/tsuru/testing"
	. "launchpad.net/gocheck"
)

func (s *S) TestProvision(c *C) {
	tmpdir, err := commandmocker.Add("sudo", "$*")
	c.Assert(err, IsNil)
	defer commandmocker.Remove(tmpdir)
	var p LocalProvisioner
	app := testing.NewFakeApp("myapp", "python", 0)
	c.Assert(p.Provision(app), IsNil)
	c.Assert(commandmocker.Ran(tmpdir), Equals, true)
	expected := "lxc-create -t ubuntu -n myapp"
	expected += "lxc-start --daemon -n myapp"
	c.Assert(commandmocker.Output(tmpdir), Equals, expected)
}
