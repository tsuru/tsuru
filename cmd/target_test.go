package main

import (
	"bytes"
	. "launchpad.net/gocheck"
	"os"
	"path"
)

func deleteTsuruTarget() {
	home := os.ExpandEnv("${HOME}")
	os.Remove(path.Join(home, ".tsuru_target"))
}

func (s *S) TestDefaultTarget(c *C) {
	c.Assert(DefaultTarget, Equals, "http://tsuru.plataformas.glb.com:8080")
}

func (s *S) TestWriteAndReadTarget(c *C) {
	defer deleteTsuruTarget()
	err := WriteTarget("http://tsuru.globo.com")
	c.Assert(err, IsNil)
	target := ReadTarget()
	c.Assert(target, Equals, "http://tsuru.globo.com")
}

func (s *S) TestWriteTargetShouldStripLeadingSlashs(c *C) {
	defer deleteTsuruTarget()
	err := WriteTarget("http://tsuru.globo.com/")
	c.Assert(err, IsNil)
	target := ReadTarget()
	c.Assert(target, Equals, "http://tsuru.globo.com")
}

func (s *S) TestWriteTargetShouldStringAllLeadingSlashs(c *C) {
	defer deleteTsuruTarget()
	err := WriteTarget("http://tsuru.globo.com////")
	c.Assert(err, IsNil)
	target := ReadTarget()
	c.Assert(target, Equals, "http://tsuru.globo.com")
}

func (s *S) TestReadTargetReturnsDefaultTargetIfTheFileDoesNotExist(c *C) {
	deleteTsuruTarget()
	target := ReadTarget()
	c.Assert(target, Equals, DefaultTarget)
}

func (s *S) TestTargetInfo(c *C) {
	expected := &Info{
		Name:    "target",
		Usage:   "target <target>",
		Desc:    "Defines the target (tsuru server)",
		MinArgs: 1,
	}
	target := &Target{}
	c.Assert(target.Info(), DeepEquals, expected)
}

func (s *S) TestTargetRun(c *C) {
	deleteTsuruTarget()
	context := &Context{[]string{}, []string{"http://tsuru.globo.com"}, manager.Stdout, manager.Stderr}
	target := &Target{}
	err := target.Run(context, nil)
	c.Assert(err, IsNil)
	c.Assert(context.Stdout.(*bytes.Buffer).String(), Equals, "New target is http://tsuru.globo.com\n")
	c.Assert(ReadTarget(), Equals, "http://tsuru.globo.com")
}

func (s *S) TestGetUrl(c *C) {
	deleteTsuruTarget()
	expected := DefaultTarget + "/apps"
	got := GetUrl("/apps")
	c.Assert(got, Equals, expected)
}

func (s *S) TestGetUrlPutsHttpIfItIsNotPresent(c *C) {
	defer deleteTsuruTarget()
	err := WriteTarget("localhost")
	c.Assert(err, IsNil)
	expected := "http://localhost/apps"
	got := GetUrl("/apps")
	c.Assert(got, Equals, expected)
}

func (s *S) TestGetUrlShouldNotPrependHttpIfTheTargetIsHttps(c *C) {
	defer deleteTsuruTarget()
	err := WriteTarget("https://localhost")
	c.Assert(err, IsNil)
	got := GetUrl("/apps")
	c.Assert(got, Equals, "https://localhost/apps")
}
