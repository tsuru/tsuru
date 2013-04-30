// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"bytes"
	"fmt"
	"github.com/globocom/config"
	etesting "github.com/globocom/tsuru/exec/testing"
	"github.com/globocom/tsuru/log"
	"github.com/globocom/tsuru/provision"
	"github.com/globocom/tsuru/testing"
	"labix.org/v2/mgo/bson"
	"launchpad.net/gocheck"
	stdlog "log"
	"runtime"
	"time"
)

func (s *S) TestShouldBeRegistered(c *gocheck.C) {
	p, err := provision.Get("docker")
	c.Assert(err, gocheck.IsNil)
	c.Assert(p, gocheck.FitsTypeOf, &DockerProvisioner{})
}

func (s *S) TestProvisionerProvision(c *gocheck.C) {
	app := testing.NewFakeApp("myapp", "python", 1)
	var p DockerProvisioner
	err := p.Provision(app)
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestProvisionerRestart(c *gocheck.C) {
	var p DockerProvisioner
	app := testing.NewFakeApp("almah", "static", 1)
	err := p.Restart(app)
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestDeployShouldCallDockerCreate(c *gocheck.C) {
	out := `
    {
            "NetworkSettings": {
            "IpAddress": "10.10.10.10",
            "IpPrefixLen": 8,
            "Gateway": "10.65.41.1",
            "PortMapping": {}
    }
}`
	fexec := &etesting.FakeExecutor{Output: []byte(out)}
	execut = fexec
	defer func() {
		execut = nil
	}()
	p := DockerProvisioner{}
	app := testing.NewFakeApp("cribcaged", "python", 1)
	w := &bytes.Buffer{}
	err := p.Deploy(app, w)
	defer p.Destroy(app)
	c.Assert(err, gocheck.IsNil)
	args := []string{"run", "-d", fmt.Sprintf("%s/python", s.repoNamespace), fmt.Sprintf("/var/lib/tsuru/deploy git://%s/cribcaged.git", s.gitHost)}
	c.Assert(fexec.ExecutedCmd("docker", args), gocheck.Equals, true)
}

func (s *S) TestDeployShouldCommitImageAndRemoveContainerAfterIt(c *gocheck.C) {
	out := `
    {
            "NetworkSettings": {
            "IpAddress": "10.10.10.10",
            "IpPrefixLen": 8,
            "Gateway": "10.65.41.1",
            "PortMapping": {}
    }
}`
	fexec := &etesting.FakeExecutor{Output: []byte(out)}
	execut = fexec
	defer func() {
		execut = nil
	}()
	p := DockerProvisioner{}
	app := testing.NewFakeApp("cribcaged", "python", 1)
	w := &bytes.Buffer{}
	err := p.Deploy(app, w)
	defer p.Destroy(app)
	c.Assert(err, gocheck.IsNil)
	got := fexec.GetCommands("docker")
	args := []string{"run", "-d", fmt.Sprintf("%s/python", s.repoNamespace), fmt.Sprintf("/var/lib/tsuru/deploy git://%s/cribcaged.git", s.gitHost)}
	c.Assert(got[0].GetArgs(), gocheck.DeepEquals, args)
	c.Assert(got[1].GetArgs()[0], gocheck.Equals, "inspect") // from container.ip call
	c.Assert(got[2].GetArgs()[0], gocheck.Equals, "commit")
	c.Assert(got[2].GetArgs()[2], gocheck.Equals, fmt.Sprintf("%s/cribcaged", s.repoNamespace))
	c.Assert(got[3].GetArgs()[0], gocheck.Equals, "rm")
}

func (s *S) TestDeployShouldCreateContainerForRunningWithGeneratedImage(c *gocheck.C) {
	out := `
    {
            "NetworkSettings": {
            "IpAddress": "10.10.10.10",
            "IpPrefixLen": 8,
            "Gateway": "10.65.41.1",
            "PortMapping": {}
    }
}`
	fexec := &etesting.FakeExecutor{Output: []byte(out)}
	execut = fexec
	defer func() {
		execut = nil
	}()
	p := DockerProvisioner{}
	app := testing.NewFakeApp("cribcaged", "python", 1)
	w := &bytes.Buffer{}
	err := p.Deploy(app, w)
	defer p.Destroy(app)
	c.Assert(err, gocheck.IsNil)
	expected, err := runContainerCmd(app)
	c.Assert(err, gocheck.IsNil)
	c.Assert(fexec.ExecutedCmd(expected[0], expected[1:]), gocheck.Equals, true)
}

func (s *S) TestProvisionerDestroy(c *gocheck.C) {
	fexec := &etesting.FakeExecutor{}
	execut = fexec
	defer func() {
		execut = nil
	}()
	config.Set("docker:authorized-key-path", "somepath")
	w := new(bytes.Buffer)
	l := stdlog.New(w, "", stdlog.LstdFlags)
	log.SetLogger(l)
	app := testing.NewFakeApp("myapp", "python", 1)
	u := provision.Unit{
		Name:       app.ProvisionUnits()[0].GetName(),
		AppName:    app.GetName(),
		Machine:    app.ProvisionUnits()[0].GetMachine(),
		InstanceId: app.ProvisionUnits()[0].GetInstanceId(),
		Status:     provision.StatusCreating,
	}
	err := s.conn.Collection(s.collName).Insert(&u)
	c.Assert(err, gocheck.IsNil)
	img := image{Name: app.GetName()}
	err = s.conn.Collection(s.imageCollName).Insert(&img)
	c.Assert(err, gocheck.IsNil)
	var p DockerProvisioner
	c.Assert(p.Destroy(app), gocheck.IsNil)
	ok := make(chan bool, 1)
	go func() {
		for {
			coll := s.conn.Collection(s.collName)
			ct, err := coll.Find(bson.M{"name": u.Name}).Count()
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
		c.Error("Timed out waiting for the container to be destroyed (10 seconds)")
	}
	args := []string{"stop", "i-01"}
	c.Assert(fexec.ExecutedCmd("docker", args), gocheck.Equals, true)
	args = []string{"rm", "i-01"}
	c.Assert(fexec.ExecutedCmd("docker", args), gocheck.Equals, true)
}

func (s *S) TestProvisionerAddr(c *gocheck.C) {
	var p DockerProvisioner
	app := testing.NewFakeApp("myapp", "python", 1)
	addr, err := p.Addr(app)
	c.Assert(err, gocheck.IsNil)
	c.Assert(addr, gocheck.Equals, app.ProvisionUnits()[0].GetIp())
}

func (s *S) TestProvisionerAddUnits(c *gocheck.C) {
	var p DockerProvisioner
	app := testing.NewFakeApp("myapp", "python", 0)
	units, err := p.AddUnits(app, 2)
	c.Assert(err, gocheck.IsNil)
	c.Assert(units, gocheck.DeepEquals, []provision.Unit{})
}

func (s *S) TestProvisionerRemoveUnit(c *gocheck.C) {
	var p DockerProvisioner
	app := testing.NewFakeApp("myapp", "python", 0)
	err := p.RemoveUnit(app, "")
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestProvisionerExecuteCommand(c *gocheck.C) {
	var p DockerProvisioner
	app := testing.NewFakeApp("almah", "static", 2)
	var buf bytes.Buffer
	err := p.ExecuteCommand(&buf, &buf, app, "ls", "-lh")
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestCollectStatus(c *gocheck.C) {
	var p DockerProvisioner
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
		err := collection().Insert(u)
		c.Assert(err, gocheck.IsNil)
	}
	units, err := p.CollectStatus()
	c.Assert(err, gocheck.IsNil)
	c.Assert(units, gocheck.DeepEquals, expected)
}

func (s *S) TestProvisionCollection(c *gocheck.C) {
	collection := collection()
	c.Assert(collection.Name, gocheck.Equals, s.collName)
}
