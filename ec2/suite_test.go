package ec2

import (
	"github.com/timeredbull/tsuru/config"
	"io/ioutil"
	"launchpad.net/goamz/ec2/ec2test"
	. "launchpad.net/gocheck"
	"testing"
)

type S struct {
	srv *ec2test.Server
}

var _ = Suite(&S{})

func Test(t *testing.T) { TestingT(t) }

func (s *S) SetUpSuite(c *C) {
	var err error
	s.srv, err = ec2test.NewServer()
	if err != nil {
		c.Fatal(err)
	}
	setupConfig(c)
}

func (s *S) TearDownSuite(c *C) {
	s.srv.Quit()
}

func setupConfig(c *C) {
	data, err := ioutil.ReadFile("../etc/tsuru.conf")
	if err != nil {
		c.Fatal(err)
	}
	err = config.ReadConfigBytes(data)
	if err != nil {
		c.Fatal(err)
	}
}
