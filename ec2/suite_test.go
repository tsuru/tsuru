package ec2

import (
	"launchpad.net/goamz/ec2/ec2test"
	. "launchpad.net/gocheck"
	"testing"
)

type S struct{}

func Test(t *testing.T) { TestingT(t) }

func (s *S) SetUpSuite(c *C) {
	srv, err := ec2test.NewServer()
	if err != nil {
		c.Fatal(err)
	}
}
