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
	inst, err := RunInstance("ami-00000001", "")
	c.Assert(err, IsNil)
	i := s.srv.Instance(inst.Id)
	c.Assert(i, Not(IsNil))
}

func (s *S) TestCreateEC2Conn(c *C) {
	conn, err := Conn()
	c.Assert(err, IsNil)
	c.Assert(conn, FitsTypeOf, &ec2.EC2{})
}

func (s *S) TestAddsUserPublicKeyInUserData(c *C) {
	s.reconfServer(c)
	s.reconfKey(c)
	inst, err := RunInstance("ami-00000001", "")
	c.Assert(err, IsNil)
	i := s.srv.Instance(inst.Id)
	c.Assert(i, Not(IsNil))
	ud := "\necho \"ssh-rsa BBBIA8ZnaC1yc2EAAAABIwAAAQEAs8nQiUnSLFHy8Mx5179FmO/n/HpbGnPtuUx20/S75AszlFSZaFxwYwlvY3P5lNvTiWzGL0JMgj2NGxFPzs4gh9IkRRUnzsNNj2z4cOzyE/6uflivlEsNjYq2lF4LeicZkQ12Ybrg1aCZVTeH38YZZJQQPxLEiXHUwhwi7uvRBiriypl13dc9wVlVhEUOEkyhRjrRh3ONG0euf0+E5YRHIoP7CGZlSZ21hgSyxXjLRmhP3vq62+ql8wGWp4LS2MN47eKt5iUFgE1fLU6rR+VBZWM+zYMx7nz7mIbGdxfYdI6hImStvXov9kOEgbVjkud0m06w2VQ26z85Rlg5ewqdFw== user@host\" >> /root/.ssh/authorized_keys"
	c.Assert(string(i.UserData), Equals, ud)
}

func (s *S) TestAppendUserPublicKeyWithExistingUserData(c *C) {
	s.reconfServer(c)
	s.reconfKey(c)
	inst, err := RunInstance("ami-00000001", "echo something")
	c.Assert(err, IsNil)
	i := s.srv.Instance(inst.Id)
	c.Assert(i, Not(IsNil))
	ud := `echo something
echo "ssh-rsa BBBIA8ZnaC1yc2EAAAABIwAAAQEAs8nQiUnSLFHy8Mx5179FmO/n/HpbGnPtuUx20/S75AszlFSZaFxwYwlvY3P5lNvTiWzGL0JMgj2NGxFPzs4gh9IkRRUnzsNNj2z4cOzyE/6uflivlEsNjYq2lF4LeicZkQ12Ybrg1aCZVTeH38YZZJQQPxLEiXHUwhwi7uvRBiriypl13dc9wVlVhEUOEkyhRjrRh3ONG0euf0+E5YRHIoP7CGZlSZ21hgSyxXjLRmhP3vq62+ql8wGWp4LS2MN47eKt5iUFgE1fLU6rR+VBZWM+zYMx7nz7mIbGdxfYdI6hImStvXov9kOEgbVjkud0m06w2VQ26z85Rlg5ewqdFw== user@host" >> /root/.ssh/authorized_keys`
	c.Assert(string(i.UserData), Equals, ud)
}

func (s *S) TestGetPubKeyFromCurrentUser(c *C) {
	k, err := getPubKey()
	c.Assert(err, IsNil)
	c.Assert(k, Not(Equals), "")
}
