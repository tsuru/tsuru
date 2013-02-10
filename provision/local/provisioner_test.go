package local

import (
	"bytes"
	"fmt"
	"github.com/globocom/commandmocker"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/provision"
	"github.com/globocom/tsuru/testing"
	. "launchpad.net/gocheck"
)

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
}

func (s *S) TestProvisionerDestroy(c *C) {
	tmpdir, err := commandmocker.Add("sudo", "$*")
	c.Assert(err, IsNil)
	defer commandmocker.Remove(tmpdir)
	var p LocalProvisioner
	app := testing.NewFakeApp("myapp", "python", 0)
	c.Assert(p.Destroy(app), IsNil)
	c.Assert(commandmocker.Ran(tmpdir), Equals, true)
	expected := "lxc-stop -n myapp"
	expected += "lxc-destroy -n myapp"
	c.Assert(commandmocker.Output(tmpdir), Equals, expected)
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
	units, err := p.CollectStatus()
	c.Assert(err, IsNil)
	c.Assert(units, DeepEquals, expected)
}

func (s *S) TestProvisionCollection(c *C) {
	collName := "collectionName"
	config.Set("local:collection", collName)
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "juju_provision_tests_s")
	var p LocalProvisioner
	collection := p.collection()
	c.Assert(collection.Name, Equals, collName)
}
