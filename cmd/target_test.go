package cmd

import (
	"bytes"
	"github.com/timeredbull/tsuru/fs/testing"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"os"
	"path"
)

func readTarget(fs *testing.RecordingFs) string {
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
	err := WriteTarget("http://tsuru.globo.com")
	c.Assert(err, IsNil)
	filePath := path.Join(os.ExpandEnv("${HOME}"), ".tsuru_target")
	c.Assert(rfs.HasAction("openfile "+filePath+" with mode 0600"), Equals, true)
	c.Assert(readTarget(rfs), Equals, "http://tsuru.globo.com")
}

func (s *S) TestWriteTargetShouldStripLeadingSlashs(c *C) {
	rfs := &testing.RecordingFs{}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	err := WriteTarget("http://tsuru.globo.com/")
	c.Assert(err, IsNil)
	c.Assert(readTarget(rfs), Equals, "http://tsuru.globo.com")
}

func (s *S) TestWriteTargetShouldStripAllLeadingSlashs(c *C) {
	rfs := &testing.RecordingFs{}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	err := WriteTarget("http://tsuru.globo.com////")
	c.Assert(err, IsNil)
	target := readTarget(rfs)
	c.Assert(target, Equals, "http://tsuru.globo.com")
}

func (s *S) TestReadTarget(c *C) {
	rfs := &testing.RecordingFs{FileContent: "http://tsuru.google.com"}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	target := ReadTarget()
	c.Assert(target, Equals, "http://tsuru.google.com")
}

func (s *S) TestReadTargetReturnsDefaultTargetIfTheFileDoesNotExist(c *C) {
	fsystem = &testing.FailureFs{}
	defer func() {
		fsystem = nil
	}()
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
	rfs := &testing.RecordingFs{FileContent: "http://tsuru.google.com"}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	context := &Context{[]string{}, []string{"http://tsuru.globo.com"}, manager.Stdout, manager.Stderr}
	target := &Target{}
	err := target.Run(context, nil)
	c.Assert(err, IsNil)
	c.Assert(context.Stdout.(*bytes.Buffer).String(), Equals, "New target is http://tsuru.globo.com\n")
	c.Assert(ReadTarget(), Equals, "http://tsuru.globo.com")
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
