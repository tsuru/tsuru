// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"bytes"
	"fmt"
	"github.com/globocom/commandmocker"
	"github.com/globocom/config"
	etesting "github.com/globocom/tsuru/exec/testing"
	fstesting "github.com/globocom/tsuru/fs/testing"
	"github.com/globocom/tsuru/log"
	"github.com/globocom/tsuru/provision"
	rtesting "github.com/globocom/tsuru/router/testing"
	"github.com/globocom/tsuru/testing"
	"io/ioutil"
	"labix.org/v2/mgo/bson"
	"launchpad.net/gocheck"
	stdlog "log"
	"os"
	"runtime"
	"time"
)

func (s *S) TestShouldBeRegistered(c *gocheck.C) {
	p, err := provision.Get("docker")
	c.Assert(err, gocheck.IsNil)
	c.Assert(p, gocheck.FitsTypeOf, &DockerProvisioner{})
}

func (s *S) TestProvisionerProvision(c *gocheck.C) {
	fexec := &etesting.FakeExecutor{}
	execut = fexec
	defer func() {
		execut = nil
	}()
	config.Set("docker:authorized-key-path", "somepath")
	formulasPath := "/home/ubuntu/formulas"
	config.Set("docker:formulas-path", formulasPath)
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
	sshTempDir, err := commandmocker.Add("ssh", "$*")
	c.Assert(err, gocheck.IsNil)
	defer commandmocker.Remove(sshTempDir)
	scpTempDir, err := commandmocker.Add("scp", "$*")
	c.Assert(err, gocheck.IsNil)
	defer commandmocker.Remove(scpTempDir)
	var p DockerProvisioner
	app := testing.NewFakeApp("myapp", "python", 0)
	c.Assert(p.Provision(app), gocheck.IsNil)
	defer collection().Remove(bson.M{"name": "myapp"})
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
	args := []string{"run", "-d", fmt.Sprintf("%s/python", s.repoNamespace), fmt.Sprintf("/var/lib/tsuru/deploy git://%s/myapp.git", s.gitHost)}
	c.Assert(fexec.ExecutedCmd("docker", args), gocheck.Equals, true)
	args = []string{"inspect", ""}
	c.Assert(fexec.ExecutedCmd("docker", args), gocheck.Equals, true) // from ip call, the instance id in the end of this command is actually wrong, so we ignore it
	r, err := p.router()
	c.Assert(err, gocheck.IsNil)
	fk := r.(*rtesting.FakeRouter)
	c.Assert(fk.HasRoute("myapp"), gocheck.Equals, true)
}

func (s *S) TestProvisionerProvisionFillsUnitIp(c *gocheck.C) {
	config.Set("docker:authorized-key-path", "somepath")
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
	sshTempDir, err := commandmocker.Add("ssh", "$*")
	c.Assert(err, gocheck.IsNil)
	defer commandmocker.Remove(sshTempDir)
	scpTempDir, err := commandmocker.Add("scp", "$*")
	c.Assert(err, gocheck.IsNil)
	defer commandmocker.Remove(scpTempDir)
	var p DockerProvisioner
	app := testing.NewFakeApp("myapp", "python", 0)
	out := `
    {
            \"NetworkSettings\": {
            \"IpAddress\": \"10.10.10.10\",
            \"IpPrefixLen\": 8,
            \"Gateway\": \"10.65.41.1\",
            \"PortMapping\": {}
    }
}`
	tmpdir, err := commandmocker.Add("docker", out)
	c.Assert(err, gocheck.IsNil)
	defer commandmocker.Remove(tmpdir)
	c.Assert(p.Provision(app), gocheck.IsNil)
	defer collection().Remove(bson.M{"name": "myapp"})
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
	c.Assert(commandmocker.Ran(tmpdir), gocheck.Equals, true)
	var unit provision.Unit
	err = s.conn.Collection(s.collName).Find(bson.M{"name": "myapp"}).One(&unit)
	c.Assert(err, gocheck.IsNil)
	c.Assert(unit.Ip, gocheck.Equals, "10.10.10.10")
}

func (s *S) TestProvisionerRestart(c *gocheck.C) {
	fexec := &etesting.FakeExecutor{}
	execut = fexec
	defer func() {
		execut = nil
	}()
	var p DockerProvisioner
	app := testing.NewFakeApp("almah", "static", 1)
	err := p.Restart(app)
	c.Assert(err, gocheck.IsNil)
	ip := app.ProvisionUnits()[0].GetIp()
	args := []string{
		"-l", "ubuntu", "-q", "-o", "StrictHostKeyChecking no", ip, "/var/lib/tsuru/hooks/restart",
	}
	c.Assert(fexec.ExecutedCmd("ssh", args), gocheck.Equals, true)
}

