package unit_test

import (
	"flag"
	"github.com/timeredbull/tsuru/api/unit"
	. "launchpad.net/gocheck"
	"testing"
)

func Test(t *testing.T) { TestingT(t) }

type S struct{}

var _ = Suite(&S{})

var lxcEnabled = flag.Bool("juju", false, "enable unit tests that require juju")

func (s *S) SetUpSuite(c *C) {
	if !*lxcEnabled {
		c.Skip("unit tests need juju installed (-juju to enable)")
	}
}

func (s *S) TestCreateAndDestroy(c *C) {
	u := unit.Unit{Type: "django", Name: "myUnit"}

	err := u.Create()
	c.Assert(err, IsNil)

	err = u.Destroy()
	c.Assert(err, IsNil)
}
