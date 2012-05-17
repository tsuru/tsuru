package repository

import (
	. "launchpad.net/gocheck"
	"os"
	"os/exec"
	"path"
)

func (s *S) TestCommitShouldCommit(c *C) {
	f, err := os.Create(path.Join(s.repoPath, "commit.txt"))
	c.Assert(err, IsNil)
	f.Close()
	err = s.git.commit("added file.txt")
	c.Assert(err, IsNil)
	out, err := s.git.run("log", "-1", "--format=%s")
	c.Assert(err, IsNil)
	c.Assert(out, Equals, "added file.txt\n")
}

func (s *S) TestPushShouldPush(c *C) {
	f, err := os.Create(path.Join(s.repoPath, "push.txt"))
	c.Assert(err, IsNil)
	f.Close()
	bare := s.repoPath + ".git"
	err = exec.Command("git", "init", "--bare", bare).Run()
	c.Assert(err, IsNil)
	_, err = s.git.run("remote", "add", "origin", bare)
	c.Assert(err, IsNil)
	err = s.git.commit("added push.txt")
	c.Assert(err, IsNil)
	err = s.git.push("origin", "master")
	c.Assert(err, IsNil)
	bareRepo := &repository{bare: true, path: bare}
	out, err := bareRepo.run("log", "-1", "--format=%s")
	c.Assert(err, IsNil)
	c.Assert(out, Equals, "added push.txt\n")
}

func (s *S) TestGetPathShouldReturnRelativePathInTheRepository(c *C) {
	expected := path.Join(s.git.path, "abc")
	c.Assert(s.git.getPath("abc"), Equals, expected)
}

func (s *S) TestGetPathShouldAcceptVariadicArguments(c *C) {
	expected := path.Join(s.git.path, "abc", "def")
	c.Assert(s.git.getPath("abc", "def"), Equals, expected)
}
