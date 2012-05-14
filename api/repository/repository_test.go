package repository_test

import (
	"fmt"
	. "github.com/timeredbull/tsuru/api/app"
	. "github.com/timeredbull/tsuru/api/repository"
	. "launchpad.net/gocheck"
	"os"
	"path"
)

func (s *S) TestNewRepository(c *C) {
	a := App{Name: "foobar"}
	err := NewRepository(a.Name)
	c.Assert(err, IsNil)

	repoPath := GetRepositoryPath(a.Name)
	_, err = os.Stat(repoPath)
	c.Assert(err, IsNil)

	_, err = os.Stat(path.Join(repoPath, "config"))
	c.Assert(err, IsNil)

	err = os.RemoveAll(repoPath)
	c.Assert(err, IsNil)
}

func (s *S) TestDeleteGitRepository(c *C) {
	a := &App{Name: "someApp"}
	repoPath := GetRepositoryPath(a.Name)

	err := NewRepository(a.Name)
	c.Assert(err, IsNil)

	_, err = os.Stat(path.Join(repoPath, "config"))
	c.Assert(err, IsNil)

	DeleteRepository(a.Name)
	_, err = os.Stat(repoPath)
	c.Assert(os.IsNotExist(err), Equals, true)
}

func (s *S) TestCloneRepository(c *C) {
	a := App{Name: "barfoo"}
	err := CloneRepository(a.Name)
	c.Assert(err, IsNil)
}

func (s *S) TestGetRepositoryUrl(c *C) {
	a := App{Name: "foobar"}
	url := GetRepositoryUrl(a.Name)
	expected := fmt.Sprintf("git@tsuru.plataformas.glb.com:%s.git", a.Name)
	c.Assert(url, Equals, expected)
}

func (s *S) TestGetRepositoryName(c *C) {
	a := App{Name: "someApp"}
	obtained := GetRepositoryName(a.Name)
	expected := fmt.Sprintf("%s.git", a.Name)
	c.Assert(obtained, Equals, expected)
}

func (s *S) TestGetRepositoryPath(c *C) {
	a := App{Name: "someApp"}
	home := os.Getenv("HOME")
	obtained := GetRepositoryPath(a.Name)
	expected := path.Join(home, "../git", GetRepositoryName(a.Name))
	c.Assert(obtained, Equals, expected)
}
