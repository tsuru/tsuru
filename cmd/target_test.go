// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"io"
	"os"
	"path"

	"github.com/tsuru/tsuru/fs/fstest"
	check "gopkg.in/check.v1"
)

func readRecordedTarget(fs *fstest.RecordingFs) string {
	filePath := path.Join(os.ExpandEnv("${HOME}"), ".tsuru", "target")
	fil, _ := fsystem.Open(filePath)
	b, _ := io.ReadAll(fil)
	return string(b)
}

func (s *S) TestWriteTarget(c *check.C) {
	rfs := &fstest.RecordingFs{}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	os.Unsetenv("TSURU_TARGET")
	err := WriteTarget("http://tsuru.globo.com")
	c.Assert(err, check.IsNil)
	filePath := path.Join(os.ExpandEnv("${HOME}"), ".tsuru", "target")
	c.Assert(rfs.HasAction("openfile "+filePath+" with mode 0600"), check.Equals, true)
	c.Assert(readRecordedTarget(rfs), check.Equals, "http://tsuru.globo.com")
}

func (s *S) TestWriteTargetKeepsLeadingSlashs(c *check.C) {
	rfs := &fstest.RecordingFs{}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	os.Unsetenv("TSURU_TARGET")
	err := WriteTarget("http://tsuru.globo.com//")
	c.Assert(err, check.IsNil)
	c.Assert(readRecordedTarget(rfs), check.Equals, "http://tsuru.globo.com//")
}

func (s *S) TestReadTarget(c *check.C) {
	os.Unsetenv("TSURU_TARGET")
	rfs := &fstest.RecordingFs{FileContent: "http://tsuru.google.com"}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	target, err := ReadTarget()
	c.Assert(err, check.IsNil)
	c.Assert(target, check.Equals, "http://tsuru.google.com")
}

func (s *S) TestReadTargetLegacy(c *check.C) {
	os.Unsetenv("TSURU_TARGET")
	var rfs fstest.RecordingFs
	fsystem = &rfs
	defer func() { fsystem = nil }()
	f, err := fsystem.Create(JoinWithUserDir(".tsuru_target"))
	c.Assert(err, check.IsNil)
	f.WriteString("http://tsuru.google.com")
	f.Close()
	target, err := ReadTarget()
	c.Assert(err, check.IsNil)
	c.Assert(target, check.Equals, "http://tsuru.google.com")
	target, err = readTarget(JoinWithUserDir(".tsuru", "target"))
	c.Assert(err, check.IsNil)
	c.Assert(target, check.Equals, "http://tsuru.google.com")
	dir := JoinWithUserDir(".tsuru")
	c.Assert(rfs.HasAction("mkdirall "+dir+" with mode 0700"), check.Equals, true)
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
	os.Unsetenv("TSURU_TARGET")
	fsystem = &fstest.FileNotFoundFs{}
	defer func() {
		fsystem = nil
	}()
	target, err := ReadTarget()
	c.Assert(target, check.Equals, "")
	c.Assert(err, check.Equals, errUndefinedTarget)
}

func (s *S) TestReadTargetTrimsFileContent(c *check.C) {
	os.Unsetenv("TSURU_TARGET")
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
	targetFile := JoinWithUserDir(".tsuru", "target")
	c.Assert(rfs.HasAction("remove "+targetFile), check.Equals, true)
}

func (s *S) TestGetTarget(c *check.C) {
	os.Unsetenv("TSURU_TARGET")
	var tests = []struct {
		expected string
		target   string
	}{
		{"http://localhost", "http://localhost"},
		{"http://localhost/tsuru/", "http://localhost/tsuru/"},
		{"https://localhost", "https://localhost"},
		{"http://remotehost", "remotehost"},
	}
	for _, t := range tests {
		fsystem = &fstest.RecordingFs{FileContent: t.target}
		got, err := GetTarget()
		c.Check(err, check.IsNil)
		c.Check(got, check.Equals, t.expected)
		fsystem = nil
	}
}

func (s *S) TestGetTargetLabel(c *check.C) {
	os.Unsetenv("TSURU_TARGET")
	rfs := &fstest.RecordingFs{FileContent: "first\thttp://tsuru.io/\nsecond\thttp://tsuru.google.com"}
	fsystem = rfs
	defer func() {
		fsystem = nil
		os.Unsetenv("TSURU_TARGET")
	}()
	var tests = []struct {
		expected string
		target   string
	}{
		{"first", "http://tsuru.io/"},
		{"second", "http://tsuru.google.com"},
	}
	for _, t := range tests {
		os.Setenv("TSURU_TARGET", t.target)
		got, err := GetTargetLabel()
		c.Check(err, check.IsNil)
		c.Check(got, check.Equals, t.expected)
	}
}

func (s *S) TestGetTargetLabelStableWithRepeatedValues(c *check.C) {
	os.Unsetenv("TSURU_TARGET")
	rfs := &fstest.RecordingFs{FileContent: "2second\thttp://tsuru.io/\n1first\thttp://tsuru.io/"}
	fsystem = rfs
	defer func() {
		fsystem = nil
		os.Unsetenv("TSURU_TARGET")
	}()
	var tests = []struct {
		expected string
		target   string
	}{
		{"1first", "http://tsuru.io/"},
		{"1first", "http://tsuru.io/"},
		{"1first", "http://tsuru.io/"},
	}
	for _, t := range tests {
		os.Setenv("TSURU_TARGET", t.target)
		got, err := GetTargetLabel()
		c.Check(err, check.IsNil)
		c.Check(got, check.Equals, t.expected)
	}
}

