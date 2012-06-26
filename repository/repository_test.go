package repository

import (
	"github.com/timeredbull/commandmocker"
	. "launchpad.net/gocheck"
)

func (s *S) TestCloneRepository(c *C) {
	_, err := Clone("barfoo", 1)
	c.Assert(err, IsNil)
}

func (s *S) TestPullRepository(c *C) {
	dir, err := commandmocker.Add("juju", "$*")
	c.Assert(err, IsNil)
	defer commandmocker.Remove(dir)
	out, err := Pull("barfoo", 1)
	c.Assert(err, IsNil)
	c.Assert(string(out), Matches, ".*cd /home/application/current && git pull origin master$")
}

func (s *S) TestCloneOrPullRepository(c *C) {
	_, err := CloneOrPull("someapp", 2)
	c.Assert(err, IsNil)
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
