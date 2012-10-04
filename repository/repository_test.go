package repository

import (
	"errors"
	"fmt"
	"github.com/globocom/config"
	. "launchpad.net/gocheck"
	"strings"
)

type FakeUnit struct {
	name     string
	commands []string
}

func (u *FakeUnit) RanCommand(cmd string) bool {
	for _, c := range u.commands {
		if c == cmd {
			return true
		}
	}
	return false
}

func (u *FakeUnit) GetName() string {
	return u.name
}

func (u *FakeUnit) Command(cmd ...string) ([]byte, error) {
	u.commands = append(u.commands, cmd[0])
	return []byte("success"), nil
}

type FailingCloneUnit struct {
	FakeUnit
}

func (u *FailingCloneUnit) Command(cmd ...string) ([]byte, error) {
	if strings.HasPrefix(cmd[0], "git clone") {
		u.commands = append(u.commands, cmd[0])
		return nil, errors.New("Failed to clone repository, it already exists!")
	}
	return u.FakeUnit.Command(cmd...)
}

func (s *S) TestCloneRepository(c *C) {
	u := FakeUnit{name: "my-unit"}
	out, err := Clone(&u)
	c.Assert(err, IsNil)
	c.Assert(string(out), Equals, "success")
	expectedCommand := fmt.Sprintf("git clone %s /home/application/current --depth 1", GetReadOnlyUrl(u.GetName()))
	c.Assert(u.RanCommand(expectedCommand), Equals, true)
}

func (s *S) TestPullRepository(c *C) {
	u := FakeUnit{name: "your-unit"}
	out, err := Pull(&u)
	c.Assert(err, IsNil)
	c.Assert(string(out), Equals, "success")
	expectedCommand := fmt.Sprintf("cd /home/application/current && git pull origin master")
	c.Assert(u.RanCommand(expectedCommand), Equals, true)
}

func (s *S) TestCloneOrPullRepositoryRunsClone(c *C) {
	u := FakeUnit{name: "my-unit"}
	out, err := CloneOrPull(&u)
	c.Assert(err, IsNil)
	c.Assert(string(out), Equals, "success")
	clone := fmt.Sprintf("git clone %s /home/application/current --depth 1", GetReadOnlyUrl(u.GetName()))
	pull := fmt.Sprintf("cd /home/application/current && git pull origin master")
	c.Assert(u.RanCommand(clone), Equals, true)
	c.Assert(u.RanCommand(pull), Equals, false)
}

func (s *S) TestCloneOrPullRepositoryRunsPullIfCloneFail(c *C) {
	u := FailingCloneUnit{FakeUnit{name: "my-unit"}}
	out, err := CloneOrPull(&u)
	c.Assert(err, IsNil)
	c.Assert(string(out), Equals, "success")
	clone := fmt.Sprintf("git clone %s /home/application/current --depth 1", GetReadOnlyUrl(u.GetName()))
	pull := fmt.Sprintf("cd /home/application/current && git pull origin master")
	c.Assert(u.RanCommand(clone), Equals, true)
	c.Assert(u.RanCommand(pull), Equals, true)
}

func (s *S) TestGetRepositoryUrl(c *C) {
	url := GetUrl("foobar")
	expected := "git@tsuru.plataformas.glb.com:foobar.git"
	c.Assert(url, Equals, expected)
}

func (s *S) TestGetReadOnlyUrl(c *C) {
	url := GetReadOnlyUrl("foobar")
	expected := "git://tsuru.plataformas.glb.com/foobar.git"
	c.Assert(url, Equals, expected)
}

func (s *S) TestGetPath(c *C) {
	path, err := GetPath()
	c.Assert(err, IsNil)
	expected := "/home/application/current"
	c.Assert(path, Equals, expected)
}

func (s *S) TestGetBarePath(c *C) {
	root, err := config.GetString("git:root")
	c.Assert(err, IsNil)
	path, err := GetBarePath("foobar")
	c.Assert(err, IsNil)
	c.Assert(path, Equals, root+"/foobar.git")
}
