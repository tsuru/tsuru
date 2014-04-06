// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package git

import (
	"errors"
	"launchpad.net/gocheck"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"reflect"
	"testing"
)

func Test(t *testing.T) {
	gocheck.TestingT(t)
}

type S struct {
	repoPath string
}

var _ = gocheck.Suite(&S{})

func (s *S) SetUpSuite(c *gocheck.C) {
	tmpdir, err := filepath.EvalSymlinks(os.TempDir())
	c.Assert(err, gocheck.IsNil)
	s.repoPath = path.Join(tmpdir, "git")
	err = os.MkdirAll(s.repoPath, 0755)
	c.Assert(err, gocheck.IsNil)
	cmd := exec.Command("git", "init")
	cmd.Dir = s.repoPath
	err = cmd.Run()
	c.Assert(err, gocheck.IsNil)
	err = exec.Command("cp", "testdata/gitconfig", path.Join(s.repoPath, ".git", "config")).Run()
	c.Assert(err, gocheck.IsNil)
	subdir := path.Join(s.repoPath, "a", "b", "c")
	err = os.MkdirAll(subdir, 0755)
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TearDownSuite(c *gocheck.C) {
	os.RemoveAll(s.repoPath)
}

func (s *S) TestDiscoverRepositoryPath(c *gocheck.C) {
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

func (s *S) TestOpenRepository(c *gocheck.C) {
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

func (s *S) TestGetRemoteURL(c *gocheck.C) {
	var data = []struct {
		name     string
		expected string
		err      error
	}{
		{"origin", "git@github.com:tsuru/tsuru-django-sample.git", nil},
		{"tsuru", "git@tsuruhost.com:gopher.git", nil},
		{"wut", "", errors.New(`Remote "wut" not found.`)},
	}
	repo, err := OpenRepository(s.repoPath)
	c.Assert(err, gocheck.IsNil)
	for _, d := range data {
		got, err := repo.RemoteURL(d.name)
		if got != d.expected {
			c.Errorf("RemoteURL(%q): Want %q. Got %q.", d.name, d.expected, got)
		}
		if !reflect.DeepEqual(d.err, err) {
			c.Errorf("RemoteURL(%q): Want error %q. Got %q.", d.name, d.err, err)
		}
	}
}

func BenchmarkGetRemoteURL(b *testing.B) {
	tmpdir, err := filepath.EvalSymlinks(os.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	repoPath := path.Join(tmpdir, "git-bench")
	err = os.MkdirAll(repoPath, 0755)
	if err != nil {
		b.Fatal(err)
	}
	cmd := exec.Command("git", "init")
	cmd.Dir = repoPath
	err = cmd.Run()
	if err != nil {
		b.Fatal(err)
	}
	err = exec.Command("cp", "testdata/gitconfig", path.Join(repoPath, ".git", "config")).Run()
	if err != nil {
		b.Fatal(err)
	}
	for i := 0; i < b.N; i++ {
		repo, _ := OpenRepository(repoPath)
		repo.RemoteURL("origin")
		repo.RemoteURL("tsuru")
	}
}
