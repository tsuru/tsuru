// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"bytes"
	"github.com/globocom/tsuru/fs/testing"
	"io/ioutil"
	"launchpad.net/gocheck"
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

func (s *S) TestWriteTarget(c *gocheck.C) {
	rfs := &testing.RecordingFs{}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	err := writeTarget("http://tsuru.globo.com")
	c.Assert(err, gocheck.IsNil)
	filePath := path.Join(os.ExpandEnv("${HOME}"), ".tsuru_target")
	c.Assert(rfs.HasAction("openfile "+filePath+" with mode 0600"), gocheck.Equals, true)
	c.Assert(readRecordedTarget(rfs), gocheck.Equals, "http://tsuru.globo.com")
}

func (s *S) TestWriteTargetShouldStripLeadingSlashs(c *gocheck.C) {
	rfs := &testing.RecordingFs{}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	err := writeTarget("http://tsuru.globo.com/")
	c.Assert(err, gocheck.IsNil)
	c.Assert(readRecordedTarget(rfs), gocheck.Equals, "http://tsuru.globo.com")
}

func (s *S) TestWriteTargetShouldStripAllLeadingSlashs(c *gocheck.C) {
	rfs := &testing.RecordingFs{}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	err := writeTarget("http://tsuru.globo.com////")
	c.Assert(err, gocheck.IsNil)
	target := readRecordedTarget(rfs)
	c.Assert(target, gocheck.Equals, "http://tsuru.globo.com")
}

func (s *S) TestReadTarget(c *gocheck.C) {
	rfs := &testing.RecordingFs{FileContent: "http://tsuru.google.com"}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	target, err := readTarget()
	c.Assert(err, gocheck.IsNil)
	c.Assert(target, gocheck.Equals, "http://tsuru.google.com")
}

func (s *S) TestReadTargetReturnsEmptyStringIfTheFileDoesNotExist(c *gocheck.C) {
	fsystem = &testing.FailureFs{}
	defer func() {
		fsystem = nil
	}()
	target, err := readTarget()
	c.Assert(target, gocheck.Equals, "")
	c.Assert(err, gocheck.NotNil)
	_, ok := err.(undefinedTargetError)
	c.Assert(ok, gocheck.Equals, true)
}

func (s *S) TestReadTargetTrimsFileContent(c *gocheck.C) {
	fsystem = &testing.RecordingFs{FileContent: "   http://tsuru.io\n\n"}
	defer func() {
		fsystem = nil
	}()
	target, err := readTarget()
	c.Assert(err, gocheck.IsNil)
	c.Assert(target, gocheck.Equals, "http://tsuru.io")
}

func (s *S) TestTargetInfo(c *gocheck.C) {
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
	c.Assert(target.Info(), gocheck.DeepEquals, expected)
}

func (s *S) TestTargetRun(c *gocheck.C) {
	context := &Context{[]string{"http://tsuru.globo.com"}, manager.stdout, manager.stderr, manager.stdin}
	target := &target{}
	err := target.Run(context, nil)
	c.Assert(err, gocheck.IsNil)
	c.Assert(context.Stdout.(*bytes.Buffer).String(), gocheck.Equals, "To add a new target use target-add\n")
}

func (s *S) TestTargetWithoutArgument(c *gocheck.C) {
	rfs := &testing.RecordingFs{FileContent: "http://tsuru.google.com"}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	context := &Context{[]string{}, manager.stdout, manager.stderr, manager.stdin}
	target := &target{}
	err := target.Run(context, nil)
	c.Assert(err, gocheck.IsNil)
	c.Assert(context.Stdout.(*bytes.Buffer).String(), gocheck.Equals, "Current target is http://tsuru.google.com\n")
}

func (s *S) TestGetUrl(c *gocheck.C) {
	fsystem = &testing.RecordingFs{FileContent: "http://localhost"}
	defer func() {
		fsystem = nil
	}()
	expected := "http://localhost/apps"
	got, err := GetUrl("/apps")
	c.Assert(err, gocheck.IsNil)
	c.Assert(got, gocheck.Equals, expected)
}

