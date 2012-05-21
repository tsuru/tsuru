package cmd

import (
	. "launchpad.net/gocheck"
	"os"
	"path"
)

func (s *S) TestDefaultTarget(c *C) {
	c.Assert(DefaultTarget, Equals, "tsuru.plataformas.glb.com")
}

func (s *S) TestWriteAndReadTarget(c *C) {
	err := WriteTarget("tsuru.globo.com")
	c.Assert(err, IsNil)
	target := ReadTarget()
	c.Assert(target, Equals, "tsuru.globo.com")
}

func (s *S) TestReadTargetReturnsDefaultTargetIfTheFileDoesNotExist(c *C) {
	home := os.ExpandEnv("${HOME}")
	os.Remove(path.Join(home, ".tsuru_target"))
	target := ReadTarget()
	c.Assert(target, Equals, DefaultTarget)
}
