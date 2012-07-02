package ec2

import (
	"github.com/timeredbull/tsuru/config"
	"io/ioutil"
	"launchpad.net/goamz/aws"
	"launchpad.net/goamz/ec2"
	"launchpad.net/goamz/ec2/ec2test"
	. "launchpad.net/gocheck"
	"testing"
)

type S struct {
	srv  *ec2test.Server
	conn *ec2.EC2
}

var _ = Suite(&S{})

func Test(t *testing.T) { TestingT(t) }

func (s *S) SetUpSuite(c *C) {
	var err error
	s.srv, err = ec2test.NewServer()
	if err != nil {
		c.Fatal(err)
	}
	s.setupConfig(c)
}

func (s *S) reconfServer(c *C) {
	region := aws.Region{EC2Endpoint: s.srv.URL()}
	auth, err := getAuth()
	c.Assert(err, IsNil)
	EC2 = ec2.New(*auth, region)
}

func (s *S) reconfKey(c *C) {
	pubKey = []byte("ssh-rsa BBBIA8ZnaC1yc2EAAAABIwAAAQEAs8nQiUnSLFHy8Mx5179FmO/n/HpbGnPtuUx20/S75AszlFSZaFxwYwlvY3P5lNvTiWzGL0JMgj2NGxFPzs4gh9IkRRUnzsNNj2z4cOzyE/6uflivlEsNjYq2lF4LeicZkQ12Ybrg1aCZVTeH38YZZJQQPxLEiXHUwhwi7uvRBiriypl13dc9wVlVhEUOEkyhRjrRh3ONG0euf0+E5YRHIoP7CGZlSZ21hgSyxXjLRmhP3vq62+ql8wGWp4LS2MN47eKt5iUFgE1fLU6rR+VBZWM+zYMx7nz7mIbGdxfYdI6hImStvXov9kOEgbVjkud0m06w2VQ26z85Rlg5ewqdFw== user@host")
}

func (s *S) TearDownSuite(c *C) {
	s.srv.Quit()
}

func (s *S) setupConfig(c *C) {
	data, err := ioutil.ReadFile("../etc/tsuru.conf")
	if err != nil {
		c.Fatal(err)
	}
	err = config.ReadConfigBytes(data)
	if err != nil {
		c.Fatal(err)
	}
}