func (s *S) TestProvisionerRestartFailure(c *gocheck.C) {
	tmpdir, err := commandmocker.Error("ssh", "fatal unexpected failure", 25)
	c.Assert(err, gocheck.IsNil)
	defer commandmocker.Remove(tmpdir)
	app := testing.NewFakeApp("cribcaged", "python", 1)
	p := DockerProvisioner{}
	err = p.Restart(app)
	c.Assert(err, gocheck.NotNil)
	pErr, ok := err.(*provision.Error)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(pErr.Reason, gocheck.Equals, "fatal unexpected failure")
	c.Assert(pErr.Err.Error(), gocheck.Equals, "exit status 25")
}

func (s *S) TestDeployShouldCallDockerCreate(c *gocheck.C) {
	fexec := &etesting.FakeExecutor{}
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
	id := "945132e7b4c9"
	tmpdir, err := commandmocker.Add("docker", id)
	c.Assert(err, gocheck.IsNil)
	defer commandmocker.Remove(tmpdir)
	p := DockerProvisioner{}
	app := testing.NewFakeApp("cribcaged", "python", 1)
	w := &bytes.Buffer{}
	err = p.Deploy(app, w)
	defer p.Destroy(app)
	c.Assert(err, gocheck.IsNil)
	args := []string{"rm", id} // reverse execution order
	args = append([]string{"commit", id, fmt.Sprintf("%s/cribcaged", s.repoNamespace)}, args...)
	args = append([]string{"run", "-d", fmt.Sprintf("%s/python", s.repoNamespace), fmt.Sprintf("/var/lib/tsuru/deploy git://%s/cribcaged.git", s.gitHost)}, args...)
	c.Assert(commandmocker.Ran(tmpdir), gocheck.Equals, true)
	got := commandmocker.Parameters(tmpdir)
	got = got[:len(got)-6] //removes the last docker run executed
	c.Assert(got, gocheck.DeepEquals, args)
}

func (s *S) TestDeployShouldCreateContainerForRunningWithGeneratedImage(c *gocheck.C) {
	fexec := &etesting.FakeExecutor{}
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
	var buf bytes.Buffer
	fexec := &etesting.FakeExecutor{}
	execut = fexec
	defer func() {
		execut = nil
	}()
	app := testing.NewFakeApp("almah", "static", 2)
	err := p.ExecuteCommand(&buf, &buf, app, "ls", "-lh")
	c.Assert(err, gocheck.IsNil)
	ip := app.ProvisionUnits()[0].GetIp()
	args := []string{"-l", "ubuntu", "-q", "-o", "StrictHostKeyChecking no", ip, "ls", "-lh"}
	c.Assert(fexec.ExecutedCmd("ssh", args), gocheck.Equals, true)
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

func (s *S) TestProvisionInstall(c *gocheck.C) {
	fexec := &etesting.FakeExecutor{}
	execut = fexec
	defer func() {
		execut = nil
	}()
	p := DockerProvisioner{}
	err := p.install("10.10.10.10")
	c.Assert(err, gocheck.IsNil)
	args := []string{
		"-q",
		"-o",
		"StrictHostKeyChecking no",
		"-l",
		"ubuntu",
		"10.10.10.10",
		"sudo /var/lib/tsuru/hooks/install",
	}
	c.Assert(fexec.ExecutedCmd("ssh", args), gocheck.Equals, true)
}

func (s *S) TestProvisionStart(c *gocheck.C) {
	fexec := &etesting.FakeExecutor{}
	execut = fexec
	defer func() {
		execut = nil
	}()
	p := DockerProvisioner{}
	err := p.start("10.10.10.10")
	c.Assert(err, gocheck.IsNil)
	args := []string{
		"-q",
		"-o",
		"StrictHostKeyChecking no",
		"-l",
		"ubuntu",
		"10.10.10.10",
		"sudo /var/lib/tsuru/hooks/start",
	}
	c.Assert(fexec.ExecutedCmd("ssh", args), gocheck.Equals, true)
}

func (s *S) TestProvisionSetup(c *gocheck.C) {
	fexec := &etesting.FakeExecutor{}
	execut = fexec
	defer func() {
		execut = nil
	}()
	p := DockerProvisioner{}
	formulasPath := "/home/ubuntu/formulas"
	config.Set("docker:formulas-path", formulasPath)
	err := p.setup("10.10.10.10", "static")
	c.Assert(err, gocheck.IsNil)
	args := []string{
		"-q",
		"-o",
		"StrictHostKeyChecking no",
		"-r",
		formulasPath + "/static/hooks",
		"ubuntu@10.10.10.10:/var/lib/tsuru",
	}
	c.Assert(fexec.ExecutedCmd("scp", args), gocheck.Equals, true)
	args = []string{
		"-q",
		"-o",
		"StrictHostKeyChecking no",
		"-l",
		"ubuntu",
		"10.10.10.10",
		"sudo mkdir -p /var/lib/tsuru/hooks",
	}
	c.Assert(fexec.ExecutedCmd("ssh", args), gocheck.Equals, true)
	args = []string{
		"-q",
		"-o",
		"StrictHostKeyChecking no",
		"-l",
		"ubuntu",
		"10.10.10.10",
		"sudo chown -R ubuntu /var/lib/tsuru/hooks",
	}
	c.Assert(fexec.ExecutedCmd("ssh", args), gocheck.Equals, true)
}
