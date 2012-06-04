package unit

import (
	"github.com/timeredbull/commandmocker"
	. "launchpad.net/gocheck"
	"testing"
)

func Test(t *testing.T) { TestingT(t) }

type S struct {
	tmpdir string
}

var _ = Suite(&S{})

func (s *S) SetUpSuite(c *C) {
	var err error
	s.tmpdir, err = commandmocker.Add("juju", "Linux")
	c.Assert(err, IsNil)
}

func (s *S) TearDownSuite(c *C) {
	err := commandmocker.Remove(s.tmpdir)
	c.Assert(err, IsNil)
}
