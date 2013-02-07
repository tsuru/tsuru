package local

import (
	"github.com/globocom/tsuru/testing"
	. "launchpad.net/gocheck"
)

func (s *S) TestProvision(c *C) {
	var p LocalProvisioner
	app := testing.NewFakeApp("myapp", "python", 0)
	c.Assert(p.Provision(app), IsNil)
}
