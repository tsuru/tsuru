// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"bytes"
	"io/ioutil"
	"os"
	"path"
	"strings"

	"github.com/tsuru/tsuru/fs/fstest"
	"gopkg.in/check.v1"
)

func readRecordedTarget(fs *fstest.RecordingFs) string {
	filePath := path.Join(os.ExpandEnv("${HOME}"), ".tsuru_target")
	fil, _ := fsystem.Open(filePath)
	b, _ := ioutil.ReadAll(fil)
	return string(b)
}

func (s *S) TestWriteTarget(c *check.C) {
	rfs := &fstest.RecordingFs{}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	err := writeTarget("http://tsuru.globo.com")
	c.Assert(err, check.IsNil)
	filePath := path.Join(os.ExpandEnv("${HOME}"), ".tsuru_target")
	c.Assert(rfs.HasAction("openfile "+filePath+" with mode 0600"), check.Equals, true)
	c.Assert(readRecordedTarget(rfs), check.Equals, "http://tsuru.globo.com")
}

func (s *S) TestWriteTargetKeepsLeadingSlashs(c *check.C) {
	rfs := &fstest.RecordingFs{}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	err := writeTarget("http://tsuru.globo.com//")
	c.Assert(err, check.IsNil)
	c.Assert(readRecordedTarget(rfs), check.Equals, "http://tsuru.globo.com//")
}

func (s *S) TestReadTarget(c *check.C) {
	rfs := &fstest.RecordingFs{FileContent: "http://tsuru.google.com"}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	target, err := ReadTarget()
	c.Assert(err, check.IsNil)
	c.Assert(target, check.Equals, "http://tsuru.google.com")
}

func (s *S) TestReadTargetEnvironmentVariable(c *check.C) {
	rfs := &fstest.RecordingFs{FileContent: "http://tsuru.google.com"}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	os.Setenv("TSURU_TARGET", "https://tsuru.google.com")
	defer os.Setenv("TSURU_TARGET", "")
	target, err := ReadTarget()
	c.Assert(err, check.IsNil)
	c.Assert(target, check.Equals, "https://tsuru.google.com")
}

func (s *S) TestReadTargetReturnsEmptyStringIfTheFileDoesNotExist(c *check.C) {
	fsystem = &fstest.FileNotFoundFs{}
	defer func() {
		fsystem = nil
	}()
	target, err := ReadTarget()
	c.Assert(target, check.Equals, "")
	c.Assert(err, check.Equals, errUndefinedTarget)
}

func (s *S) TestReadTargetTrimsFileContent(c *check.C) {
	fsystem = &fstest.RecordingFs{FileContent: "   http://tsuru.io\n\n"}
	defer func() {
		fsystem = nil
	}()
	target, err := ReadTarget()
	c.Assert(err, check.IsNil)
	c.Assert(target, check.Equals, "http://tsuru.io")
}

func (s *S) TestDeleteTargetFile(c *check.C) {
	rfs := &fstest.RecordingFs{FileContent: "   http://tsuru.io\n\n"}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	deleteTargetFile()
	targetFile := JoinWithUserDir(".tsuru_target")
	c.Assert(rfs.HasAction("remove "+targetFile), check.Equals, true)
}

func (s *S) TestGetURL(c *check.C) {
	fsystem = &fstest.RecordingFs{FileContent: "http://localhost"}
	defer func() {
		fsystem = nil
	}()
	expected := "http://localhost/apps"
	got, err := GetURL("/apps")
	c.Assert(err, check.IsNil)
	c.Assert(got, check.Equals, expected)
}

func (s *S) TestGetURLPutsHTTPIfItIsNotPresent(c *check.C) {
	rfs := &fstest.RecordingFs{FileContent: "remotehost"}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	expected := "http://remotehost/apps"
	got, err := GetURL("/apps")
	c.Assert(err, check.IsNil)
	c.Assert(got, check.Equals, expected)
}

func (s *S) TestGetURLShouldNotPrependHTTPIfTheTargetIsHTTPs(c *check.C) {
	rfs := &fstest.RecordingFs{FileContent: "https://localhost"}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	got, err := GetURL("/apps")
	c.Assert(err, check.IsNil)
	c.Assert(got, check.Equals, "https://localhost/apps")
}

func (s *S) TestGetURLUndefinedTarget(c *check.C) {
	rfs := &fstest.FileNotFoundFs{}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	got, err := GetURL("/apps")
	c.Assert(got, check.Equals, "")
	c.Assert(err, check.Equals, errUndefinedTarget)
}

