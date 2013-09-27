// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package juju

import (
	"github.com/globocom/commandmocker"
	"github.com/globocom/tsuru/provision"
	"github.com/globocom/tsuru/testing"
	"labix.org/v2/mgo/bson"
	"launchpad.net/gocheck"
	"runtime"
	"strings"
	"sync"
	"time"
)

func (s *S) TestSaveBootstrapMachine(c *gocheck.C) {
	p := JujuProvisioner{}
	m := machine{
		AgentState:    "state",
		IPAddress:     "ip",
		InstanceID:    "id",
		InstanceState: "state",
	}
	p.saveBootstrapMachine(m)
	collection := p.bootstrapCollection()
	defer collection.Close()
	defer collection.Remove(m)
	var mach machine
	collection.Find(nil).One(&mach)
	c.Assert(mach, gocheck.DeepEquals, m)
}

func (s *S) TestCollectStatusShouldNotAddBootstraTwice(c *gocheck.C) {
	tmpdir, err := commandmocker.Add("juju", collectOutput)
	c.Assert(err, gocheck.IsNil)
	defer commandmocker.Remove(tmpdir)
	p := JujuProvisioner{}
	_, err = p.CollectStatus()
	c.Assert(err, gocheck.IsNil)
	_, err = p.CollectStatus()
	c.Assert(err, gocheck.IsNil)
	collection := p.bootstrapCollection()
	defer collection.Close()
	l, err := collection.Find(nil).Count()
	c.Assert(err, gocheck.IsNil)
	c.Assert(l, gocheck.Equals, 1)
}

