// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"bytes"
	"github.com/globocom/tsuru/fs/testing"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"os"
	"path"
	"strings"
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

func (s *S) TestReadTargetTrimsFileContent(c *C) {
	fsystem = &testing.RecordingFs{FileContent: "   http://tsuru.io\n\n"}
	defer func() {
		fsystem = nil
	}()
	target := readTarget()
	c.Assert(target, Equals, "http://tsuru.io")
}

func (s *S) TestTargetInfo(c *C) {
	desc := `Retrieve current target (tsuru server)

Displays the current target.
`
	expected := &Info{
		Name:    "target",
		Usage:   "target",
		Desc:    desc,
		MinArgs: 0,
	}
	target := &target{}
	c.Assert(target.Info(), DeepEquals, expected)
}

func (s *S) TestTargetRun(c *C) {
	context := &Context{[]string{"http://tsuru.globo.com"}, manager.stdout, manager.stderr, manager.stdin}
	target := &target{}
	err := target.Run(context, nil)
	c.Assert(err, IsNil)
	c.Assert(context.Stdout.(*bytes.Buffer).String(), Equals, "To add a new target use target-add\n")
}

func (s *S) TestTargetWithoutArgument(c *C) {
	rfs := &testing.RecordingFs{FileContent: "http://tsuru.google.com"}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	context := &Context{[]string{}, manager.stdout, manager.stderr, manager.stdin}
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

func (s *S) TestTargetAddInfo(c *C) {
	desc := `Add a new target on target-list (tsuru server)
`
	expected := &Info{
		Name:    "target-add",
		Usage:   "target-add <label> <target>",
		Desc:    desc,
		MinArgs: 2,
	}
	targetAdd := &targetAdd{}
	c.Assert(targetAdd.Info(), DeepEquals, expected)
}

func (s *S) TestTargetAddRun(c *C) {
	rfs := &testing.RecordingFs{FileContent: "default   http://tsuru.google.com"}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	context := &Context{[]string{"default", "http://tsuru.google.com"}, manager.stdout, manager.stderr, manager.stdin}
	targetAdd := &targetAdd{}
	err := targetAdd.Run(context, nil)
	c.Assert(err, IsNil)
	c.Assert(context.Stdout.(*bytes.Buffer).String(), Equals, "New target default -> http://tsuru.google.com added to target-list\n")
}

func (s *S) TestTargetAddRunOnlyOneArg(c *C) {
	rfs := &testing.RecordingFs{FileContent: "default   http://tsuru.google.com"}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	context := &Context{[]string{"default http://tsuru.google.com"}, manager.stdout, manager.stderr, manager.stdin}
	targetAdd := &targetAdd{}
	err := targetAdd.Run(context, nil)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "Invalid arguments")
}

func (s *S) TestIfTargetLabelExists(c *C) {
	rfs := &testing.RecordingFs{FileContent: "first\thttp://tsuru.io/\ndefault\thttp://tsuru.google.com"}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	mustBeTrueIfExist, err := checkIfTargetLabelExists("default")
	c.Assert(err, IsNil)
	c.Assert(mustBeTrueIfExist, Equals, true)
}

func (s *S) TestIfTargetLabelDoesNotExists(c *C) {
	rfs := &testing.RecordingFs{FileContent: "first\thttp://tsuru.io/\ndefault\thttp://tsuru.google.com"}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	mustBeFalse, err := checkIfTargetLabelExists("doesnotexist")
	c.Assert(err, IsNil)
	c.Assert(mustBeFalse, Equals, false)
}

func (s *S) TestGetTargets(c *C) {
	rfs := &testing.RecordingFs{FileContent: "first\thttp://tsuru.io/\ndefault\thttp://tsuru.google.com"}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	var expected = map[string]string{
		"first":   "http://tsuru.io/",
		"default": "http://tsuru.google.com",
	}
	got, err := getTargets()
	c.Assert(err, IsNil)
	c.Assert(len(got), Equals, len(expected))
	for k, v := range got {
		c.Assert(expected[k], Equals, v)
	}
}

func (s *S) TestTargetListInfo(c *C) {
	desc := `List all targets (tsuru server)
`
	expected := &Info{
		Name:    "target-list",
		Usage:   "target-list",
		Desc:    desc,
		MinArgs: 0,
	}
	targetList := &targetList{}
	c.Assert(targetList.Info(), DeepEquals, expected)
}

