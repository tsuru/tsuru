// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package local

import (
	"bytes"
	"fmt"
	"github.com/globocom/commandmocker"
	"github.com/globocom/config"
	fstesting "github.com/globocom/tsuru/fs/testing"
	"github.com/globocom/tsuru/provision"
	"github.com/globocom/tsuru/testing"
	"io/ioutil"
	"labix.org/v2/mgo/bson"
	"launchpad.net/gocheck"
	"os"
	"time"
)

func (s *S) TestShouldBeRegistered(c *gocheck.C) {
	p, err := provision.Get("local")
	c.Assert(err, gocheck.IsNil)
	c.Assert(p, gocheck.FitsTypeOf, &LocalProvisioner{})
}

func (s *S) TestProvisionerProvision(c *gocheck.C) {
	config.Set("local:authorized-key-path", "somepath")
	rfs := &fstesting.RecordingFs{}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	f, _ := os.Open("testdata/dnsmasq.leases")
	data, err := ioutil.ReadAll(f)
	c.Assert(err, gocheck.IsNil)
	file, err := rfs.Create("/var/lib/misc/dnsmasq.leases")
	c.Assert(err, gocheck.IsNil)
	_, err = file.Write(data)
	c.Assert(err, gocheck.IsNil)
	tmpdir, err := commandmocker.Add("sudo", "$*")
	c.Assert(err, gocheck.IsNil)
	defer commandmocker.Remove(tmpdir)
	sshTempDir, err := commandmocker.Add("ssh", "$*")
	c.Assert(err, gocheck.IsNil)
	defer commandmocker.Remove(sshTempDir)
	scpTempDir, err := commandmocker.Add("scp", "$*")
	c.Assert(err, gocheck.IsNil)
	defer commandmocker.Remove(scpTempDir)
	var p LocalProvisioner
	app := testing.NewFakeApp("myapp", "python", 0)
	defer p.collection().Remove(bson.M{"name": "myapp"})
	c.Assert(p.Provision(app), gocheck.IsNil)
	ok := make(chan bool, 1)
	go func() {
		for {
			coll := s.conn.Collection(s.collName)
			ct, err := coll.Find(bson.M{"name": "myapp", "status": provision.StatusStarted}).Count()
			if err != nil {
				c.Fatal(err)
			}
			if ct > 0 {
				ok <- true
				return
			}
			time.Sleep(1e3)
		}
	}()
	select {
	case <-ok:
	case <-time.After(10e9):
		c.Fatal("Timed out waiting for the container to be provisioned (10 seconds)")
	}
	c.Assert(commandmocker.Ran(tmpdir), gocheck.Equals, true)
	expected := "lxc-create -t ubuntu -n myapp -- -S somepath"
	expected += "lxc-start --daemon -n myapp"
	expected += "service nginx restart"
	c.Assert(commandmocker.Output(tmpdir), gocheck.Equals, expected)
	var unit provision.Unit
	err = s.conn.Collection(s.collName).Find(bson.M{"name": "myapp"}).One(&unit)
	c.Assert(err, gocheck.IsNil)
	c.Assert(unit.Ip, gocheck.Equals, "10.10.10.15")
}

func (s *S) TestProvisionerRestart(c *gocheck.C) {
	var p LocalProvisioner
	tmpdir, err := commandmocker.Add("ssh", "ok")
	c.Assert(err, gocheck.IsNil)
	defer commandmocker.Remove(tmpdir)
	app := testing.NewFakeApp("almah", "static", 1)
	err = p.Restart(app)
	c.Assert(err, gocheck.IsNil)
	ip := app.ProvisionUnits()[0].GetIp()
	expected := []string{
		"-l", "ubuntu", "-q", "-o", "StrictHostKeyChecking no", ip, "/var/lib/tsuru/hooks/restart",
	}
	c.Assert(commandmocker.Ran(tmpdir), gocheck.Equals, true)
	c.Assert(commandmocker.Parameters(tmpdir), gocheck.DeepEquals, expected)
}

