package ec2

import (
	. "launchpad.net/gocheck"
)

func (s *S) TestShouldInstanciateAuthWithConfigValues(c *C) {
	auth, err := getAuth()
	c.Assert(err, IsNil)
	c.Assert(auth.AccessKey, Equals, "8ap28j1d3ojd1398jd")
	c.Assert(auth.SecretKey, Equals, "6gff673g19173gd19n")
}
