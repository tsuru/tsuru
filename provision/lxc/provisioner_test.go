// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package lxc

import (
	"bytes"
	"github.com/globocom/commandmocker"
	"github.com/globocom/config"
	etesting "github.com/globocom/tsuru/exec/testing"
	fstesting "github.com/globocom/tsuru/fs/testing"
	"github.com/globocom/tsuru/provision"
	rtesting "github.com/globocom/tsuru/router/testing"
	"github.com/globocom/tsuru/testing"
	"io/ioutil"
	"labix.org/v2/mgo/bson"
	"launchpad.net/gocheck"
	"net"
	"os"
	"runtime"
	"time"
)

func (s *S) TestShouldBeRegistered(c *gocheck.C) {
	p, err := provision.Get("lxc")
	c.Assert(err, gocheck.IsNil)
	c.Assert(p, gocheck.FitsTypeOf, &LXCProvisioner{})
}

func (s *S) TestProvisionerProvision(c *gocheck.C) {
	fexec := &etesting.FakeExecutor{}
	execut = fexec
	defer func() {
		execut = nil
	}()
	ln, err := net.Listen("tcp", ":2222")
	c.Assert(err, gocheck.IsNil)
	defer ln.Close()
	formulasPath := "/home/ubuntu/formulas"
	config.Set("lxc:formulas-path", formulasPath)
	config.Set("lxc:ip-timeout", 5)
	config.Set("lxc:ssh-port", 2222)
	config.Set("lxc:authorized-key-path", "somepath")
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
	var p LXCProvisioner
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
			runtime.Gosched()
		}
	}()
	select {
	case <-ok:
	case <-time.After(10e9):
		c.Fatal("Timed out waiting for the container to be provisioned (10 seconds)")
	}
	args := []string{"lxc-create", "-t", "ubuntu-cloud", "-n", "myapp", "--", "-S", "somepath"}
	c.Assert(fexec.ExecutedCmd("sudo", args), gocheck.Equals, true)
	args = []string{"lxc-start", "--daemon", "-n", "myapp"}
	c.Assert(fexec.ExecutedCmd("sudo", args), gocheck.Equals, true)
	c.Assert(rtesting.FakeRouter.HasBackend("myapp"), gocheck.Equals, true)
	c.Assert(rtesting.FakeRouter.HasRoute("myapp", "127.0.0.1"), gocheck.Equals, true)
	var unit provision.Unit
	err = s.conn.Collection(s.collName).Find(bson.M{"name": "myapp"}).One(&unit)
	c.Assert(err, gocheck.IsNil)
	c.Assert(unit.Ip, gocheck.Equals, "127.0.0.1")
}

func (s *S) TestProvisionerRestart(c *gocheck.C) {
	fexec := &etesting.FakeExecutor{}
	execut = fexec
	defer func() {
		execut = nil
	}()
	var p LXCProvisioner
	app := testing.NewFakeApp("almah", "static", 1)
	err := p.Restart(app)
	c.Assert(err, gocheck.IsNil)
	args := []string{"-l", "ubuntu", "-q", "-o", "StrictHostKeyChecking no", "10.10.10.1", "/var/lib/tsuru/hooks/restart"}
	c.Assert(fexec.ExecutedCmd("ssh", args), gocheck.Equals, true)
}

