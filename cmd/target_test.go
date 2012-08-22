package cmd

import (
	"bytes"
	"github.com/timeredbull/tsuru/fs/testing"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"os"
	"path"
)

func readRecordedTarget(fs *testing.RecordingFs) string {
	filePath := path.Join(os.ExpandEnv("${HOME}"), ".tsuru_target")
	fil, _ := fsystem.Open(filePath)
	b, _ := ioutil.ReadAll(fil)
	return string(b)
}

func (s *S) TestDefaultTarget(c *C) {
	c.Assert(DefaultTarget, Equals, "http://tsuru.plataformas.glb.com:8080")
}

func (s *S) TestWriteTarget(c *C) {
	rfs := &testing.RecordingFs{}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	err := writeTarget("http://tsuru.globo.com")
	c.Assert(err, IsNil)
	filePath := path.Join(os.ExpandEnv("${HOME}"), ".tsuru_target")
	c.Assert(rfs.HasAction("openfile "+filePath+" with mode 0600"), Equals, true)
	c.Assert(readRecordedTarget(rfs), Equals, "http://tsuru.globo.com")
}

func (s *S) TestWriteTargetShouldStripLeadingSlashs(c *C) {
	rfs := &testing.RecordingFs{}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	err := writeTarget("http://tsuru.globo.com/")
	c.Assert(err, IsNil)
	c.Assert(readRecordedTarget(rfs), Equals, "http://tsuru.globo.com")
}

func (s *S) TestWriteTargetShouldStripAllLeadingSlashs(c *C) {
	rfs := &testing.RecordingFs{}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	err := writeTarget("http://tsuru.globo.com////")
	c.Assert(err, IsNil)
	target := readRecordedTarget(rfs)
	c.Assert(target, Equals, "http://tsuru.globo.com")
}

func (s *S) TestReadTarget(c *C) {
	rfs := &testing.RecordingFs{FileContent: "http://tsuru.google.com"}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	target := readTarget()
	c.Assert(target, Equals, "http://tsuru.google.com")
}

func (s *S) TestReadTargetReturnsDefaultTargetIfTheFileDoesNotExist(c *C) {
	fsystem = &testing.FailureFs{}
	defer func() {
		fsystem = nil
	}()
	target := readTarget()
	c.Assert(target, Equals, DefaultTarget)
}

func (s *S) TestTargetInfo(c *C) {
	desc := `Defines or retrieve the target (tsuru server)

If an argument is provided, this command sets the target, otherwise it displays the current target.
`
	expected := &Info{
		Name:    "target",
		Usage:   "target [target]",
		Desc:    desc,
		MinArgs: 0,
	}
	target := &target{}
	c.Assert(target.Info(), DeepEquals, expected)
}

func (s *S) TestTargetRun(c *C) {
	rfs := &testing.RecordingFs{FileContent: "http://tsuru.google.com"}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	context := &Context{[]string{}, []string{"http://tsuru.globo.com"}, manager.Stdout, manager.Stderr}
	target := &target{}
	err := target.Run(context, nil)
	c.Assert(err, IsNil)
	c.Assert(context.Stdout.(*bytes.Buffer).String(), Equals, "New target is http://tsuru.globo.com\n")
	c.Assert(readTarget(), Equals, "http://tsuru.globo.com")
}

func (s *S) TestTargetWithoutArgument(c *C) {
	rfs := &testing.RecordingFs{FileContent: "http://tsuru.google.com"}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	context := &Context{[]string{}, []string{}, manager.Stdout, manager.Stderr}
	target := &target{}
	err := target.Run(context, nil)
	c.Assert(err, IsNil)
	c.Assert(context.Stdout.(*bytes.Buffer).String(), Equals, "Current target is http://tsuru.google.com\n")
}

func (s *S) TestGetUrl(c *C) {
	fsystem = &testing.FailureFs{}
	defer func() {
		fsystem = nil
	}()
	expected := DefaultTarget + "/apps"
	got := GetUrl("/apps")
	c.Assert(got, Equals, expected)
}

func (s *S) TestGetUrlPutsHttpIfItIsNotPresent(c *C) {
	rfs := &testing.RecordingFs{FileContent: "localhost"}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	expected := "http://localhost/apps"
	got := GetUrl("/apps")
	c.Assert(got, Equals, expected)
}

func (s *S) TestGetUrlShouldNotPrependHttpIfTheTargetIsHttps(c *C) {
	rfs := &testing.RecordingFs{FileContent: "https://localhost"}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	got := GetUrl("/apps")
	c.Assert(got, Equals, "https://localhost/apps")
}
