package repository

import (
	. "launchpad.net/gocheck"
)

func (s *S) TestCloneRepository(c *C) {
	err := Clone("barfoo")
	c.Assert(err, IsNil)
}

func (s *S) TestGetRepositoryUrl(c *C) {
	url := GetUrl("foobar")
	expected := "git@tsuru.plataformas.glb.com:foobar.git"
	c.Assert(url, Equals, expected)
}
