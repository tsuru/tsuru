package ec2

import (
	"launchpad.net/goamz/ec2"
	. "launchpad.net/gocheck"
)

func (s *S) TestShouldInstanciateAuthWithConfigValues(c *C) {
	auth, err := getAuth()
	c.Assert(err, IsNil)
	c.Assert(auth.AccessKey, Equals, "8ap28j1d3ojd1398jd")
	c.Assert(auth.SecretKey, Equals, "6gff673g19173gd19n")
}

func (s *S) TestShouldInstanciateRegionWithConfigValues(c *C) {
	region, err := getRegion()
	c.Assert(err, IsNil)
	c.Assert(region.EC2Endpoint, Equals, "http://ec2endpoint.com:8080")
}

func (s *S) TestShouldRunAnInstance(c *C) {
	s.reconfServer(c)
	instId, err := RunInstance("ami-00000001", "")
	c.Assert(err, IsNil)
	i := s.srv.Instance(instId)
	c.Assert(i, Not(IsNil))
}

func (s *S) TestCreateEC2Conn(c *C) {
	conn, err := Conn()
	c.Assert(err, IsNil)
	c.Assert(conn, FitsTypeOf, &ec2.EC2{})
}
