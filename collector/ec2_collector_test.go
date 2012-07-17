package main

import (
	"bytes"
	"github.com/timeredbull/tsuru/api/service"
	"github.com/timeredbull/tsuru/db"
	tec2 "github.com/timeredbull/tsuru/ec2"
	"github.com/timeredbull/tsuru/log"
	"labix.org/v2/mgo/bson"
	"launchpad.net/goamz/ec2"
	. "launchpad.net/gocheck"
	stdlog "log"
	"strings"
)

func (s *S) TestCollectShouldRetrieveCreatedInstances(c *C) {
	si := service.ServiceInstance{Name: "instance 1", ServiceName: "mysql", Instance: s.instances[0]}
	si.Create()
	si2 := service.ServiceInstance{Name: "instance 2", ServiceName: "mysql", Instance: s.instances[1]}
	si2.Create()
	defer si.Delete()
	defer si2.Delete()
	ec := Ec2Collector{}
	instances, err := ec.Collect()
	c.Assert(err, IsNil)
	c.Assert(len(instances), Equals, 2)
	c.Assert(instances[0].ImageId, Equals, "ami-0000007")
	c.Assert(instances[1].ImageId, Equals, "ami-0000007")
	_, err = tec2.EC2.DeleteSecurityGroup(s.secGroup)
	if err != nil {
		c.Fail()
	}
}

func (s *S) TestCollectShouldSkipIfNoServiceInstancesAreFound(c *C) {
	w := bytes.NewBuffer([]byte{})
	l := stdlog.New(w, "", stdlog.LstdFlags)
	log.Target = l
	ec := Ec2Collector{}
	instances, err := ec.Collect()
	c.Assert(err, IsNil)
	c.Assert(instances, DeepEquals, []ec2.Instance{})
	c.Assert(len(instances), Equals, 0)
	logStr := strings.Replace(w.String(), "\n", "", -1)
	c.Assert(logStr, Matches, ".*no service instances found for collect. Skipping....*")
}

func (s *S) TestUpdateShouldUpdateInstancesStateAndHostInDatabase(c *C) {
	s.createTestInstances(c)
	si := service.ServiceInstance{Name: "instance 1", ServiceName: "mysql", Instance: s.instances[0]}
	si.Create()
	si2 := service.ServiceInstance{Name: "instance 2", ServiceName: "mysql", Instance: s.instances[1]}
	si2.Create()
	defer si.Delete()
	defer si2.Delete()
	ec := Ec2Collector{}
	instances, err := ec.Collect()
	c.Assert(err, IsNil)
	err = ec.Update(instances)
	c.Assert(err, IsNil)
	db.Session.ServiceInstances().Find(bson.M{"_id": si.Name, "service_name": si.ServiceName}).One(&si)
	c.Assert(si.Host, Not(Equals), "")
	c.Assert(si.State, Not(Equals), "creating")
	_, err = tec2.EC2.DeleteSecurityGroup(s.secGroup)
	if err != nil {
		c.Fail()
	}
}

func (s *S) TestInstanceIdsReturnsInstancesIdsOnly(c *C) {
	inst := ec2.Instance{InstanceId: "i-0"}
	instIds := instancesIds([]ec2.Instance{inst})
	c.Assert(instIds, DeepEquals, []string{"i-0"})
}

func (s *S) TestFilterInstancesShouldNotRetrieveEmptyStringsAsIds(c *C) {
	si := service.ServiceInstance{Name: "instance 1", ServiceName: "mysql", Instance: s.instances[0]}
	si.Create()
	defer si.Delete()
	si2 := service.ServiceInstance{Name: "outsider instance", ServiceName: "Alien Service"}
	si2.Create()
	defer si2.Delete()
	insts, n := filterInstances()
	expected := []string{s.instances[0]}
	c.Assert(insts, DeepEquals, expected)
	c.Assert(n, Equals, 1)
}

func (s *S) TestFilterInstancesShouldReturnEmptyStringSliceAndLogMsgWhenNoInstancesAreFound(c *C) {
	si := service.ServiceInstance{Name: "outsider instance", ServiceName: "Alien Service"}
	si.Create()
	defer si.Delete()
	w := bytes.NewBuffer([]byte{})
	l := stdlog.New(w, "", stdlog.LstdFlags)
	log.Target = l
	instIds, n := filterInstances()
	c.Assert(instIds, DeepEquals, []string{})
	logStr := strings.Replace(w.String(), "\n", "", -1)
	c.Assert(logStr, Matches, ".*no service instances found for collect. Skipping....*")
	c.Assert(n, Equals, 0)
}