func (s *S) TestProvisionerRestartFailure(c *gocheck.C) {
	tmpdir, err := commandmocker.Error("ssh", "fatal unexpected failure", 25)
	c.Assert(err, gocheck.IsNil)
	defer commandmocker.Remove(tmpdir)
	app := testing.NewFakeApp("cribcaged", "python", 1)
	p := LocalProvisioner{}
	err = p.Restart(app)
	c.Assert(err, gocheck.NotNil)
	pErr, ok := err.(*provision.Error)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(pErr.Reason, gocheck.Equals, "fatal unexpected failure")
	c.Assert(pErr.Err.Error(), gocheck.Equals, "exit status 25")
}

func (s *S) TestProvisionerDestroy(c *gocheck.C) {
	config.Set("local:authorized-key-path", "somepath")
	rfs := &fstesting.RecordingFs{}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	f, _ := os.Open("testdata/dnsmasq.leases")
	data, err := ioutil.ReadAll(f)
	c.Assert(err, gocheck.IsNil)
	file, err := rfs.Create("/var/lib/misc/dnsmasq.leases")
	c.Assert(err, gocheck.IsNil)
	_, err = file.Write(data)
	c.Assert(err, gocheck.IsNil)
	tmpdir, err := commandmocker.Add("sudo", "$*")
	c.Assert(err, gocheck.IsNil)
	defer commandmocker.Remove(tmpdir)
	sshTempDir, err := commandmocker.Add("ssh", "$*")
	c.Assert(err, gocheck.IsNil)
	defer commandmocker.Remove(sshTempDir)
	scpTempDir, err := commandmocker.Add("scp", "$*")
	c.Assert(err, gocheck.IsNil)
	defer commandmocker.Remove(scpTempDir)
	var p LocalProvisioner
	app := testing.NewFakeApp("myapp", "python", 0)
	err = p.Provision(app)
	time.Sleep(5 * time.Second)
	c.Assert(err, gocheck.IsNil)
	c.Assert(p.Destroy(app), gocheck.IsNil)
	time.Sleep(5 * time.Second)
	c.Assert(commandmocker.Ran(tmpdir), gocheck.Equals, true)
	expected := "lxc-create -t ubuntu -n myapp -- -S somepath"
	expected += "lxc-start --daemon -n myapp"
	expected += "service nginx restart"
	expected += "lxc-stop -n myapp"
	expected += "lxc-destroy -n myapp"
	c.Assert(commandmocker.Output(tmpdir), gocheck.Equals, expected)
	length, err := p.collection().Find(bson.M{"name": "myapp"}).Count()
	c.Assert(err, gocheck.IsNil)
	c.Assert(length, gocheck.Equals, 0)
}

func (s *S) TestProvisionerAddr(c *gocheck.C) {
	var p LocalProvisioner
	app := testing.NewFakeApp("myapp", "python", 1)
	addr, err := p.Addr(app)
	c.Assert(err, gocheck.IsNil)
	c.Assert(addr, gocheck.Equals, app.ProvisionUnits()[0].GetIp())
}

func (s *S) TestProvisionerAddUnits(c *gocheck.C) {
	var p LocalProvisioner
	app := testing.NewFakeApp("myapp", "python", 0)
	units, err := p.AddUnits(app, 2)
	c.Assert(err, gocheck.IsNil)
	c.Assert(units, gocheck.DeepEquals, []provision.Unit{})
}

func (s *S) TestProvisionerRemoveUnit(c *gocheck.C) {
	var p LocalProvisioner
	app := testing.NewFakeApp("myapp", "python", 0)
	err := p.RemoveUnit(app, "")
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestProvisionerExecuteCommand(c *gocheck.C) {
	var p LocalProvisioner
	var buf bytes.Buffer
	tmpdir, err := commandmocker.Add("ssh", "$*")
	c.Assert(err, gocheck.IsNil)
	defer commandmocker.Remove(tmpdir)
	app := testing.NewFakeApp("almah", "static", 2)
	err = p.ExecuteCommand(&buf, &buf, app, "ls", "-lh")
	c.Assert(err, gocheck.IsNil)
	cmdOutput := fmt.Sprintf("-l ubuntu -q -o StrictHostKeyChecking no %s ls -lh", app.ProvisionUnits()[0].GetIp())
	c.Assert(commandmocker.Ran(tmpdir), gocheck.Equals, true)
	c.Assert(commandmocker.Output(tmpdir), gocheck.Equals, cmdOutput)
}

func (s *S) TestCollectStatus(c *gocheck.C) {
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
		c.Assert(err, gocheck.IsNil)
	}
	units, err := p.CollectStatus()
	c.Assert(err, gocheck.IsNil)
	c.Assert(units, gocheck.DeepEquals, expected)
}