func (s *S) TestTargetListRun(c *C) {
	rfs := &testing.RecordingFs{FileContent: "first\thttp://tsuru.io/\ndefault\thttp://tsuru.google.com"}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	expected := []string{"+---------+-------------------------+",
		"| first   | http://tsuru.io/        |",
		"| default | http://tsuru.google.com |",
		"+---------+-------------------------+", ""}
	targetList := &targetList{}
	context := &Context{[]string{""}, manager.stdout, manager.stderr, manager.stdin}
	err := targetList.Run(context, nil)
	c.Assert(err, IsNil)
	got := context.Stdout.(*bytes.Buffer).String()
	for i := range expected {
		c.Assert(strings.Contains(got, expected[i]), Equals, true)
	}
}

func (s *S) TestResetTargetList(c *C) {
	rfs := &testing.RecordingFs{FileContent: "first\thttp://tsuru.io/\ndefault\thttp://tsuru.google.com"}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	var expected = map[string]string{
		"first":   "http://tsuru.io/",
		"default": "http://tsuru.google.com",
	}
	got, err := getTargets()
	c.Assert(err, IsNil)
	c.Assert(len(got), Equals, len(expected))
	err = resetTargetList()
	c.Assert(err, IsNil)
	got, err = getTargets()
	c.Assert(err, IsNil)
	c.Assert(got, DeepEquals, map[string]string{})
}

func (s *S) TestTargetRemoveInfo(c *C) {
	desc := `Remove a target from target-list (tsuru server)
`
	expected := &Info{
		Name:    "target-remove",
		Usage:   "target-remove",
		Desc:    desc,
		MinArgs: 1,
	}
	targetRemove := &targetRemove{}
	c.Assert(targetRemove.Info(), DeepEquals, expected)
}

func (s *S) TestTargetRemove(c *C) {
	rfs := &testing.RecordingFs{FileContent: "first\thttp://tsuru.io/\ndefault\thttp://tsuru.google.com"}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	var expectedBefore = map[string]string{
		"first":   "http://tsuru.io/",
		"default": "http://tsuru.google.com",
	}
	var expectedAfter = map[string]string{
		"default": "http://tsuru.google.com",
	}
	got, err := getTargets()
	c.Assert(err, IsNil)
	c.Assert(len(got), Equals, len(expectedBefore))
	targetRemove := &targetRemove{}
	context := &Context{[]string{"first"}, manager.stdout, manager.stderr, manager.stdin}
	err = targetRemove.Run(context, nil)
	c.Assert(err, IsNil)
	got, err = getTargets()
	c.Assert(err, IsNil)
	c.Assert(len(got), Equals, len(expectedAfter))
	_, hasKey := got["default"]
	c.Assert(hasKey, Equals, true)
	_, hasKey = got["first"]
	c.Assert(hasKey, Equals, false)
}

func (s *S) TestTargetSetInfo(c *C) {
	desc := `Change current target (tsuru server)
`
	expected := &Info{
		Name:    "target-set",
		Usage:   "target-set <label>",
		Desc:    desc,
		MinArgs: 1,
	}
	targetSet := &targetSet{}
	c.Assert(targetSet.Info(), DeepEquals, expected)
}

func (s *S) TestTargetSetRun(c *C) {
	rfs := &testing.RecordingFs{FileContent: "first\thttp://tsuru.io/\ndefault\thttp://tsuru.google.com"}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	targetSet := &targetSet{}
	context := &Context{[]string{"default"}, manager.stdout, manager.stderr, manager.stdin}
	err := targetSet.Run(context, nil)
	c.Assert(err, IsNil)
	got := context.Stdout.(*bytes.Buffer).String()
	c.Assert(strings.Contains(got, "New target is default -> http://tsuru.google.com\n"), Equals, true)
}

func (s *S) TestTargetSetRunUnknowTarget(c *C) {
	rfs := &testing.RecordingFs{FileContent: "first\thttp://tsuru.io/\ndefault\thttp://tsuru.google.com"}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	targetSet := &targetSet{}
	context := &Context{[]string{"doesnotexist"}, manager.stdout, manager.stderr, manager.stdin}
	err := targetSet.Run(context, nil)
	c.Assert(err, ErrorMatches, "Target not found")
}