func (s *S) TestGetURLLeadingSlashes(c *check.C) {
	rfs := &fstest.RecordingFs{FileContent: "https://localhost/tsuru/"}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	got, err := GetURL("/apps")
	c.Assert(err, check.IsNil)
	c.Assert(got, check.Equals, "https://localhost/tsuru/apps")
}

func (s *S) TestTargetAddInfo(c *check.C) {
	expected := &Info{
		Name:    "target-add",
		Usage:   "target-add <label> <target> [--set-current|-s]",
		Desc:    "Adds a new entry to the list of available targets",
		MinArgs: 2,
	}
	targetAdd := &targetAdd{}
	c.Assert(targetAdd.Info(), check.DeepEquals, expected)
}

func (s *S) TestTargetAddRun(c *check.C) {
	rfs := &fstest.RecordingFs{FileContent: "default   http://tsuru.google.com"}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	context := &Context{[]string{"default", "http://tsuru.google.com"}, manager.stdout, manager.stderr, manager.stdin}
	targetAdd := &targetAdd{}
	err := targetAdd.Run(context, nil)
	c.Assert(err, check.IsNil)
	c.Assert(context.Stdout.(*bytes.Buffer).String(), check.Equals, "New target default -> http://tsuru.google.com added to target list\n")
}

func (s *S) TestTargetAddRunOnlyOneArg(c *check.C) {
	rfs := &fstest.RecordingFs{FileContent: "default   http://tsuru.google.com"}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	context := &Context{[]string{"default http://tsuru.google.com"}, manager.stdout, manager.stderr, manager.stdin}
	targetAdd := &targetAdd{}
	err := targetAdd.Run(context, nil)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Invalid arguments")
}

func (s *S) TestTargetAddWithSet(c *check.C) {
	rfs := &fstest.RecordingFs{FileContent: "old\thttp://tsuru.io"}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	context := &Context{[]string{"default", "http://tsuru.google.com"}, manager.stdout, manager.stderr, manager.stdin}
	targetAdd := &targetAdd{}
	targetAdd.Flags().Parse(true, []string{"-s"})
	err := targetAdd.Run(context, nil)
	c.Assert(err, check.IsNil)
	c.Assert(context.Stdout.(*bytes.Buffer).String(), check.Equals, "New target default -> http://tsuru.google.com added to target list and defined as the current target\n")
	t, err := ReadTarget()
	c.Assert(err, check.IsNil)
	c.Assert(t, check.Equals, "http://tsuru.google.com")
}

func (s *S) TestTargetAddFlags(c *check.C) {
	command := targetAdd{}
	flagset := command.Flags()
	c.Assert(flagset, check.NotNil)
	flagset.Parse(true, []string{"--set-current"})
	set := flagset.Lookup("set-current")
	c.Assert(set, check.NotNil)
	c.Check(set.Name, check.Equals, "set-current")
	c.Check(set.Usage, check.Equals, "Add and define the target as the current target")
	c.Check(set.Value.String(), check.Equals, "true")
	c.Check(set.DefValue, check.Equals, "false")
	sset := flagset.Lookup("s")
	c.Assert(sset, check.NotNil)
	c.Check(sset.Name, check.Equals, "s")
	c.Check(sset.Usage, check.Equals, "Add and define the target as the current target")
	c.Check(sset.Value.String(), check.Equals, "true")
	c.Check(sset.DefValue, check.Equals, "false")
	c.Check(command.set, check.Equals, true)
}

func (s *S) TestIfTargetLabelExists(c *check.C) {
	rfs := &fstest.RecordingFs{FileContent: "first\thttp://tsuru.io/\ndefault\thttp://tsuru.google.com"}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	mustBeTrueIfExist, err := checkIfTargetLabelExists("default")
	c.Assert(err, check.IsNil)
	c.Assert(mustBeTrueIfExist, check.Equals, true)
}

func (s *S) TestIfTargetLabelDoesNotExist(c *check.C) {
	rfs := &fstest.RecordingFs{FileContent: "first\thttp://tsuru.io/\ndefault\thttp://tsuru.google.com"}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	mustBeFalse, err := checkIfTargetLabelExists("doesnotexist")
	c.Assert(err, check.IsNil)
	c.Assert(mustBeFalse, check.Equals, false)
}

