package local

import (
	"bytes"
	"fmt"
	"github.com/globocom/commandmocker"
	"github.com/globocom/tsuru/provision"
	"github.com/globocom/tsuru/testing"
	"labix.org/v2/mgo/bson"
	. "launchpad.net/gocheck"
)

func (s *S) TestShouldBeRegistered(c *C) {
	p, err := provision.Get("local")
	c.Assert(err, IsNil)
	c.Assert(p, FitsTypeOf, &LocalProvisioner{})
}

func (s *S) TestProvisionerProvision(c *C) {
	tmpdir, err := commandmocker.Add("sudo", "$*")
	c.Assert(err, IsNil)
	defer commandmocker.Remove(tmpdir)
	var p LocalProvisioner
	app := testing.NewFakeApp("myapp", "python", 0)
	c.Assert(p.Provision(app), IsNil)
	c.Assert(commandmocker.Ran(tmpdir), Equals, true)
	expected := "lxc-create -t ubuntu -n myapp"
	expected += "lxc-start --daemon -n myapp"
	c.Assert(commandmocker.Output(tmpdir), Equals, expected)
	var unit provision.Unit
	err = p.collection().Find(bson.M{"name": "myapp"}).One(&unit)
	c.Assert(err, IsNil)
	defer p.collection().Remove(bson.M{"name": "myapp"})
}

func (s *S) TestProvisionerDestroy(c *C) {
	tmpdir, err := commandmocker.Add("sudo", "$*")
	c.Assert(err, IsNil)
	defer commandmocker.Remove(tmpdir)
	var p LocalProvisioner
	app := testing.NewFakeApp("myapp", "python", 0)
	err = p.Provision(app)
	c.Assert(err, IsNil)
	c.Assert(p.Destroy(app), IsNil)
	c.Assert(commandmocker.Ran(tmpdir), Equals, true)
	expected := "lxc-create -t ubuntu -n myapp"
	expected += "lxc-start --daemon -n myapp"
	expected += "lxc-stop -n myapp"
	expected += "lxc-destroy -n myapp"
	c.Assert(commandmocker.Output(tmpdir), Equals, expected)
	length, err := p.collection().Find(bson.M{"name": "myapp"}).Count()
	c.Assert(err, IsNil)
	c.Assert(length, Equals, 0)
}

func (s *S) TestProvisionerAddr(c *C) {
	var p LocalProvisioner
	app := testing.NewFakeApp("myapp", "python", 1)
	addr, err := p.Addr(app)
	c.Assert(err, IsNil)
	c.Assert(addr, Equals, app.ProvisionUnits()[0].GetIp())
}

func (s *S) TestProvisionerAddUnits(c *C) {
	var p LocalProvisioner
	app := testing.NewFakeApp("myapp", "python", 0)
	units, err := p.AddUnits(app, 2)
	c.Assert(err, IsNil)
	c.Assert(units, DeepEquals, []provision.Unit{})
}

func (s *S) TestProvisionerRemoveUnit(c *C) {
	var p LocalProvisioner
	app := testing.NewFakeApp("myapp", "python", 0)
	err := p.RemoveUnit(app, "")
	c.Assert(err, IsNil)
}

func (s *S) TestProvisionerExecuteCommand(c *C) {
	var p LocalProvisioner
	var buf bytes.Buffer
	tmpdir, err := commandmocker.Add("ssh", "$*")
	c.Assert(err, IsNil)
	defer commandmocker.Remove(tmpdir)
	app := testing.NewFakeApp("almah", "static", 2)
	err = p.ExecuteCommand(&buf, &buf, app, "ls", "-lh")
	c.Assert(err, IsNil)
	cmdOutput := fmt.Sprintf("-l ubuntu -q -o StrictHostKeyChecking no %s ls -lh", app.ProvisionUnits()[0].GetIp())
	c.Assert(commandmocker.Ran(tmpdir), Equals, true)
	c.Assert(commandmocker.Output(tmpdir), Equals, cmdOutput)
}

func (s *S) TestCollectStatus(c *C) {
	var p LocalProvisioner
	expected := []provision.Unit{
		{
			Name:       "vm1",
			AppName:    "vm1",
			Type:       "django",
			Machine:    0,
			InstanceId: "vm1",
			Ip:         "10.10.10.9",
			Status:     provision.StatusStarted,
		},
		{
			Name:       "vm2",
			AppName:    "vm2",
			Type:       "gunicorn",
			Machine:    0,
			InstanceId: "vm2",
			Ip:         "10.10.10.10",
			Status:     provision.StatusInstalling,
		},
	}
	for _, u := range expected {
		err := p.collection().Insert(u)
		c.Assert(err, IsNil)
	}
	units, err := p.CollectStatus()
	c.Assert(err, IsNil)
	c.Assert(units, DeepEquals, expected)
}

func (s *S) TestProvisionCollection(c *C) {
	var p LocalProvisioner
	collection := p.collection()
	c.Assert(collection.Name, Equals, s.collName)
}
