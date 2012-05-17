package repository

import (
	. "launchpad.net/gocheck"
)

func (s *S) TestCloneRepository(c *C) {
	err := CloneRepository("barfoo")
	c.Assert(err, IsNil)
}

func (s *S) TestGetRepositoryUrl(c *C) {
	url := GetRepositoryUrl("foobar")
	expected := "git@tsuru.plataformas.glb.com:foobar.git"
	c.Assert(url, Equals, expected)
}