func (s *S) TestGetTargetLabelNotFound(c *check.C) {
	os.Setenv("TSURU_TARGET", "http://notfound.io")
	rfs := &fstest.RecordingFs{FileContent: "first\thttp://tsuru.io/\nsecond\thttp://tsuru.google.com"}
	fsystem = rfs
	defer func() {
		fsystem = nil
		os.Unsetenv("TSURU_TARGET")
	}()
	got, err := GetTargetLabel()
	c.Check(err, check.NotNil)
	c.Check(got, check.Equals, "")
}

func (s *S) TestGetURLVersion(c *check.C) {
	os.Unsetenv("TSURU_TARGET")
	var tests = []struct {
		version  string
		path     string
		expected string
		target   string
	}{
		{"1.2", "/apps", "http://localhost/1.2/apps", "http://localhost"},
		{"1.2", "/apps", "http://localhost/tsuru/1.2/apps", "http://localhost/tsuru/"},
		{"1.2", "/apps", "https://localhost/1.2/apps", "https://localhost"},
		{"1.2", "/apps", "http://remotehost/1.2/apps", "remotehost"},
	}
	for _, t := range tests {
		fsystem = &fstest.RecordingFs{FileContent: t.target}
		got, err := GetURLVersion(t.version, t.path)
		c.Check(err, check.IsNil)
		c.Check(got, check.Equals, t.expected)
		fsystem = nil
	}
}

func (s *S) TestGetURLVersionUndefinedTarget(c *check.C) {
	os.Unsetenv("TSURU_TARGET")
	rfs := &fstest.FileNotFoundFs{}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	got, err := GetURLVersion("1.3", "/apps")
	c.Assert(got, check.Equals, "")
	c.Assert(err, check.Equals, errUndefinedTarget)
}

func (s *S) TestGetURL(c *check.C) {
	os.Unsetenv("TSURU_TARGET")
	var tests = []struct {
		path     string
		expected string
		target   string
	}{
		{"/apps", "http://localhost/1.0/apps", "http://localhost"},
		{"/apps", "http://localhost/tsuru/1.0/apps", "http://localhost/tsuru/"},
		{"/apps", "https://localhost/1.0/apps", "https://localhost"},
		{"/apps", "http://remotehost/1.0/apps", "remotehost"},
	}
	for _, t := range tests {
		fsystem = &fstest.RecordingFs{FileContent: t.target}
		got, err := GetURL(t.path)
		c.Check(err, check.IsNil)
		c.Check(got, check.Equals, t.expected)
		fsystem = nil
	}
}

func (s *S) TestGetURLUndefinedTarget(c *check.C) {
	os.Unsetenv("TSURU_TARGET")
	rfs := &fstest.FileNotFoundFs{}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	got, err := GetURL("/apps")
	c.Assert(got, check.Equals, "")
	c.Assert(err, check.Equals, errUndefinedTarget)
}

func (s *S) TestIfTargetLabelExists(c *check.C) {
	rfs := &fstest.RecordingFs{FileContent: "first\thttp://tsuru.io/\ndefault\thttp://tsuru.google.com"}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	mustBeTrueIfExist, err := CheckIfTargetLabelExists("default")
	c.Assert(err, check.IsNil)
	c.Assert(mustBeTrueIfExist, check.Equals, true)
}

func (s *S) TestIfTargetLabelDoesNotExist(c *check.C) {
	rfs := &fstest.RecordingFs{FileContent: "first\thttp://tsuru.io/\ndefault\thttp://tsuru.google.com"}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	mustBeFalse, err := CheckIfTargetLabelExists("doesnotexist")
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
	c.Assert(got, check.DeepEquals, expected)
	dir := JoinWithUserDir(".tsuru")
	c.Assert(rfs.HasAction("mkdirall "+dir+" with mode 0700"), check.Equals, true)
}

func (s *S) TestGetTargetsLegacy(c *check.C) {
	var rfs fstest.RecordingFs
	fsystem = &rfs
	defer func() { fsystem = nil }()
	content := "first\thttp://tsuru.io/\ndefault\thttp://tsuru.google.com\n"
	f, err := fsystem.Create(JoinWithUserDir(".tsuru_targets"))
	c.Assert(err, check.IsNil)
	f.WriteString(content)
	f.Close()
	var expected = map[string]string{
		"first":   "http://tsuru.io/",
		"default": "http://tsuru.google.com",
	}
	got, err := getTargets()
	c.Assert(err, check.IsNil)
	c.Assert(got, check.DeepEquals, expected)
	f, err = fsystem.Open(JoinWithUserDir(".tsuru", "targets"))
	c.Assert(err, check.IsNil)
	defer f.Close()
	b, err := io.ReadAll(f)
	c.Assert(err, check.IsNil)
	c.Assert(string(b), check.Equals, content)
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
	c.Assert(got, check.HasLen, len(expected))
	err = resetTargetList()
	c.Assert(err, check.IsNil)
	got, err = getTargets()
	c.Assert(err, check.IsNil)
	c.Assert(got, check.DeepEquals, map[string]string{})
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