func (s *S) TestCollectStatus(c *gocheck.C) {
	tmpdir, err := commandmocker.Add("juju", collectOutput)
	c.Assert(err, gocheck.IsNil)
	defer commandmocker.Remove(tmpdir)
	p := JujuProvisioner{}
	collection := p.unitsCollection()
	defer collection.Close()
	err = collection.Insert(instance{UnitName: "as_i_rise/0", InstanceID: "i-00000439"})
	c.Assert(err, gocheck.IsNil)
	defer collection.Remove(bson.M{"_id": bson.M{"$in": []string{"as_i_rise/0", "the_infanta/0"}}})
	expected := []provision.Unit{
		{
			Name:       "as_i_rise/0",
			AppName:    "as_i_rise",
			Type:       "django",
			Machine:    105,
			InstanceId: "i-00000439",
			Ip:         "10.10.10.163",
			Status:     provision.StatusStarted,
		},
		{
			Name:       "the_infanta/0",
			AppName:    "the_infanta",
			Type:       "gunicorn",
			Machine:    107,
			InstanceId: "i-0000043e",
			Ip:         "10.10.10.168",
			Status:     provision.StatusBuilding,
		},
	}
	units, err := p.CollectStatus()
	c.Assert(err, gocheck.IsNil)
	cp := make([]provision.Unit, len(units))
	copy(cp, units)
	if cp[0].Type == "gunicorn" {
		cp[0], cp[1] = cp[1], cp[0]
	}
	c.Assert(cp, gocheck.DeepEquals, expected)
	c.Assert(commandmocker.Ran(tmpdir), gocheck.Equals, true)
	done := make(chan int8)
	go func() {
		for {
			ct, err := collection.Find(nil).Count()
			c.Assert(err, gocheck.IsNil)
			if ct == 2 {
				done <- 1
				return
			}
		}
	}()
	select {
	case <-done:
	case <-time.After(5e9):
		c.Fatal("Did not save the unit after 5 seconds.")
	}
	var instances []instance
	err = collection.Find(nil).Sort("_id").All(&instances)
	c.Assert(err, gocheck.IsNil)
	c.Assert(instances, gocheck.HasLen, 2)
	c.Assert(instances[0].UnitName, gocheck.Equals, "as_i_rise/0")
	c.Assert(instances[0].InstanceID, gocheck.Equals, "i-00000439")
	c.Assert(instances[1].UnitName, gocheck.Equals, "the_infanta/0")
	c.Assert(instances[1].InstanceID, gocheck.Equals, "i-0000043e")
	var b machine
	err = collection.Find(nil).One(&b)
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestCollectStatusDirtyOutput(c *gocheck.C) {
	tmpdir, err := commandmocker.Add("juju", dirtyCollectOutput)
	c.Assert(err, gocheck.IsNil)
	defer commandmocker.Remove(tmpdir)
	expected := []provision.Unit{
		{
			Name:       "as_i_rise/0",
			AppName:    "as_i_rise",
			Type:       "django",
			Machine:    105,
			InstanceId: "i-00000439",
			Ip:         "10.10.10.163",
			Status:     provision.StatusStarted,
		},
		{
			Name:       "the_infanta/1",
			AppName:    "the_infanta",
			Type:       "gunicorn",
			Machine:    107,
			InstanceId: "i-0000043e",
			Ip:         "10.10.10.168",
			Status:     provision.StatusBuilding,
		},
	}
	p := JujuProvisioner{}
	units, err := p.CollectStatus()
	c.Assert(err, gocheck.IsNil)
	cp := make([]provision.Unit, len(units))
	copy(cp, units)
	if cp[0].Type == "gunicorn" {
		cp[0], cp[1] = cp[1], cp[0]
	}
	c.Assert(cp, gocheck.DeepEquals, expected)
	c.Assert(commandmocker.Ran(tmpdir), gocheck.Equals, true)
	var wg sync.WaitGroup
	wg.Add(1)
	collection := p.unitsCollection()
	defer collection.Close()
	go func() {
		q := bson.M{"_id": bson.M{"$in": []string{"as_i_rise/0", "the_infanta/1"}}}
		for {
			if n, _ := collection.Find(q).Count(); n == 2 {
				break
			}
			runtime.Gosched()
		}
		collection.Remove(q)
		wg.Done()
	}()
	wg.Wait()
}

func (s *S) TestCollectStatusIDChangeDisabledELB(c *gocheck.C) {
	tmpdir, err := commandmocker.Add("juju", collectOutput)
	c.Assert(err, gocheck.IsNil)
	defer commandmocker.Remove(tmpdir)
	p := JujuProvisioner{}
	collection := p.unitsCollection()
	defer collection.Close()
	err = collection.Insert(instance{UnitName: "as_i_rise/0", InstanceID: "i-00000239"})
	c.Assert(err, gocheck.IsNil)
	defer collection.Remove(bson.M{"_id": bson.M{"$in": []string{"as_i_rise/0", "the_infanta/0"}}})
	_, err = p.CollectStatus()
	c.Assert(err, gocheck.IsNil)
	done := make(chan int8)
	go func() {
		for {
			q := bson.M{"_id": "as_i_rise/0", "instanceid": "i-00000439"}
			ct, err := collection.Find(q).Count()
			c.Assert(err, gocheck.IsNil)
			if ct == 1 {
				done <- 1
				return
			}
			runtime.Gosched()
		}
	}()
	select {
	case <-done:
	case <-time.After(5e9):
		c.Fatal("Did not update the unit after 5 seconds.")
	}
}

func (s *S) TestCollectStatusIDChangeFromPending(c *gocheck.C) {
	tmpdir, err := commandmocker.Add("juju", collectOutput)
	c.Assert(err, gocheck.IsNil)
	defer commandmocker.Remove(tmpdir)
	p := JujuProvisioner{}
	collection := p.unitsCollection()
	defer collection.Close()
	err = collection.Insert(instance{UnitName: "as_i_rise/0", InstanceID: "pending"})
	c.Assert(err, gocheck.IsNil)
	defer collection.Remove(bson.M{"_id": bson.M{"$in": []string{"as_i_rise/0", "the_infanta/0"}}})
	_, err = p.CollectStatus()
	c.Assert(err, gocheck.IsNil)
	done := make(chan int8)
	go func() {
		for {
			q := bson.M{"_id": "as_i_rise/0", "instanceid": "i-00000439"}
			ct, err := collection.Find(q).Count()
			c.Assert(err, gocheck.IsNil)
			if ct == 1 {
				done <- 1
				return
			}
			runtime.Gosched()
		}
	}()
	select {
	case <-done:
	case <-time.After(5e9):
		c.Fatal("Did not update the unit after 5 seconds.")
	}
}

func (s *S) TestCollectStatusFailure(c *gocheck.C) {
	tmpdir, err := commandmocker.Error("juju", "juju failed", 1)
	c.Assert(err, gocheck.IsNil)
	defer commandmocker.Remove(tmpdir)
	p := JujuProvisioner{}
	_, err = p.CollectStatus()
	c.Assert(err, gocheck.NotNil)
	pErr, ok := err.(*provision.Error)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(pErr.Reason, gocheck.Equals, "juju failed")
	c.Assert(pErr.Err.Error(), gocheck.Equals, "exit status 1")
	c.Assert(commandmocker.Ran(tmpdir), gocheck.Equals, true)
}

func (s *S) TestCollectStatusInvalidYAML(c *gocheck.C) {
	tmpdir, err := commandmocker.Add("juju", "local: somewhere::")
	c.Assert(err, gocheck.IsNil)
	defer commandmocker.Remove(tmpdir)
	p := JujuProvisioner{}
	_, err = p.CollectStatus()
	c.Assert(err, gocheck.NotNil)
	pErr, ok := err.(*provision.Error)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(pErr.Reason, gocheck.Equals, `"juju status" returned invalid data: local: somewhere::`)
	c.Assert(pErr.Err, gocheck.ErrorMatches, `^YAML error:.*$`)
}

func (s *S) TestUnitStatus(c *gocheck.C) {
	var tests = []struct {
		instance     string
		agent        string
		machineAgent string
		expected     provision.Status
	}{
		{"something", "nothing", "wut", provision.StatusBuilding},
		{"", "", "", provision.StatusBuilding},
		{"", "", "pending", provision.StatusBuilding},
		{"", "", "not-started", provision.StatusBuilding},
		{"pending", "", "", provision.StatusBuilding},
		{"", "not-started", "running", provision.StatusBuilding},
		{"error", "install-error", "start-error", provision.StatusDown},
		{"started", "start-error", "running", provision.StatusDown},
		{"started", "charm-upgrade-error", "running", provision.StatusDown},
		{"running", "pending", "running", provision.StatusBuilding},
		{"running", "started", "running", provision.StatusStarted},
		{"running", "down", "running", provision.StatusDown},
	}
	for _, t := range tests {
		got := unitStatus(t.instance, t.agent, t.machineAgent)
		c.Assert(got, gocheck.Equals, t.expected)
	}
}

func (s *ELBSuite) TestCollectStatusWithELBAndIDChange(c *gocheck.C) {
	a := testing.NewFakeApp("symfonia", "symfonia", 0)
	p := JujuProvisioner{}
	router, err := Router()
	c.Assert(err, gocheck.IsNil)
	err = router.AddBackend(a.GetName())
	c.Assert(err, gocheck.IsNil)
	defer router.RemoveBackend(a.GetName())
	id1 := s.server.NewInstance()
	defer s.server.RemoveInstance(id1)
	id2 := s.server.NewInstance()
	defer s.server.RemoveInstance(id2)
	id3 := s.server.NewInstance()
	defer s.server.RemoveInstance(id3)
	collection := p.unitsCollection()
	defer collection.Close()
	err = collection.Insert(instance{UnitName: "symfonia/0", InstanceID: id3})
	c.Assert(err, gocheck.IsNil)
	err = router.AddRoute(a.GetName(), id3)
	c.Assert(err, gocheck.IsNil)
	err = router.AddRoute(a.GetName(), id2)
	c.Assert(err, gocheck.IsNil)
	q := bson.M{"_id": bson.M{"$in": []string{"symfonia/0", "symfonia/1", "symfonia/2", "raise/0"}}}
	defer collection.Remove(q)
	output := strings.Replace(simpleCollectOutput, "i-00004444", id1, 1)
	output = strings.Replace(output, "i-00004445", id2, 1)
	tmpdir, err := commandmocker.Add("juju", output)
	c.Assert(err, gocheck.IsNil)
	defer commandmocker.Remove(tmpdir)
	_, err = p.CollectStatus()
	c.Assert(err, gocheck.IsNil)
	done := make(chan int8)
	go func() {
		for {
			q := bson.M{"_id": "symfonia/0", "instanceid": id1}
			ct, err := collection.Find(q).Count()
			c.Assert(err, gocheck.IsNil)
			if ct == 1 {
				done <- 1
				return
			}
			runtime.Gosched()
		}
	}()
	select {
	case <-done:
	case <-time.After(5e9):
		c.Fatal("Did not save the unit after 5 seconds.")
	}
	resp, err := s.client.DescribeLoadBalancers(a.GetName())
	c.Assert(err, gocheck.IsNil)
	c.Assert(resp.LoadBalancerDescriptions, gocheck.HasLen, 1)
	instances := resp.LoadBalancerDescriptions[0].Instances
	c.Assert(instances, gocheck.HasLen, 2)
	c.Assert(instances[0].InstanceId, gocheck.Equals, id2)
	c.Assert(instances[1].InstanceId, gocheck.Equals, id1)
}