func (s *S) TestGetTargets(c *check.C) {
	rfs := &fstest.RecordingFs{FileContent: "first\thttp://tsuru.io/\ndefault\thttp://tsuru.google.com"}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	var expected = map[string]string{
		"first":   "http://tsuru.io/",
		"default": "http://tsuru.google.com",
	}
	got, err := getTargets()
	c.Assert(err, check.IsNil)
	c.Assert(len(got), check.Equals, len(expected))
	for k, v := range got {
		c.Assert(expected[k], check.Equals, v)
	}
}

func (s *S) TestTargetInfo(c *check.C) {
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
	c.Assert(target.Info(), check.DeepEquals, expected)
}

func (s *S) TestTargetRun(c *check.C) {
	content := `first	http://tsuru.io
default	http://tsuru.google.com
other	http://other.tsuru.io`
	rfs := &fstest.RecordingFs{}
	f, _ := rfs.Create(JoinWithUserDir(".tsuru_target"))
	f.Write([]byte("http://tsuru.io"))
	f.Close()
	f, _ = rfs.Create(JoinWithUserDir(".tsuru_targets"))
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
	c.Assert(err, check.IsNil)
	got := context.Stdout.(*bytes.Buffer).String()
	c.Assert(got, check.Equals, expected)
}

func (s *S) TestResetTargetList(c *check.C) {
	rfs := &fstest.RecordingFs{FileContent: "first\thttp://tsuru.io/\ndefault\thttp://tsuru.google.com"}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	var expected = map[string]string{
		"first":   "http://tsuru.io/",
		"default": "http://tsuru.google.com",
	}
	got, err := getTargets()
	c.Assert(err, check.IsNil)
	c.Assert(len(got), check.Equals, len(expected))
	err = resetTargetList()
	c.Assert(err, check.IsNil)
	got, err = getTargets()
	c.Assert(err, check.IsNil)
	c.Assert(got, check.DeepEquals, map[string]string{})
}

func (s *S) TestTargetRemoveInfo(c *check.C) {
	desc := `Remove a target from target-list (tsuru server)
`
	expected := &Info{
		Name:    "target-remove",
		Usage:   "target-remove",
		Desc:    desc,
		MinArgs: 1,
	}
	targetRemove := &targetRemove{}
	c.Assert(targetRemove.Info(), check.DeepEquals, expected)
}

func (s *S) TestTargetRemove(c *check.C) {
	rfs := &fstest.RecordingFs{FileContent: "first\thttp://tsuru.io/\ndefault\thttp://tsuru.google.com"}
	f, _ := rfs.Create(JoinWithUserDir(".tsuru_target"))
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
	c.Assert(err, check.IsNil)
	c.Assert(len(got), check.Equals, len(expectedBefore))
	targetRemove := &targetRemove{}
	context := &Context{[]string{"first"}, manager.stdout, manager.stderr, manager.stdin}
	err = targetRemove.Run(context, nil)
	c.Assert(err, check.IsNil)
	got, err = getTargets()
	c.Assert(err, check.IsNil)
	c.Assert(len(got), check.Equals, len(expectedAfter))
	_, hasKey := got["default"]
	c.Assert(hasKey, check.Equals, true)
	_, hasKey = got["first"]
	c.Assert(hasKey, check.Equals, false)
}

func (s *S) TestTargetRemoveCurrentTarget(c *check.C) {
	rfs := &fstest.RecordingFs{}
	f, _ := rfs.Create(JoinWithUserDir(".tsuru_targets"))
	f.Write([]byte("first\thttp://tsuru.io/\ndefault\thttp://tsuru.google.com"))
	f.Close()
	f, _ = rfs.Create(JoinWithUserDir(".tsuru_target"))
	f.Write([]byte("http://tsuru.google.com"))
	f.Close()
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	targetRemove := &targetRemove{}
	context := &Context{[]string{"default"}, manager.stdout, manager.stderr, manager.stdin}
	err := targetRemove.Run(context, nil)
	c.Assert(err, check.IsNil)
	_, err = ReadTarget()
	c.Assert(err, check.NotNil)
}

func (s *S) TestTargetSetInfo(c *check.C) {
	desc := `Change current target (tsuru server)
`
	expected := &Info{
		Name:    "target-set",
		Usage:   "target-set <label>",
		Desc:    desc,
		MinArgs: 1,
	}
	targetSet := &targetSet{}
	c.Assert(targetSet.Info(), check.DeepEquals, expected)
}

