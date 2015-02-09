// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"io"
	"os"
	"os/exec"
	"path"
	"syscall"

	"github.com/tsuru/tsuru/cmd/cmdtest"
	"gopkg.in/check.v1"
	"launchpad.net/gnuflag"
)

var appflag = &gnuflag.Flag{
	Name:     "app",
	Usage:    "The name of the app.",
	Value:    nil,
	DefValue: "",
}

var appshortflag = &gnuflag.Flag{
	Name:     "a",
	Usage:    "The name of the app.",
	Value:    nil,
	DefValue: "",
}

func writeConfig(sourceFile string, c *check.C) string {
	srcConfig, err := os.Open(sourceFile)
	c.Assert(err, check.IsNil)
	defer srcConfig.Close()
	p := path.Join(os.TempDir(), "guesser-tests")
	err = os.MkdirAll(p, 0700)
	c.Assert(err, check.IsNil)
	cmd := exec.Command("git", "init")
	cmd.Dir = p
	c.Assert(err, check.IsNil)
	err = cmd.Run()
	c.Assert(err, check.IsNil)
	dstConfig, err := os.OpenFile(path.Join(p, ".git", "config"), syscall.O_WRONLY|syscall.O_TRUNC|syscall.O_CREAT|syscall.O_CLOEXEC, 0644)
	c.Assert(err, check.IsNil)
	defer dstConfig.Close()
	_, err = io.Copy(dstConfig, srcConfig)
	c.Assert(err, check.IsNil)
	return p
}

func (s *S) TestGitGuesser(c *check.C) {
	p := writeConfig("testdata/gitconfig-ok", c)
	defer os.RemoveAll(p)
	dirPath := path.Join(p, "somepath")
	err := os.MkdirAll(dirPath, 0700) // Will be removed when p is removed.
	c.Assert(err, check.IsNil)
	g := GitGuesser{}
	name, err := g.GuessName(p)
	c.Assert(err, check.IsNil)
	c.Assert(name, check.Equals, "gopher")
	name, err = g.GuessName(dirPath)
	c.Assert(err, check.IsNil)
	c.Assert(name, check.Equals, "gopher")
}

func (s *S) TestGitGuesserNonGitUser(c *check.C) {
	p := writeConfig("testdata/gitconfig-ok-nongit-user", c)
	defer os.RemoveAll(p)
	dirPath := path.Join(p, "somepath")
	err := os.MkdirAll(dirPath, 0700)
	c.Assert(err, check.IsNil)
	g := GitGuesser{}
	name, err := g.GuessName(p)
	c.Assert(err, check.IsNil)
	c.Assert(name, check.Equals, "gopher")
	name, err = g.GuessName(dirPath)
	c.Assert(err, check.IsNil)
	c.Assert(name, check.Equals, "gopher")
}

// This test may fail if you have a git repository in /tmp. By the way, if you
// do have a repository in the temporary file hierarchy, please kill yourself.
func (s *S) TestGitGuesserWhenTheDirectoryIsNotAGitRepository(c *check.C) {
	p := path.Join(os.TempDir(), "guesser-tests")
	err := os.MkdirAll(p, 0700)
	c.Assert(err, check.IsNil)
	defer os.RemoveAll(p)
	name, err := GitGuesser{}.GuessName(p)
	c.Assert(name, check.Equals, "")
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, "^Git repository not found:.*")
}

func (s *S) TestGitGuesserWithoutTsuruRemote(c *check.C) {
	p := writeConfig("testdata/gitconfig-without-tsuru-remote", c)
	defer os.RemoveAll(p)
	name, err := GitGuesser{}.GuessName(p)
	c.Assert(name, check.Equals, "")
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "tsuru remote not declared.")
}

func (s *S) TestGitGuesserWithTsuruRemoteNotMatchingTsuruPattern(c *check.C) {
	p := writeConfig("testdata/gitconfig-not-matching", c)
	defer os.RemoveAll(p)
	name, err := GitGuesser{}.GuessName(p)
	c.Assert(name, check.Equals, "")
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, `"tsuru" remote did not match the pattern. Want something like <user>@<host>:<app-name>.git, got git://myhost.com/gopher.git`)
}

func (s *S) TestDirnameGuesser(c *check.C) {
	name, err := DirnameGuesser{}.GuessName("/something/wat")
	c.Assert(err, check.IsNil)
	c.Assert(name, check.Equals, "wat")
}

func (s *S) TestGuessingCommandGuesserNil(c *check.C) {
	g := GuessingCommand{G: nil}
	c.Assert(g.guesser(), check.FitsTypeOf, MultiGuesser{})
}

func (s *S) TestGuessingCommandGuesserNonNil(c *check.C) {
	fake := &cmdtest.FakeGuesser{}
	g := GuessingCommand{G: fake}
	c.Assert(g.guesser(), check.DeepEquals, fake)
}

func (s *S) TestGuessingCommandWithFlagDefined(c *check.C) {
	fake := &cmdtest.FakeGuesser{Name: "other-app"}
	g := GuessingCommand{G: fake}
	g.Flags().Parse(true, []string{"--app", "myapp"})
	name, err := g.Guess()
	c.Assert(err, check.IsNil)
	c.Assert(name, check.Equals, "myapp")
	pwd, err := os.Getwd()
	c.Assert(err, check.IsNil)
	c.Assert(fake.HasGuess(pwd), check.Equals, false)
}

func (s *S) TestGuessingCommandWithShortFlagDefined(c *check.C) {
	fake := &cmdtest.FakeGuesser{Name: "other-app"}
	g := GuessingCommand{G: fake}
	g.Flags().Parse(true, []string{"-a", "myapp"})
	name, err := g.Guess()
	c.Assert(err, check.IsNil)
	c.Assert(name, check.Equals, "myapp")
	pwd, err := os.Getwd()
	c.Assert(err, check.IsNil)
	c.Assert(fake.HasGuess(pwd), check.Equals, false)
}

func (s *S) TestGuessingCommandWithoutFlagDefined(c *check.C) {
	fake := &cmdtest.FakeGuesser{Name: "other-app"}
	g := GuessingCommand{G: fake}
	name, err := g.Guess()
	c.Assert(err, check.IsNil)
	c.Assert(name, check.Equals, "other-app")
	pwd, err := os.Getwd()
	c.Assert(err, check.IsNil)
	c.Assert(fake.HasGuess(pwd), check.Equals, true)
}

func (s *S) TestGuessingCommandFailToGuess(c *check.C) {
	fake := &cmdtest.FailingFakeGuesser{ErrorMessage: "Something's always wrong"}
	g := GuessingCommand{G: fake}
	name, err := g.Guess()
	c.Assert(name, check.Equals, "")
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, `tsuru wasn't able to guess the name of the app.

Use the --app flag to specify it.

Something's always wrong`)
	pwd, err := os.Getwd()
	c.Assert(err, check.IsNil)
	c.Assert(fake.HasGuess(pwd), check.Equals, true)
}

func (s *S) TestGuessingCommandFlags(c *check.C) {
	var flags []gnuflag.Flag
	expected := []gnuflag.Flag{*appshortflag, *appflag}
	command := GuessingCommand{}
	flagset := command.Flags()
	flagset.VisitAll(func(f *gnuflag.Flag) {
		f.Value = nil
		flags = append(flags, *f)
	})
	c.Assert(flags, check.DeepEquals, expected)
}