func (s *S) TestGetUrlPutsHttpIfItIsNotPresent(c *gocheck.C) {
	rfs := &testing.RecordingFs{FileContent: "remotehost"}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	expected := "http://remotehost/apps"
	got, err := GetUrl("/apps")
	c.Assert(err, gocheck.IsNil)
	c.Assert(got, gocheck.Equals, expected)
}

func (s *S) TestGetUrlShouldNotPrependHttpIfTheTargetIsHttps(c *gocheck.C) {
	rfs := &testing.RecordingFs{FileContent: "https://localhost"}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	got, err := GetUrl("/apps")
	c.Assert(err, gocheck.IsNil)
	c.Assert(got, gocheck.Equals, "https://localhost/apps")
}

func (s *S) TestGetUrlUndefinedTarget(c *gocheck.C) {
	rfs := &testing.FailureFs{}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	got, err := GetUrl("/apps")
	c.Assert(got, gocheck.Equals, "")
	c.Assert(err, gocheck.NotNil)
	_, ok := err.(undefinedTargetError)
	c.Assert(ok, gocheck.Equals, true)
}

func (s *S) TestTargetAddInfo(c *gocheck.C) {
	desc := `Add a new target on target-list (tsuru server)
`
	expected := &Info{
		Name:    "target-add",
		Usage:   "target-add <label> <target>",
		Desc:    desc,
		MinArgs: 2,
	}
	targetAdd := &targetAdd{}
	c.Assert(targetAdd.Info(), gocheck.DeepEquals, expected)
}

func (s *S) TestTargetAddRun(c *gocheck.C) {
	rfs := &testing.RecordingFs{FileContent: "default   http://tsuru.google.com"}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	context := &Context{[]string{"default", "http://tsuru.google.com"}, manager.stdout, manager.stderr, manager.stdin}
	targetAdd := &targetAdd{}
	err := targetAdd.Run(context, nil)
	c.Assert(err, gocheck.IsNil)
	c.Assert(context.Stdout.(*bytes.Buffer).String(), gocheck.Equals, "New target default -> http://tsuru.google.com added to target-list\n")
}

func (s *S) TestTargetAddRunOnlyOneArg(c *gocheck.C) {
	rfs := &testing.RecordingFs{FileContent: "default   http://tsuru.google.com"}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	context := &Context{[]string{"default http://tsuru.google.com"}, manager.stdout, manager.stderr, manager.stdin}
	targetAdd := &targetAdd{}
	err := targetAdd.Run(context, nil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "Invalid arguments")
}

func (s *S) TestIfTargetLabelExists(c *gocheck.C) {
	rfs := &testing.RecordingFs{FileContent: "first\thttp://tsuru.io/\ndefault\thttp://tsuru.google.com"}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	mustBeTrueIfExist, err := checkIfTargetLabelExists("default")
	c.Assert(err, gocheck.IsNil)
	c.Assert(mustBeTrueIfExist, gocheck.Equals, true)
}

func (s *S) TestIfTargetLabelDoesNotExists(c *gocheck.C) {
	rfs := &testing.RecordingFs{FileContent: "first\thttp://tsuru.io/\ndefault\thttp://tsuru.google.com"}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	mustBeFalse, err := checkIfTargetLabelExists("doesnotexist")
	c.Assert(err, gocheck.IsNil)
	c.Assert(mustBeFalse, gocheck.Equals, false)
}

func (s *S) TestGetTargets(c *gocheck.C) {
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
	c.Assert(err, gocheck.IsNil)
	c.Assert(len(got), gocheck.Equals, len(expected))
	for k, v := range got {
		c.Assert(expected[k], gocheck.Equals, v)
	}
}

func (s *S) TestTargetListInfo(c *gocheck.C) {
	desc := `List all targets (tsuru server)
`
	expected := &Info{
		Name:    "target-list",
		Usage:   "target-list",
		Desc:    desc,
		MinArgs: 0,
	}
	targetList := &targetList{}
	c.Assert(targetList.Info(), gocheck.DeepEquals, expected)
}