func (s *S) TestTargetSetRun(c *check.C) {
	rfs := &fstest.RecordingFs{FileContent: "first\thttp://tsuru.io/\ndefault\thttp://tsuru.google.com"}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	targetSet := &targetSet{}
	context := &Context{[]string{"default"}, manager.stdout, manager.stderr, manager.stdin}
	err := targetSet.Run(context, nil)
	c.Assert(err, check.IsNil)
	got := context.Stdout.(*bytes.Buffer).String()
	c.Assert(strings.Contains(got, "New target is default -> http://tsuru.google.com\n"), check.Equals, true)
}

func (s *S) TestTargetSetRunUnknowTarget(c *check.C) {
	rfs := &fstest.RecordingFs{FileContent: "first\thttp://tsuru.io/\ndefault\thttp://tsuru.google.com"}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	targetSet := &targetSet{}
	context := &Context{[]string{"doesnotexist"}, manager.stdout, manager.stderr, manager.stdin}
	err := targetSet.Run(context, nil)
	c.Assert(err, check.ErrorMatches, "Target not found")
}

func (s *S) TestNewTargetSlice(c *check.C) {
	t := newTargetSlice()
	c.Assert(t.sorted, check.Equals, false)
	c.Assert(t.current, check.Equals, -1)
	c.Assert(t.targets, check.IsNil)
}

func (s *S) TestTargetSliceAdd(c *check.C) {
	var t targetSlice
	t.sorted = true
	t.add("default", "http://tsuru.io")
	c.Assert(t.targets, check.DeepEquals, []tsuruTarget{{label: "default", url: "http://tsuru.io"}})
	c.Assert(t.sorted, check.Equals, true)
	t.add("abc", "http://tsuru.io")
	c.Assert(t.sorted, check.Equals, false)
}

func (s *S) TestTargetSliceLen(c *check.C) {
	t := targetSlice{
		targets: []tsuruTarget{{label: "default", url: ""}},
	}
	c.Assert(t.Len(), check.Equals, len(t.targets))
}

func (s *S) TestTargetSliceLess(c *check.C) {
	t := targetSlice{
		targets: []tsuruTarget{
			{label: "first", url: ""},
			{label: "default", url: ""},
			{label: "second", url: ""},
		},
	}
	c.Check(t.Less(0, 1), check.Equals, false)
	c.Check(t.Less(0, 2), check.Equals, true)
	c.Check(t.Less(1, 0), check.Equals, true)
	c.Check(t.Less(1, 2), check.Equals, true)
	c.Check(t.Less(2, 0), check.Equals, false)
}

func (s *S) TestTargetSliceSwap(c *check.C) {
	t := targetSlice{
		targets: []tsuruTarget{
			{label: "first", url: ""},
			{label: "default", url: ""},
			{label: "second", url: ""},
		},
	}
	c.Assert(t.Less(0, 1), check.Equals, false)
	t.Swap(0, 1)
	c.Assert(t.Less(0, 1), check.Equals, true)
}

func (s *S) TestTargetSliceSort(c *check.C) {
	t := targetSlice{
		targets: []tsuruTarget{
			{label: "first", url: ""},
			{label: "default", url: ""},
			{label: "second", url: ""},
		},
	}
	t.Sort()
	c.Assert(t.Less(0, 1), check.Equals, true)
	c.Assert(t.Less(1, 2), check.Equals, true)
	c.Assert(t.sorted, check.Equals, true)
}

func (s *S) TestTargetSliceSetCurrent(c *check.C) {
	t := targetSlice{
		targets: []tsuruTarget{
			{label: "first", url: "first.tsuru.io"},
			{label: "default", url: "default.tsuru.io"},
			{label: "second", url: "second.tsuru.io"},
		},
		current: -1,
	}
	t.setCurrent("unknown.tsuru.io")
	c.Check(t.current, check.Equals, -1)
	t.setCurrent("first.tsuru.io")
	c.Check(t.current, check.Equals, 1) // sort the slice
	t.setCurrent("second.tsuru.io")
	c.Check(t.current, check.Equals, 2)
	t.setCurrent("unknown.tsuru.io")
	c.Check(t.current, check.Equals, 2)
}

func (s *S) TestTargetSliceStringSortedNoCurrent(c *check.C) {
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
	c.Assert(t.String(), check.Equals, expected)
}

func (s *S) TestTargetSliceStringUnsortedNoCurrent(c *check.C) {
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
	c.Assert(t.String(), check.Equals, expected)
}

func (s *S) TestTargetSliceStringSortedWithCurrent(c *check.C) {
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
	c.Assert(t.String(), check.Equals, expected)
}

func (s *S) TestTargetSliceStringUnsortedWithCurrent(c *check.C) {
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
	c.Assert(t.String(), check.Equals, expected)
}
