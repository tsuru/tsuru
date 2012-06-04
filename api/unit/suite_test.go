package unit

import (
	. "launchpad.net/gocheck"
	"testing"
)

func Test(t *testing.T) { TestingT(t) }

type S struct{}

var _ = Suite(&S{})

func (s *S) SetUpSuite(c *C) {
	if !*jujuEnabled {
		c.Skip("unit tests need juju installed (-juju to enable)")
	}
}