func (s *S) TestProvisionCollection(c *gocheck.C) {
	var p LocalProvisioner
	collection := p.collection()
	c.Assert(collection.Name, gocheck.Equals, s.collName)
}

func (s *S) TestProvisionInstall(c *gocheck.C) {
	tmpdir, err := commandmocker.Add("ssh", "$*")
	c.Assert(err, gocheck.IsNil)
	defer commandmocker.Remove(tmpdir)
	p := LocalProvisioner{}
	err = p.install("10.10.10.10")
	c.Assert(err, gocheck.IsNil)
	c.Assert(commandmocker.Ran(tmpdir), gocheck.Equals, true)
	cmds := []string{
		"-q",
		"-o",
		"StrictHostKeyChecking no",
		"-l",
		"ubuntu",
		"10.10.10.10",
		"sudo /var/lib/tsuru/hooks/install",
	}
	c.Assert(commandmocker.Parameters(tmpdir), gocheck.DeepEquals, cmds)
}

func (s *S) TestProvisionStart(c *gocheck.C) {
	tmpdir, err := commandmocker.Add("ssh", "$*")
	c.Assert(err, gocheck.IsNil)
	defer commandmocker.Remove(tmpdir)
	p := LocalProvisioner{}
	err = p.start("10.10.10.10")
	c.Assert(err, gocheck.IsNil)
	c.Assert(commandmocker.Ran(tmpdir), gocheck.Equals, true)
	cmds := []string{
		"-q",
		"-o",
		"StrictHostKeyChecking no",
		"-l",
		"ubuntu",
		"10.10.10.10",
		"sudo /var/lib/tsuru/hooks/start",
	}
	c.Assert(commandmocker.Parameters(tmpdir), gocheck.DeepEquals, cmds)
}

func (s *S) TestProvisionSetup(c *gocheck.C) {
	tmpdir, err := commandmocker.Add("scp", "$*")
	c.Assert(err, gocheck.IsNil)
	defer commandmocker.Remove(tmpdir)
	sshTempDir, err := commandmocker.Add("ssh", "$*")
	c.Assert(err, gocheck.IsNil)
	defer commandmocker.Remove(sshTempDir)
	p := LocalProvisioner{}
	formulasPath := "/home/ubuntu/formulas"
	config.Set("local:formulas-path", formulasPath)
	err = p.setup("10.10.10.10", "static")
	c.Assert(err, gocheck.IsNil)
	c.Assert(commandmocker.Ran(tmpdir), gocheck.Equals, true)
	cmds := []string{
		"-q",
		"-o",
		"StrictHostKeyChecking no",
		"-r",
		formulasPath + "/static/hooks",
		"ubuntu@10.10.10.10:/var/lib/tsuru",
	}
	c.Assert(commandmocker.Parameters(tmpdir), gocheck.DeepEquals, cmds)
	c.Assert(commandmocker.Ran(sshTempDir), gocheck.Equals, true)
	cmds = []string{
		"-q",
		"-o",
		"StrictHostKeyChecking no",
		"-l",
		"ubuntu",
		"10.10.10.10",
		"sudo mkdir -p /var/lib/tsuru/hooks",
		"-q",
		"-o",
		"StrictHostKeyChecking no",
		"-l",
		"ubuntu",
		"10.10.10.10",
		"sudo chown -R ubuntu /var/lib/tsuru/hooks",
	}
	c.Assert(commandmocker.Parameters(sshTempDir), gocheck.DeepEquals, cmds)
}
