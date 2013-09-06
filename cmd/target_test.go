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

func (s *S) TestWriteTargetKeepsLeadingSlashs(c *gocheck.C) {
	rfs := &testing.RecordingFs{}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	err := writeTarget("http://tsuru.globo.com//")
	c.Assert(err, gocheck.IsNil)
	c.Assert(readRecordedTarget(rfs), gocheck.Equals, "http://tsuru.globo.com//")
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

func (s *S) TestDeleteTargetFile(c *gocheck.C) {
	rfs := &testing.RecordingFs{FileContent: "   http://tsuru.io\n\n"}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	deleteTargetFile()
	targetFile := joinWithUserDir(".tsuru_target")
	c.Assert(rfs.HasAction("remove "+targetFile), gocheck.Equals, true)
}

func (s *S) TestGetURL(c *gocheck.C) {
	fsystem = &testing.RecordingFs{FileContent: "http://localhost"}
	defer func() {
		fsystem = nil
	}()
	expected := "http://localhost/apps"
	got, err := GetURL("/apps")
	c.Assert(err, gocheck.IsNil)
	c.Assert(got, gocheck.Equals, expected)
}

func (s *S) TestGetURLPutsHTTPIfItIsNotPresent(c *gocheck.C) {
	rfs := &testing.RecordingFs{FileContent: "remotehost"}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	expected := "http://remotehost/apps"
	got, err := GetURL("/apps")
	c.Assert(err, gocheck.IsNil)
	c.Assert(got, gocheck.Equals, expected)
}

func (s *S) TestGetURLShouldNotPrependHTTPIfTheTargetIsHTTPs(c *gocheck.C) {
	rfs := &testing.RecordingFs{FileContent: "https://localhost"}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	got, err := GetURL("/apps")
	c.Assert(err, gocheck.IsNil)
	c.Assert(got, gocheck.Equals, "https://localhost/apps")
}

func (s *S) TestGetURLUndefinedTarget(c *gocheck.C) {
	rfs := &testing.FailureFs{}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	got, err := GetURL("/apps")
	c.Assert(got, gocheck.Equals, "")
	c.Assert(err, gocheck.NotNil)
	_, ok := err.(undefinedTargetError)
	c.Assert(ok, gocheck.Equals, true)
}

func (s *S) TestGetURLLeadingSlashes(c *gocheck.C) {
	rfs := &testing.RecordingFs{FileContent: "https://localhost/tsuru/"}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	got, err := GetURL("/apps")
	c.Assert(err, gocheck.IsNil)
	c.Assert(got, gocheck.Equals, "https://localhost/tsuru/apps")
}

func (s *S) TestTargetAddInfo(c *gocheck.C) {
	expected := &Info{
		Name:    "target-add",
		Usage:   "target-add <label> <target> [--set-current|-s]",
		Desc:    "Adds a new entry to the list of available targets",
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
	c.Assert(context.Stdout.(*bytes.Buffer).String(), gocheck.Equals, "New target default -> http://tsuru.google.com added to target list\n")
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

func (s *S) TestTargetAddWithSet(c *gocheck.C) {
	rfs := &testing.RecordingFs{FileContent: "old\thttp://tsuru.io"}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	context := &Context{[]string{"default", "http://tsuru.google.com"}, manager.stdout, manager.stderr, manager.stdin}
	targetAdd := &targetAdd{}
	targetAdd.Flags().Parse(true, []string{"-s"})
	err := targetAdd.Run(context, nil)
	c.Assert(err, gocheck.IsNil)
	c.Assert(context.Stdout.(*bytes.Buffer).String(), gocheck.Equals, "New target default -> http://tsuru.google.com added to target list and defined as the current target\n")
	t, err := readTarget()
	c.Assert(err, gocheck.IsNil)
	c.Assert(t, gocheck.Equals, "http://tsuru.google.com")
}

func (s *S) TestTargetAddFlags(c *gocheck.C) {
	command := targetAdd{}
	flagset := command.Flags()
	c.Assert(flagset, gocheck.NotNil)
	flagset.Parse(true, []string{"--set-current"})
	set := flagset.Lookup("set-current")
	c.Assert(set, gocheck.NotNil)
	c.Check(set.Name, gocheck.Equals, "set-current")
	c.Check(set.Usage, gocheck.Equals, "Add and define the target as the current target")
	c.Check(set.Value.String(), gocheck.Equals, "true")
	c.Check(set.DefValue, gocheck.Equals, "false")
	sset := flagset.Lookup("s")
	c.Assert(sset, gocheck.NotNil)
	c.Check(sset.Name, gocheck.Equals, "s")
	c.Check(sset.Usage, gocheck.Equals, "Add and define the target as the current target")
	c.Check(sset.Value.String(), gocheck.Equals, "true")
	c.Check(sset.DefValue, gocheck.Equals, "false")
	c.Check(command.set, gocheck.Equals, true)
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

func (s *S) TestIfTargetLabelDoesNotExist(c *gocheck.C) {
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

func (s *S) TestTargetInfo(c *gocheck.C) {
	desc := `Displays the list of targets, marking the current.

Other commands related to target:

  - target-add: adds a new target to the list of targets
  - target-set: defines one of the targets in the list as the current target
  - target-remove: removes one target from the list`
	expected := &Info{
		Name:    "target-list",
		Usage:   "target-list",
		Desc:    desc,
		MinArgs: 0,
	}
	target := &targetList{}
	c.Assert(target.Info(), gocheck.DeepEquals, expected)
}

func (s *S) TestTargetRun(c *gocheck.C) {
	content := `first	http://tsuru.io
default	http://tsuru.google.com
other	http://other.tsuru.io`
	rfs := &testing.RecordingFs{}
	f, _ := rfs.Create(joinWithUserDir(".tsuru_target"))
	f.Write([]byte("http://tsuru.io"))
	f.Close()
	f, _ = rfs.Create(joinWithUserDir(".tsuru_targets"))
	f.Write([]byte(content))
	f.Close()
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	expected := `  default (http://tsuru.google.com)
* first (http://tsuru.io)
  other (http://other.tsuru.io)` + "\n"
	target := &targetList{}
	context := &Context{[]string{""}, manager.stdout, manager.stderr, manager.stdin}
	err := target.Run(context, nil)
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
	f, _ := rfs.Create(joinWithUserDir(".tsuru_target"))
	f.Write([]byte("http://tsuru.google.com"))
	f.Close()
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

func (s *S) TestTargetRemoveCurrentTarget(c *gocheck.C) {
	rfs := &testing.RecordingFs{}
	f, _ := rfs.Create(joinWithUserDir(".tsuru_targets"))
	f.Write([]byte("first\thttp://tsuru.io/\ndefault\thttp://tsuru.google.com"))
	f.Close()
	f, _ = rfs.Create(joinWithUserDir(".tsuru_target"))
	f.Write([]byte("http://tsuru.google.com"))
	f.Close()
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	targetRemove := &targetRemove{}
	context := &Context{[]string{"default"}, manager.stdout, manager.stderr, manager.stdin}
	err := targetRemove.Run(context, nil)
	c.Assert(err, gocheck.IsNil)
	_, err = readTarget()
	c.Assert(err, gocheck.NotNil)
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

func (s *S) TestNewTargetSlice(c *gocheck.C) {
	t := newTargetSlice()
	c.Assert(t.sorted, gocheck.Equals, false)
	c.Assert(t.current, gocheck.Equals, -1)
	c.Assert(t.targets, gocheck.IsNil)
}

func (s *S) TestTargetSliceAdd(c *gocheck.C) {
	var t targetSlice
	t.sorted = true
	t.add("default", "http://tsuru.io")
	c.Assert(t.targets, gocheck.DeepEquals, []tsuruTarget{{label: "default", url: "http://tsuru.io"}})
	c.Assert(t.sorted, gocheck.Equals, true)
	t.add("abc", "http://tsuru.io")
	c.Assert(t.sorted, gocheck.Equals, false)
}

func (s *S) TestTargetSliceLen(c *gocheck.C) {
	t := targetSlice{
		targets: []tsuruTarget{{label: "default", url: ""}},
	}
	c.Assert(t.Len(), gocheck.Equals, len(t.targets))
}

func (s *S) TestTargetSliceLess(c *gocheck.C) {
	t := targetSlice{
		targets: []tsuruTarget{
			{label: "first", url: ""},
			{label: "default", url: ""},
			{label: "second", url: ""},
		},
	}
	c.Check(t.Less(0, 1), gocheck.Equals, false)
	c.Check(t.Less(0, 2), gocheck.Equals, true)
	c.Check(t.Less(1, 0), gocheck.Equals, true)
	c.Check(t.Less(1, 2), gocheck.Equals, true)
	c.Check(t.Less(2, 0), gocheck.Equals, false)
}

func (s *S) TestTargetSliceSwap(c *gocheck.C) {
	t := targetSlice{
		targets: []tsuruTarget{
			{label: "first", url: ""},
			{label: "default", url: ""},
			{label: "second", url: ""},
		},
	}
	c.Assert(t.Less(0, 1), gocheck.Equals, false)
	t.Swap(0, 1)
	c.Assert(t.Less(0, 1), gocheck.Equals, true)
}

func (s *S) TestTargetSliceSort(c *gocheck.C) {
	t := targetSlice{
		targets: []tsuruTarget{
			{label: "first", url: ""},
			{label: "default", url: ""},
			{label: "second", url: ""},
		},
	}
	t.Sort()
	c.Assert(t.Less(0, 1), gocheck.Equals, true)
	c.Assert(t.Less(1, 2), gocheck.Equals, true)
	c.Assert(t.sorted, gocheck.Equals, true)
}

func (s *S) TestTargetSliceSetCurrent(c *gocheck.C) {
	t := targetSlice{
		targets: []tsuruTarget{
			{label: "first", url: "first.tsuru.io"},
			{label: "default", url: "default.tsuru.io"},
			{label: "second", url: "second.tsuru.io"},
		},
		current: -1,
	}
	t.setCurrent("unknown.tsuru.io")
	c.Check(t.current, gocheck.Equals, -1)
	t.setCurrent("first.tsuru.io")
	c.Check(t.current, gocheck.Equals, 1) // sort the slice
	t.setCurrent("second.tsuru.io")
	c.Check(t.current, gocheck.Equals, 2)
	t.setCurrent("unknown.tsuru.io")
	c.Check(t.current, gocheck.Equals, 2)
}

func (s *S) TestTargetSliceStringSortedNoCurrent(c *gocheck.C) {
	expected := `  default (default.tsuru.io)
  first (first.tsuru.io)
  second (second.tsuru.io)`
	t := targetSlice{
		targets: []tsuruTarget{
			{label: "first", url: "first.tsuru.io"},
			{label: "default", url: "default.tsuru.io"},
			{label: "second", url: "second.tsuru.io"},
		},
		current: -1,
	}
	t.Sort()
	c.Assert(t.String(), gocheck.Equals, expected)
}

func (s *S) TestTargetSliceStringUnsortedNoCurrent(c *gocheck.C) {
	expected := `  default (default.tsuru.io)
  first (first.tsuru.io)
  second (second.tsuru.io)`
	t := targetSlice{
		targets: []tsuruTarget{
			{label: "first", url: "first.tsuru.io"},
			{label: "default", url: "default.tsuru.io"},
			{label: "second", url: "second.tsuru.io"},
		},
		current: -1,
	}
	c.Assert(t.String(), gocheck.Equals, expected)
}

func (s *S) TestTargetSliceStringSortedWithCurrent(c *gocheck.C) {
	expected := `  default (default.tsuru.io)
  first (first.tsuru.io)
* second (second.tsuru.io)`
	t := targetSlice{
		targets: []tsuruTarget{
			{label: "first", url: "first.tsuru.io"},
			{label: "default", url: "default.tsuru.io"},
			{label: "second", url: "second.tsuru.io"},
		},
		current: 2,
	}
	t.Sort()
	c.Assert(t.String(), gocheck.Equals, expected)
}

func (s *S) TestTargetSliceStringUnsortedWithCurrent(c *gocheck.C) {
	expected := `  default (default.tsuru.io)
* first (first.tsuru.io)
  second (second.tsuru.io)`
	t := targetSlice{
		targets: []tsuruTarget{
			{label: "first", url: "first.tsuru.io"},
			{label: "default", url: "default.tsuru.io"},
			{label: "second", url: "second.tsuru.io"},
		},
		current: 1,
	}
	c.Assert(t.String(), gocheck.Equals, expected)
}
