// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"errors"
	"github.com/fsouza/gogit/git"
	"io"
	. "launchpad.net/gocheck"
	"os"
	"path"
	"syscall"
)

func writeConfig(sourceFile string, c *C) string {
	srcConfig, err := os.Open(sourceFile)
	c.Assert(err, IsNil)
	defer srcConfig.Close()
	p := path.Join(os.TempDir(), "guesser-tests")
	err = os.MkdirAll(p, 0700)
	c.Assert(err, IsNil)
	repo, err := git.InitRepository(p, false)
	c.Assert(err, IsNil)
	defer repo.Free()
	dstConfig, err := os.OpenFile(path.Join(p, ".git", "config"), syscall.O_WRONLY|syscall.O_TRUNC|syscall.O_CREAT|syscall.O_CLOEXEC, 0644)
	c.Assert(err, IsNil)
	defer dstConfig.Close()
	_, err = io.Copy(dstConfig, srcConfig)
	c.Assert(err, IsNil)
	return p
}

func (s *S) TestGitGuesser(c *C) {
	p := writeConfig("testdata/gitconfig-ok", c)
	defer os.RemoveAll(p)
	dirPath := path.Join(p, "somepath")
	err := os.MkdirAll(dirPath, 0700) // Will be removed when p is removed.
	c.Assert(err, IsNil)
	g := GitGuesser{}
	name, err := g.GuessName(p) // repository root
	c.Assert(err, IsNil)
	c.Assert(name, Equals, "gopher")
	name, err = g.GuessName(dirPath) // subdirectory
	c.Assert(err, IsNil)
	c.Assert(name, Equals, "gopher")
}

// This test may fail if you have a git repository in /tmp. By the way, if you
// do have a repository in the temporary file hierarchy, please kill yourself.
func (s *S) TestGitGuesserWhenTheDirectoryIsNotAGitRepository(c *C) {
	p := path.Join(os.TempDir(), "guesser-tests")
	err := os.MkdirAll(p, 0700)
	c.Assert(err, IsNil)
	defer os.RemoveAll(p)
	name, err := GitGuesser{}.GuessName(p)
	c.Assert(name, Equals, "")
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^Git repository not found:.*")
}

func (s *S) TestGitGuesserWithoutTsuruRemote(c *C) {
	p := writeConfig("testdata/gitconfig-without-tsuru-remote", c)
	defer os.RemoveAll(p)
	name, err := GitGuesser{}.GuessName(p)
	c.Assert(name, Equals, "")
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "tsuru remote not declared.")
}

func (s *S) TestGitGuesserWithTsuruRemoteNotMatchingTsuruPattern(c *C) {
	p := writeConfig("testdata/gitconfig-not-matching", c)
	defer os.RemoveAll(p)
	name, err := GitGuesser{}.GuessName(p)
	c.Assert(name, Equals, "")
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, `"tsuru" remote did not match the pattern. Want something like git@<host>:<app-name>.git, got me@myhost.com:gopher.git`)
}

func (s *S) TestGuessingCommandGuesserNil(c *C) {
	g := GuessingCommand{g: nil}
	c.Assert(g.guesser(), FitsTypeOf, GitGuesser{})
}

func (s *S) TestGuessingCommandGuesserNonNil(c *C) {
	fake := &FakeGuesser{}
	g := GuessingCommand{g: fake}
	c.Assert(g.guesser(), DeepEquals, fake)
}

type FakeGuesser struct {
	name string
}

func (f *FakeGuesser) GuessName(path string) (string, error) {
	return f.name, nil
}

type FailingFakeGuesser struct {
	message string
}

func (f *FailingFakeGuesser) GuessName(path string) (string, error) {
	return "", errors.New(f.message)
}
