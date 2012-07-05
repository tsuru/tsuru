package main

import (
	"bytes"
	"github.com/timeredbull/tsuru/api/service"
	tec2 "github.com/timeredbull/tsuru/ec2"
	"github.com/timeredbull/tsuru/log"
	"launchpad.net/goamz/ec2"
	. "launchpad.net/gocheck"
	stdlog "log"
	"strings"
)

func (s *S) TestCollectShouldRetrieveCreatedInstances(c *C) {
	s.createTestInstances(c)
	si := service.ServiceInstance{Name: "instance 1", ServiceName: "mysql", Instance: s.instances[0]}
	si.Create()
	si2 := service.ServiceInstance{Name: "instance 2", ServiceName: "mysql", Instance: s.instances[1]}
	si2.Create()
	defer si.Delete()
	defer si2.Delete()
	instances, err := CollectEc2()
	c.Assert(err, IsNil)
	c.Assert(len(instances), Equals, 2)
	c.Assert(instances[0].ImageId, Equals, "ami-0000007")
	c.Assert(instances[1].ImageId, Equals, "ami-0000007")
	_, err = tec2.EC2.TerminateInstances(s.instances)
	if err != nil {
		c.Fail()
	}
}

func (s *S) TestCollectShouldSkipIfNoServiceInstancesAreFound(c *C) {
	w := bytes.NewBuffer([]byte{})
	l := stdlog.New(w, "", stdlog.LstdFlags)
	log.Target = l
	instances, err := CollectEc2()
	c.Assert(err, IsNil)
	c.Assert(instances, FitsTypeOf, []ec2.Instance{})
	c.Assert(len(instances), Equals, 0)
	logStr := strings.Replace(w.String(), "\n", "", -1)
	c.Assert(logStr, Matches, ".*no service instances found for collect. Skipping....*")
}
