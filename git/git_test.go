// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package git

import (
	"errors"
	. "launchpad.net/gocheck"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"reflect"
	"testing"
)

func Test(t *testing.T) {
	TestingT(t)
}

type S struct {
	repoPath string
}

var _ = Suite(&S{})

func (s *S) SetUpSuite(c *C) {
	tmpdir, err := filepath.EvalSymlinks(os.TempDir())
	c.Assert(err, IsNil)
	s.repoPath = path.Join(tmpdir, "git")
	err = os.MkdirAll(s.repoPath, 0755)
	c.Assert(err, IsNil)
	cmd := exec.Command("git", "init")
	cmd.Dir = s.repoPath
	err = cmd.Run()
	c.Assert(err, IsNil)
	err = exec.Command("cp", "testdata/gitconfig", path.Join(s.repoPath, ".git")).Run()
	c.Assert(err, IsNil)
	subdir := path.Join(s.repoPath, "a", "b", "c")
	err = os.MkdirAll(subdir, 0755)
	c.Assert(err, IsNil)
}

func (s *S) TearDownSuite(c *C) {
	os.RemoveAll(s.repoPath)
}

func (s *S) TestDiscoverRepositoryPath(c *C) {
	var data = []struct {
		path     string
		expected string
		err      error
	}{
		{
			path:     s.repoPath,
			expected: path.Join(s.repoPath, ".git"),
			err:      nil,
		},
		{
			path:     path.Join(s.repoPath, ".git"),
			expected: path.Join(s.repoPath, ".git"),
			err:      nil,
		},
		{
			path:     path.Join(s.repoPath, "a"),
			expected: path.Join(s.repoPath, ".git"),
			err:      nil,
		},
		{
			path:     path.Join(s.repoPath, "a", "b"),
			expected: path.Join(s.repoPath, ".git"),
			err:      nil,
		},
		{
			path:     path.Join(s.repoPath, "a", "b", "c"),
			expected: path.Join(s.repoPath, ".git"),
			err:      nil,
		},
		{
			path:     path.Join(s.repoPath, "a", "b", "c", "d"),
			expected: "",
			err:      errors.New("Repository not found."),
		},
		{
			path:     path.Join(os.TempDir(), "aoshae8yahhh8ua", "doctor-jimmy"),
			expected: "",
			err:      errors.New("Repository not found."),
		},
	}
	for _, d := range data {
		got, err := DiscoverRepositoryPath(d.path)
		if got != d.expected {
			c.Errorf("DiscoverRepositoryPath(%q): Got %q. Want %q.", d.path, got, d.expected)
		}
		if err == nil && d.err != nil {
			c.Errorf("DiscoverRepositoryPath(%q): Expected non-nil error (%+v), got <nil>.", d.path, d.err)
		} else if err != nil && d.err != nil && d.err.Error() != err.Error() {
			c.Errorf("DiscoverRepositoryPath(%q): Expected error %v. Got %v.", d.path, d.err, err)
		}
	}
}

func (s *S) TestOpenRepository(c *C) {
	var data = []struct {
		path     string
		expected *Repository
		err      error
	}{
		{
			path:     s.repoPath,
			expected: &Repository{path: path.Join(s.repoPath, ".git")},
			err:      nil,
		},
		{
			path:     path.Join(s.repoPath, ".git"),
			expected: &Repository{path: path.Join(s.repoPath, ".git")},
			err:      nil,
		},
		{
			path:     path.Join(s.repoPath, ".git") + "/",
			expected: &Repository{path: path.Join(s.repoPath, ".git")},
			err:      nil,
		},
		{
			path:     "/",
			expected: nil,
			err:      errors.New("Repository not found."),
		},
	}
	for _, d := range data {
		repo, err := OpenRepository(d.path)
		if !reflect.DeepEqual(repo, d.expected) {
			c.Errorf("OpenRepository(%q): Got repository %+v. Want %+v.", d.path, repo, d.expected)
		}
		if d.err != nil && err == nil {
			c.Errorf("OpenRepository(%q): Expected non-nil error (%+v), got <nil>.", d.path, d.err)
		} else if d.err != nil && err != nil && d.err.Error() != err.Error() {
			c.Errorf("OpenRepository(%q): Expected error %v. Got %v.", d.path, d.err, err)
		}
	}
}