func (s *S) TestDeploy(c *gocheck.C) {
	tmpdir, err := commandmocker.Add("ssh", "")
	c.Assert(err, gocheck.IsNil)
	defer commandmocker.Remove(tmpdir)
	config.Set("git:unit-repo", "test/dir")
	config.Set("git:ro-host", "gandalf.com")
	defer func() {
		config.Unset("git:unit-repo")
		config.Unset("git:ro-host")
	}()
	app := testing.NewFakeApp("cribcaged", "python", 1)
	w := &bytes.Buffer{}
	p := LXCProvisioner{}
	err = p.Deploy(app, w)
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestProvisionerDestroy(c *gocheck.C) {
	fexec := &etesting.FakeExecutor{}
	execut = fexec
	defer func() {
		execut = nil
	}()
	var p LXCProvisioner
	app := testing.NewFakeApp("myapp", "python", 1)
	u := provision.Unit{
		Name:   "myapp",
		Status: provision.StatusStarted,
	}
	err := s.conn.Collection(s.collName).Insert(&u)
	c.Assert(err, gocheck.IsNil)
	c.Assert(p.Destroy(app), gocheck.IsNil)
	ok := make(chan bool, 1)
	go func() {
		for {
			coll := s.conn.Collection(s.collName)
			ct, err := coll.Find(bson.M{"name": "myapp", "status": provision.StatusStarted}).Count()
			if err != nil {
				c.Fatal(err)
			}
			if ct == 0 {
				ok <- true
				return
			}
			runtime.Gosched()
		}
	}()
	select {
	case <-ok:
	case <-time.After(10e9):
		c.Fatal("Timed out waiting for the container to be provisioned (10 seconds)")
	}
	c.Assert(err, gocheck.IsNil)
	args := []string{"lxc-stop", "-n", "myapp"}
	c.Assert(fexec.ExecutedCmd("sudo", args), gocheck.Equals, true)
	args = []string{"lxc-destroy", "-n", "myapp"}
	c.Assert(fexec.ExecutedCmd("sudo", args), gocheck.Equals, true)
	length, err := p.collection().Find(bson.M{"name": "myapp"}).Count()
	c.Assert(err, gocheck.IsNil)
	c.Assert(length, gocheck.Equals, 0)
}

func (s *S) TestProvisionerAddr(c *gocheck.C) {
	config.Set("router", "fake")
	var p LXCProvisioner
	app := testing.NewFakeApp("myapp", "python", 1)
	rtesting.FakeRouter.AddBackend(app.GetName())
	defer rtesting.FakeRouter.RemoveBackend(app.GetName())
	rtesting.FakeRouter.AddRoute(app.GetName(), "http://10.10.10.10")
	addr, err := p.Addr(app)
	c.Assert(err, gocheck.IsNil)
	c.Assert(addr, gocheck.Equals, "http://10.10.10.10")
}

func (s *S) TestProvisionerAddUnits(c *gocheck.C) {
	var p LXCProvisioner
	app := testing.NewFakeApp("myapp", "python", 0)
	units, err := p.AddUnits(app, 2)
	c.Assert(err, gocheck.IsNil)
	c.Assert(units, gocheck.DeepEquals, []provision.Unit{})
}

func (s *S) TestProvisionerRemoveUnit(c *gocheck.C) {
	var p LXCProvisioner
	app := testing.NewFakeApp("myapp", "python", 0)
	err := p.RemoveUnit(app, "")
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestProvisionerInstallDepsExecutesHook(c *gocheck.C) {
	var p LXCProvisioner
	app := testing.NewFakeApp("myapp", "python", 0)
	w := &bytes.Buffer{}
	err := p.InstallDeps(app, w)
	c.Assert(err, gocheck.IsNil)
	expected := []string{"ran /var/lib/tsuru/hooks/dependencies"}
	c.Assert(app.Commands, gocheck.DeepEquals, expected)
}

func (s *S) TestProvisionerExecuteCommand(c *gocheck.C) {
	fexec := &etesting.FakeExecutor{}
	execut = fexec
	defer func() {
		execut = nil
	}()
	var p LXCProvisioner
	var buf bytes.Buffer
	app := testing.NewFakeApp("almah", "static", 2)
	err := p.ExecuteCommand(&buf, &buf, app, "ls", "-lh")
	c.Assert(err, gocheck.IsNil)
	args := []string{"-l", "ubuntu", "-q", "-o", "StrictHostKeyChecking no", "10.10.10.1", "ls", "-lh"}
	c.Assert(fexec.ExecutedCmd("ssh", args), gocheck.Equals, true)
}

func (s *S) TestCollectStatus(c *gocheck.C) {
	var p LXCProvisioner
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
	var p LXCProvisioner
	collection := p.collection()
	c.Assert(collection.Name, gocheck.Equals, s.collName)
}

func (s *S) TestProvisionInstall(c *gocheck.C) {
	fexec := &etesting.FakeExecutor{}
	execut = fexec
	defer func() {
		execut = nil
	}()
	p := LXCProvisioner{}
	err := p.install("10.10.10.10")
	c.Assert(err, gocheck.IsNil)
	cmd := "ssh"
	args := []string{
		"-q",
		"-o",
		"StrictHostKeyChecking no",
		"-l",
		"ubuntu",
		"10.10.10.10",
		"sudo /var/lib/tsuru/hooks/install",
	}
	c.Assert(fexec.ExecutedCmd(cmd, args), gocheck.Equals, true)
}

func (s *S) TestProvisionStart(c *gocheck.C) {
	fexec := &etesting.FakeExecutor{}
	execut = fexec
	defer func() {
		execut = nil
	}()
	p := LXCProvisioner{}
	err := p.start("10.10.10.10")
	c.Assert(err, gocheck.IsNil)
	cmd := "ssh"
	args := []string{
		"-q",
		"-o",
		"StrictHostKeyChecking no",
		"-l",
		"ubuntu",
		"10.10.10.10",
		"sudo /var/lib/tsuru/hooks/start",
	}
	c.Assert(fexec.ExecutedCmd(cmd, args), gocheck.Equals, true)
}

func (s *S) TestProvisionSetup(c *gocheck.C) {
	fexec := &etesting.FakeExecutor{}
	execut = fexec
	defer func() {
		execut = nil
	}()
	p := LXCProvisioner{}
	formulasPath := "/home/ubuntu/formulas"
	config.Set("lxc:formulas-path", formulasPath)
	err := p.setup("10.10.10.10", "static")
	c.Assert(err, gocheck.IsNil)
	cmd := "scp"
	args := []string{
		"-q",
		"-o",
		"StrictHostKeyChecking no",
		"-r",
		formulasPath + "/static/hooks",
		"ubuntu@10.10.10.10:/var/lib/tsuru",
	}
	c.Assert(fexec.ExecutedCmd(cmd, args), gocheck.Equals, true)
	cmd = "ssh"
	args = []string{
		"-q",
		"-o",
		"StrictHostKeyChecking no",
		"-l",
		"ubuntu",
		"10.10.10.10",
		"sudo mkdir -p /var/lib/tsuru/hooks",
	}
	c.Assert(fexec.ExecutedCmd(cmd, args), gocheck.Equals, true)
	args = []string{
		"-q",
		"-o",
		"StrictHostKeyChecking no",
		"-l",
		"ubuntu",
		"10.10.10.10",
		"sudo chown -R ubuntu /var/lib/tsuru/hooks",
	}
	c.Assert(fexec.ExecutedCmd(cmd, args), gocheck.Equals, true)
}