func (s *S) TestTargetListRun(c *gocheck.C) {
	rfs := &testing.RecordingFs{FileContent: "first\thttp://tsuru.io/\ndefault\thttp://tsuru.google.com"}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	expected := `+---------+-------------------------+
| default | http://tsuru.google.com |
| first   | http://tsuru.io/        |
+---------+-------------------------+
`
	targetList := &targetList{}
	context := &Context{[]string{""}, manager.stdout, manager.stderr, manager.stdin}
	err := targetList.Run(context, nil)
	c.Assert(err, gocheck.IsNil)
	got := context.Stdout.(*bytes.Buffer).String()
	c.Assert(got, gocheck.Equals, expected)
}

func (s *S) TestResetTargetList(c *gocheck.C) {
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
	c.Assert(err, gocheck.IsNil)
	c.Assert(len(got), gocheck.Equals, len(expected))
	err = resetTargetList()
	c.Assert(err, gocheck.IsNil)
	got, err = getTargets()
	c.Assert(err, gocheck.IsNil)
	c.Assert(got, gocheck.DeepEquals, map[string]string{})
}

func (s *S) TestTargetRemoveInfo(c *gocheck.C) {
	desc := `Remove a target from target-list (tsuru server)
`
	expected := &Info{
		Name:    "target-remove",
		Usage:   "target-remove",
		Desc:    desc,
		MinArgs: 1,
	}
	targetRemove := &targetRemove{}
	c.Assert(targetRemove.Info(), gocheck.DeepEquals, expected)
}

func (s *S) TestTargetRemove(c *gocheck.C) {
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
	c.Assert(err, gocheck.IsNil)
	c.Assert(len(got), gocheck.Equals, len(expectedBefore))
	targetRemove := &targetRemove{}
	context := &Context{[]string{"first"}, manager.stdout, manager.stderr, manager.stdin}
	err = targetRemove.Run(context, nil)
	c.Assert(err, gocheck.IsNil)
	got, err = getTargets()
	c.Assert(err, gocheck.IsNil)
	c.Assert(len(got), gocheck.Equals, len(expectedAfter))
	_, hasKey := got["default"]
	c.Assert(hasKey, gocheck.Equals, true)
	_, hasKey = got["first"]
	c.Assert(hasKey, gocheck.Equals, false)
}

func (s *S) TestTargetSetInfo(c *gocheck.C) {
	desc := `Change current target (tsuru server)
`
	expected := &Info{
		Name:    "target-set",
		Usage:   "target-set <label>",
		Desc:    desc,
		MinArgs: 1,
	}
	targetSet := &targetSet{}
	c.Assert(targetSet.Info(), gocheck.DeepEquals, expected)
}

func (s *S) TestTargetSetRun(c *gocheck.C) {
	rfs := &testing.RecordingFs{FileContent: "first\thttp://tsuru.io/\ndefault\thttp://tsuru.google.com"}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	targetSet := &targetSet{}
	context := &Context{[]string{"default"}, manager.stdout, manager.stderr, manager.stdin}
	err := targetSet.Run(context, nil)
	c.Assert(err, gocheck.IsNil)
	got := context.Stdout.(*bytes.Buffer).String()
	c.Assert(strings.Contains(got, "New target is default -> http://tsuru.google.com\n"), gocheck.Equals, true)
}

func (s *S) TestTargetSetRunUnknowTarget(c *gocheck.C) {
	rfs := &testing.RecordingFs{FileContent: "first\thttp://tsuru.io/\ndefault\thttp://tsuru.google.com"}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	targetSet := &targetSet{}
	context := &Context{[]string{"doesnotexist"}, manager.stdout, manager.stderr, manager.stdin}
	err := targetSet.Run(context, nil)
	c.Assert(err, gocheck.ErrorMatches, "Target not found")
}

func (s *S) TestUndefinedTarget(c *gocheck.C) {
	expectedMsg := `No target defined. Please use target-add/target-set to define a target.

For more details, please run "tsuru help target".`
	var e error = undefinedTargetError{}
	c.Assert(e.Error(), gocheck.Equals, expectedMsg)
}
