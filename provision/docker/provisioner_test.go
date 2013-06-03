// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"bytes"
	"fmt"
	"github.com/globocom/tsuru/exec"
	etesting "github.com/globocom/tsuru/exec/testing"
	"github.com/globocom/tsuru/log"
	"github.com/globocom/tsuru/provision"
	rtesting "github.com/globocom/tsuru/router/testing"
	"github.com/globocom/tsuru/testing"
	"labix.org/v2/mgo/bson"
	"launchpad.net/gocheck"
	stdlog "log"
	"net"
	"runtime"
	"strings"
	"time"
)

func setExecut(e exec.Executor) {
	emutex.Lock()
	execut = e
	emutex.Unlock()
}

func (s *S) TestShouldBeRegistered(c *gocheck.C) {
	p, err := provision.Get("docker")
	c.Assert(err, gocheck.IsNil)
	c.Assert(p, gocheck.FitsTypeOf, &dockerProvisioner{})
}

func (s *S) TestProvisionerProvision(c *gocheck.C) {
	app := testing.NewFakeApp("myapp", "python", 1)
	var p dockerProvisioner
	err := p.Provision(app)
	c.Assert(err, gocheck.IsNil)
	c.Assert(rtesting.FakeRouter.HasBackend("myapp"), gocheck.Equals, true)
	c.Assert(app.IsReady(), gocheck.Equals, true)
}

func (s *S) TestProvisionerRestartCallsTheRestartHook(c *gocheck.C) {
	id := "caad7bbd5411"
	fexec := &etesting.FakeExecutor{Output: map[string][][]byte{"*": {[]byte(id)}}}
	setExecut(fexec)
	defer setExecut(nil)
	var p dockerProvisioner
	app := testing.NewFakeApp("almah", "static", 1)
	cont := container{
		Id:      id,
		AppName: app.GetName(),
		Type:    app.GetPlatform(),
		Ip:      "10.10.10.10",
	}
	err := collection().Insert(cont)
	c.Assert(err, gocheck.IsNil)
	defer collection().RemoveId(cont.Id)
	err = p.Restart(app)
	c.Assert(err, gocheck.IsNil)
	args := []string{
		cont.Ip, "-l", s.sshUser, "-o", "StrictHostKeyChecking no",
		"--", "/var/lib/tsuru/restart",
	}
	c.Assert(fexec.ExecutedCmd("ssh", args), gocheck.Equals, true)
}

func (s *S) TestDeployShouldCallDockerCreate(c *gocheck.C) {
	out := `{
	"NetworkSettings": {
		"IpAddress": "10.10.10.10",
		"IpPrefixLen": 8,
		"Gateway": "10.65.41.1",
		"PortMapping": {"8888": "37574"}
	}
}`
	fexec := &etesting.FakeExecutor{Output: map[string][][]byte{"*": {[]byte(out)}}}
	setExecut(fexec)
	defer setExecut(nil)
	p := dockerProvisioner{}
	app := testing.NewFakeApp("cribcaged", "python", 1)
	p.Provision(app)
	defer p.Destroy(app)
	w := &bytes.Buffer{}
	err := p.Deploy(app, w)
	defer p.Destroy(app)
	defer s.conn.Collection(s.collName).RemoveId(out)
	c.Assert(err, gocheck.IsNil)
	image := fmt.Sprintf("%s/python", s.repoNamespace)
	appRepo := fmt.Sprintf("git://%s/cribcaged.git", s.gitHost)
	sshCmd := "/var/lib/tsuru/add-key key-content && /usr/sbin/sshd -D"
	args := []string{
		"run", "-d", "-t", "-p", s.port, image,
		"/bin/bash", "-c", sshCmd,
	}
	c.Assert(fexec.ExecutedCmd("docker", args), gocheck.Equals, true)
	deployArgs := []string{
		"10.10.10.10", "-l", s.sshUser, "-o", "StrictHostKeyChecking no",
		"--", "/var/lib/tsuru/deploy", appRepo,
	}
	c.Assert(fexec.ExecutedCmd("ssh", deployArgs), gocheck.Equals, true)
	runArgs := []string{
		"10.10.10.10", "-l", s.sshUser, "-o", "StrictHostKeyChecking no",
		"--", s.runBin, s.runArgs,
	}
	c.Assert(fexec.ExecutedCmd("ssh", runArgs), gocheck.Equals, true)
}

func (s *S) TestDeployShouldReplaceAllContainers(c *gocheck.C) {
	var p dockerProvisioner
	s.conn.Collection(s.collName).Insert(
		container{Id: "app/0", AppName: "app"},
		container{Id: "app/1", AppName: "app"},
	)
	defer s.conn.Collection(s.collName).RemoveAll(bson.M{"appname": "app"})
	app := testing.NewFakeApp("app", "python", 0)
	p.Provision(app)
	defer p.Destroy(app)
	app.AddUnit(&testing.FakeUnit{Name: "app/0"})
	app.AddUnit(&testing.FakeUnit{Name: "app/1"})
	out := `{
	"NetworkSettings": {
		"IpAddress": "10.10.10.%d",
		"IpPrefixLen": 8,
		"Gateway": "10.65.41.1",
		"PortMapping": {"8888": "37574"}
	}
}`
	fexec := &etesting.FakeExecutor{
		Output: map[string][][]byte{
			"*":            {[]byte("c-60"), []byte("c-61")},
			"inspect c-60": {[]byte(fmt.Sprintf(out, 1))},
			"inspect c-61": {[]byte(fmt.Sprintf(out, 2))},
		},
	}
	setExecut(fexec)
	defer setExecut(nil)
	var w bytes.Buffer
	err := p.Deploy(app, &w)
	defer p.Destroy(app)
	defer s.conn.Collection(s.collName).RemoveId(out)
	c.Assert(err, gocheck.IsNil)
	c.Assert(app.ProvisionUnits(), gocheck.HasLen, 0)
	commands := fexec.GetCommands("ssh")
	c.Assert(commands, gocheck.HasLen, 4)
}

func (s *S) TestDeployShouldRestart(c *gocheck.C) {
	out := `{
	"NetworkSettings": {
		"IpAddress": "10.10.10.10",
		"IpPrefixLen": 8,
		"Gateway": "10.65.41.1",
		"PortMapping": {"8888": "37574"}
	}
}`
	fexec := &etesting.FakeExecutor{
		Output: map[string][][]byte{
			"*": {[]byte("8yasfiajfias")},
			"inspect 8yasfiajfias": {[]byte(out)},
		},
	}
	setExecut(fexec)
	defer setExecut(nil)
	p := dockerProvisioner{}
	app := testing.NewFakeApp("cribcaged", "python", 1)
	p.Provision(app)
	defer p.Destroy(app)
	w := &bytes.Buffer{}
	err := p.Deploy(app, w)
	defer p.Destroy(app)
	defer s.conn.Collection(s.collName).RemoveId(out)
	c.Assert(err, gocheck.IsNil)
	c.Assert(app.Commands, gocheck.DeepEquals, []string{"restart"})
}

func (s *S) TestDeployFailureFirstStep(c *gocheck.C) {
	var (
		p   dockerProvisioner
		buf bytes.Buffer
	)
	app := testing.NewFakeApp("app", "python", 0)
	app.AddUnit(&testing.FakeUnit{Name: "app/0"})
	fexec := etesting.ErrorExecutor{
		FakeExecutor: etesting.FakeExecutor{
			Output: map[string][][]byte{
				"*": {[]byte("failed to start container")},
			},
		},
	}
	setExecut(&fexec)
	defer setExecut(nil)
	err := p.Deploy(app, &buf)
	c.Assert(err, gocheck.NotNil)
}

func (s *S) TestDeployFailureSecondStep(c *gocheck.C) {
	var (
		p   dockerProvisioner
		buf bytes.Buffer
	)
	app := testing.NewFakeApp("app", "python", 0)
	app.AddUnit(&testing.FakeUnit{Name: "app/0"})
	p.Provision(app)
	defer p.Destroy(app)
	defer s.conn.Collection(s.collName).RemoveAll(bson.M{"appname": app.GetName()})
	output := `{
	"NetworkSettings": {
		"IpAddress": "10.10.10.%d",
		"IpPrefixLen": 8,
		"Gateway": "10.65.41.1",
		"PortMapping": {"8888": "37574"}
	}
}`
	fexec := etesting.FailLaterExecutor{
		Succeeds: 3,
		FakeExecutor: etesting.FakeExecutor{
			Output: map[string][][]byte{
				"*":              {[]byte("c-0955")},
				"inspect c-0955": {[]byte(output)},
			},
		},
	}
	setExecut(&fexec)
	defer setExecut(nil)
	err := p.Deploy(app, &buf)
	c.Assert(err, gocheck.NotNil)
	c.Assert(buf.String(), gocheck.Equals, "c-0955")
	c.Assert(fexec.ExecutedCmd("docker", []string{"rm", "c-0955"}), gocheck.Equals, true)
	c.Assert(app.ProvisionUnits(), gocheck.HasLen, 1)
}

func (s *S) TestProvisionerDestroy(c *gocheck.C) {
	fexec := &etesting.FakeExecutor{}
	setExecut(fexec)
	defer setExecut(nil)
	w := new(bytes.Buffer)
	l := stdlog.New(w, "", stdlog.LstdFlags)
	log.SetLogger(l)
	app := testing.NewFakeApp("myapp", "python", 1)
	cont := container{
		Id:      app.ProvisionUnits()[0].GetName(),
		AppName: app.GetName(),
	}
	err := s.conn.Collection(s.collName).Insert(cont)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Collection(s.collName).RemoveId(cont.Id)
	s.conn.Collection(s.collName).Insert(container{Id: "something-01", AppName: app.GetName()})
	defer s.conn.Collection(s.collName).RemoveId("something-01")
	img := image{Name: app.GetName()}
	err = s.conn.Collection(s.imageCollName).Insert(&img)
	c.Assert(err, gocheck.IsNil)
	var p dockerProvisioner
	p.Provision(app)
	c.Assert(p.Destroy(app), gocheck.IsNil)
	ok := make(chan bool, 1)
	go func() {
		coll := s.conn.Collection(s.collName)
		for {
			ct, err := coll.Find(bson.M{"appname": cont.AppName}).Count()
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
		c.Fatal("Timed out waiting for the container to be destroyed (10 seconds)")
	}
	args := []string{"rm", "myapp/0"}
	c.Assert(fexec.ExecutedCmd("docker", args), gocheck.Equals, true)
	c.Assert(rtesting.FakeRouter.HasBackend("myapp"), gocheck.Equals, false)
}

func (s *S) TestProvisionerDestroyEmptyUnit(c *gocheck.C) {
	fexec := &etesting.FakeExecutor{}
	setExecut(fexec)
	defer setExecut(nil)
	w := new(bytes.Buffer)
	l := stdlog.New(w, "", stdlog.LstdFlags)
	log.SetLogger(l)
	app := testing.NewFakeApp("myapp", "python", 0)
	app.AddUnit(&testing.FakeUnit{})
	var p dockerProvisioner
	p.Provision(app)
	err := p.Destroy(app)
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestProvisionerDestroyRemovesRouterBackend(c *gocheck.C) {
	fexec := &etesting.FakeExecutor{}
	setExecut(fexec)
	defer setExecut(nil)
	app := testing.NewFakeApp("myapp", "python", 0)
	var p dockerProvisioner
	err := p.Provision(app)
	c.Assert(err, gocheck.IsNil)
	err = p.Destroy(app)
	c.Assert(err, gocheck.IsNil)
	c.Assert(rtesting.FakeRouter.HasBackend("myapp"), gocheck.Equals, false)
}

func (s *S) TestProvisionerAddr(c *gocheck.C) {
	out := `{
	"NetworkSettings": {
		"IpAddress": "10.10.10.10",
		"IpPrefixLen": 8,
		"Gateway": "10.65.41.1",
		"PortMapping": {"8888": "37574"}
	}
}`
	id := "123"
	runCmd := "run -d -t -p 8888 tsuru/python /bin/bash -c /var/lib/tsuru/add-key key-content && /usr/sbin/sshd -D"
	fexec := &etesting.FakeExecutor{Output: map[string][][]byte{runCmd: {[]byte(id)}, "inspect " + id: {[]byte(out)}}}
	setExecut(fexec)
	defer setExecut(nil)
	var p dockerProvisioner
	app := testing.NewFakeApp("myapp", "python", 1)
	p.Provision(app)
	defer p.Destroy(app)
	w := &bytes.Buffer{}
	err := p.Deploy(app, w)
	c.Assert(err, gocheck.IsNil)
	defer p.Destroy(app)
	defer s.conn.Collection(s.collName).RemoveId(id)
	addr, err := p.Addr(app)
	c.Assert(err, gocheck.IsNil)
	r, err := getRouter()
	c.Assert(err, gocheck.IsNil)
	expected, err := r.Addr("myapp")
	c.Assert(err, gocheck.IsNil)
	c.Assert(addr, gocheck.Equals, expected)
}

func (s *S) TestProvisionerAddUnits(c *gocheck.C) {
	sshCmd := "/var/lib/tsuru/add-key key-content && /usr/sbin/sshd -D"
	runCmd := fmt.Sprintf("run -d -t -p %s tsuru/python /bin/bash -c %s", s.port, sshCmd)
	out := `{
	"NetworkSettings": {
		"IpAddress": "10.10.10.%d",
		"IpPrefixLen": 8,
		"Gateway": "10.65.41.1",
		"PortMapping": {"8888": "37574"}
	}
}`
	fexec := etesting.FakeExecutor{
		Output: map[string][][]byte{
			runCmd:          {[]byte("c-300"), []byte("c-301"), []byte("c-302")},
			"inspect c-300": {[]byte(fmt.Sprintf(out, 1))},
			"inspect c-301": {[]byte(fmt.Sprintf(out, 2))},
			"inspect c-302": {[]byte(fmt.Sprintf(out, 3))},
			"*":             {[]byte("ok sir")},
		},
	}
	setExecut(&fexec)
	defer setExecut(nil)
	var p dockerProvisioner
	app := testing.NewFakeApp("myapp", "python", 0)
	p.Provision(app)
	defer p.Destroy(app)
	s.conn.Collection(s.collName).Insert(container{Id: "c-89320", AppName: app.GetName()})
	defer s.conn.Collection(s.collName).RemoveId("c-89320")
	expected := []provision.Unit{
		{Name: "c-300", AppName: app.GetName(),
			Type: app.GetPlatform(), Ip: "10.10.10.1",
			Status: provision.StatusInstalling},
		{Name: "c-301", AppName: app.GetName(),
			Type: app.GetPlatform(), Ip: "10.10.10.2",
			Status: provision.StatusInstalling},
		{Name: "c-302", AppName: app.GetName(),
			Type: app.GetPlatform(), Ip: "10.10.10.3",
			Status: provision.StatusInstalling},
	}
	units, err := p.AddUnits(app, 3)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Collection(s.collName).RemoveAll(bson.M{"appname": app.GetName()})
	c.Assert(units, gocheck.DeepEquals, expected)
	c.Assert(fexec.ExecutedCmd("docker", []string{"inspect", "c-300"}), gocheck.Equals, true)
	c.Assert(fexec.ExecutedCmd("docker", []string{"inspect", "c-301"}), gocheck.Equals, true)
	c.Assert(fexec.ExecutedCmd("docker", []string{"inspect", "c-302"}), gocheck.Equals, true)
	count, err := s.conn.Collection(s.collName).Find(bson.M{"appname": app.GetName()}).Count()
	c.Assert(err, gocheck.IsNil)
	c.Assert(count, gocheck.Equals, 4)
	ok := make(chan bool, 1)
	go func() {
		for {
			commands := fexec.GetCommands("ssh")
			if len(commands) == 6 {
				ok <- true
				return
			}
			runtime.Gosched()
		}
	}()
	select {
	case <-ok:
	case <-time.After(5e9):
		c.Fatal("Did not run deploy script on containers after 5 seconds.")
	}
}

func (s *S) TestProvisionerAddZeroUnits(c *gocheck.C) {
	var p dockerProvisioner
	units, err := p.AddUnits(nil, 0)
	c.Assert(units, gocheck.IsNil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "Cannot add 0 units")
}

func (s *S) TestProvisionerAddUnitsFailure(c *gocheck.C) {
	fexec := etesting.ErrorExecutor{}
	setExecut(&fexec)
	defer setExecut(nil)
	app := testing.NewFakeApp("myapp", "python", 1)
	s.conn.Collection(s.collName).Insert(container{Id: "c-89320", AppName: app.GetName()})
	defer s.conn.Collection(s.collName).RemoveId("c-89320")
	var p dockerProvisioner
	units, err := p.AddUnits(app, 1)
	c.Assert(units, gocheck.IsNil)
	c.Assert(err, gocheck.NotNil)
}

func (s *S) TestProvisionerAddUnitsWithoutContainers(c *gocheck.C) {
	app := testing.NewFakeApp("myapp", "python", 1)
	var p dockerProvisioner
	p.Provision(app)
	defer p.Destroy(app)
	units, err := p.AddUnits(app, 1)
	c.Assert(units, gocheck.IsNil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "New units can only be added after the first deployment")
}

func (s *S) TestProvisionerRemoveUnit(c *gocheck.C) {
	out := `{
	"NetworkSettings": {
		"IpAddress": "127.0.0.1",
		"IpPrefixLen": 8,
		"Gateway": "10.65.41.1",
		"PortMapping": {
			"8888": "90293"
		}
	}
}`
	app := testing.NewFakeApp("myapp", "python", 0)
	fexec := &etesting.FakeExecutor{
		Output: map[string][][]byte{
			"*":            {[]byte("c-10")},
			"inspect c-10": {[]byte(out)},
		},
	}
	setExecut(fexec)
	defer setExecut(nil)
	container, err := newContainer(app)
	c.Assert(err, gocheck.IsNil)
	defer container.remove()
	var p dockerProvisioner
	err = p.RemoveUnit(app, container.Id)
	c.Assert(err, gocheck.IsNil)
	_, err = getContainer(container.Id)
	c.Assert(err, gocheck.NotNil)
}

func (s *S) TestProvisionerRemoveUnitNotFound(c *gocheck.C) {
	var p dockerProvisioner
	err := p.RemoveUnit(nil, "not-found")
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "not found")
}

func (s *S) TestProvisionerRemoveUnitNotInApp(c *gocheck.C) {
	out := `{
	"NetworkSettings": {
		"IpAddress": "127.0.0.1",
		"IpPrefixLen": 8,
		"Gateway": "10.65.41.1",
		"PortMapping": {
			"8888": "90293"
		}
	}
}`
	app := testing.NewFakeApp("myapp", "python", 0)
	fexec := &etesting.FakeExecutor{
		Output: map[string][][]byte{
			"*":            {[]byte("c-10")},
			"inspect c-10": {[]byte(out)},
		},
	}
	setExecut(fexec)
	defer setExecut(nil)
	container, err := newContainer(app)
	c.Assert(err, gocheck.IsNil)
	defer container.remove()
	var p dockerProvisioner
	err = p.RemoveUnit(testing.NewFakeApp("hisapp", "python", 1), container.Id)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "Unit does not belong to this app")
	_, err = getContainer(container.Id)
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestProvisionerExecuteCommand(c *gocheck.C) {
	fexec := &etesting.FakeExecutor{
		Output: map[string][][]byte{"*": {[]byte(". ..")}},
	}
	setExecut(fexec)
	defer setExecut(nil)
	app := testing.NewFakeApp("starbreaker", "python", 1)
	container := container{Id: "c-036", AppName: "starbreaker", Type: "python", Ip: "10.10.10.1"}
	err := s.conn.Collection(s.collName).Insert(container)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Collection(s.collName).Remove(bson.M{"_id": container.Id})
	var stdout, stderr bytes.Buffer
	var p dockerProvisioner
	err = p.ExecuteCommand(&stdout, &stderr, app, "ls", "-ar")
	c.Assert(err, gocheck.IsNil)
	c.Assert(stderr.Bytes(), gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, ". ..")
	args := []string{
		"10.10.10.1", "-l", s.sshUser,
		"-o", "StrictHostKeyChecking no",
		"--", "ls", "-ar",
	}
	c.Assert(fexec.ExecutedCmd("ssh", args), gocheck.Equals, true)
}

func (s *S) TestProvisionerExecuteCommandMultipleContainers(c *gocheck.C) {
	fexec := &etesting.FakeExecutor{
		Output: map[string][][]byte{"*": {[]byte(". ..")}},
	}
	setExecut(fexec)
	defer setExecut(nil)
	app := testing.NewFakeApp("starbreaker", "python", 1)
	err := s.conn.Collection(s.collName).Insert(
		container{Id: "c-036", AppName: "starbreaker", Type: "python", Ip: "10.10.10.1"},
		container{Id: "c-037", AppName: "starbreaker", Type: "python", Ip: "10.10.10.2"},
	)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Collection(s.collName).RemoveAll(bson.M{"_id": bson.M{"$in": []string{"c-036", "c-037"}}})
	var stdout, stderr bytes.Buffer
	var p dockerProvisioner
	err = p.ExecuteCommand(&stdout, &stderr, app, "ls", "-ar")
	c.Assert(err, gocheck.IsNil)
	c.Assert(stderr.Bytes(), gocheck.IsNil)
	args1 := []string{
		"10.10.10.1", "-l", s.sshUser,
		"-o", "StrictHostKeyChecking no",
		"--", "ls", "-ar",
	}
	args2 := []string{
		"10.10.10.1", "-l", s.sshUser,
		"-o", "StrictHostKeyChecking no",
		"--", "ls", "-ar",
	}
	c.Assert(fexec.ExecutedCmd("ssh", args1), gocheck.Equals, true)
	c.Assert(fexec.ExecutedCmd("ssh", args2), gocheck.Equals, true)
}

func (s *S) TestProvisionerExecuteCommandFailure(c *gocheck.C) {
	fexec := &etesting.ErrorExecutor{
		FakeExecutor: etesting.FakeExecutor{
			Output: map[string][][]byte{"*": {[]byte("permission denied")}},
		},
	}
	setExecut(fexec)
	defer setExecut(nil)
	app := testing.NewFakeApp("starbreaker", "python", 1)
	container := container{Id: "c-036", AppName: "starbreaker", Type: "python", Ip: "10.10.10.1"}
	err := s.conn.Collection(s.collName).Insert(container)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Collection(s.collName).Remove(bson.M{"_id": container.Id})
	var stdout, stderr bytes.Buffer
	var p dockerProvisioner
	err = p.ExecuteCommand(&stdout, &stderr, app, "ls", "-ar")
	c.Assert(err, gocheck.NotNil)
	c.Assert(stdout.Bytes(), gocheck.IsNil)
	c.Assert(stderr.String(), gocheck.Equals, "permission denied")
}

func (s *S) TestProvisionerExecuteCommandNoContainers(c *gocheck.C) {
	var p dockerProvisioner
	app := testing.NewFakeApp("almah", "static", 2)
	var buf bytes.Buffer
	err := p.ExecuteCommand(&buf, &buf, app, "ls", "-lh")
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "No containers for this app")
}

func (s *S) TestCollectStatus(c *gocheck.C) {
	rtesting.FakeRouter.AddBackend("ashamed")
	defer rtesting.FakeRouter.RemoveBackend("ashamed")
	rtesting.FakeRouter.AddBackend("make-up")
	defer rtesting.FakeRouter.RemoveBackend("make-up")
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	c.Assert(err, gocheck.IsNil)
	defer listener.Close()
	listenPort := strings.Split(listener.Addr().String(), ":")[1]
	err = collection().Insert(
		container{
			Id: "9930c24f1c5f", AppName: "ashamed", Type: "python",
			Port: listenPort, Status: "running", Ip: "127.0.0.1",
			HostPort: "90293",
		},
		container{
			Id: "9930c24f1c4f", AppName: "make-up", Type: "python",
			Port: "8889", Status: "running", Ip: "127.0.0.4",
			HostPort: "90295",
		},
		container{Id: "9930c24f1c6f", AppName: "make-up", Type: "python", Port: "9090", Status: "error"},
		container{Id: "9930c24f1c7f", AppName: "make-up", Type: "python", Port: "9090", Status: "created"},
	)
	rtesting.FakeRouter.AddRoute("ashamed", "http://"+s.hostAddr+":90293")
	rtesting.FakeRouter.AddRoute("make-up", "http://"+s.hostAddr+":90295")
	c.Assert(err, gocheck.IsNil)
	defer collection().RemoveAll(bson.M{"appname": "make-up"})
	psOutput := `9930c24f1c5f
9930c24f1c4f
9930c24f1c3f
9930c24f1c6f
9930c24f1c7f
`
	c1Output := fmt.Sprintf(`{
	"NetworkSettings": {
		"IpAddress": "127.0.0.1",
		"IpPrefixLen": 8,
		"Gateway": "10.65.41.1",
		"PortMapping": {
			"%s": "90293"
		}
	}
}`, listenPort)
	c2Output := `{
	"NetworkSettings": {
		"IpAddress": "127.0.0.1",
		"IpPrefixLen": 8,
		"Gateway": "10.65.41.1",
		"PortMapping": {
			"8889": "90294"
		}
	}
}`
	expected := []provision.Unit{
		{
			Name:    "9930c24f1c5f",
			AppName: "ashamed",
			Type:    "python",
			Machine: 0,
			Ip:      "127.0.0.1",
			Status:  provision.StatusStarted,
		},
		{
			Name:    "9930c24f1c4f",
			AppName: "make-up",
			Type:    "python",
			Machine: 0,
			Ip:      "127.0.0.1",
			Status:  provision.StatusInstalling,
		},
		{
			Name:    "9930c24f1c6f",
			AppName: "make-up",
			Type:    "python",
			Status:  provision.StatusError,
		},
	}
	output := map[string][][]byte{
		"ps -q":                {[]byte(psOutput)},
		"inspect 9930c24f1c5f": {[]byte(c1Output)},
		"inspect 9930c24f1c4f": {[]byte(c2Output)},
	}
	fexec := &etesting.FakeExecutor{Output: output}
	setExecut(fexec)
	defer setExecut(nil)
	var p dockerProvisioner
	units, err := p.CollectStatus()
	c.Assert(err, gocheck.IsNil)
	sortUnits(units)
	sortUnits(expected)
	c.Assert(units, gocheck.DeepEquals, expected)
	cont, err := getContainer("9930c24f1c4f")
	c.Assert(err, gocheck.IsNil)
	c.Assert(cont.Ip, gocheck.Equals, "127.0.0.1")
	c.Assert(cont.HostPort, gocheck.Equals, "90294")
	c.Assert(fexec.ExecutedCmd("ssh-keygen", []string{"-R", "127.0.0.4"}), gocheck.Equals, true)
	c.Assert(rtesting.FakeRouter.HasRoute("make-up", "http://"+s.hostAddr+":90295"), gocheck.Equals, false)
	c.Assert(rtesting.FakeRouter.HasRoute("make-up", "http://"+s.hostAddr+":90294"), gocheck.Equals, true)
}

func (s *S) TestProvisionCollectStatusEmpty(c *gocheck.C) {
	s.conn.Collection(s.collName).RemoveAll(nil)
	output := map[string][][]byte{"ps -q": {[]byte("")}}
	fexec := &etesting.FakeExecutor{Output: output}
	setExecut(fexec)
	defer setExecut(nil)
	var p dockerProvisioner
	units, err := p.CollectStatus()
	c.Assert(err, gocheck.IsNil)
	c.Assert(units, gocheck.HasLen, 0)
}

// There was a dead lock in the error handling. This test prevents regression.
func (s *S) TestProvisionCollectStatusMultipleErrors(c *gocheck.C) {
	s.conn.Collection(s.collName).Insert(
		container{Id: "abcdef-800"},
		container{Id: "abcdef-801"},
		container{Id: "abcdef-802"},
		container{Id: "abcdef-802"},
	)
	defer s.conn.Collection(s.collName).RemoveAll(nil)
	var p dockerProvisioner
	units, err := p.CollectStatus()
	c.Assert(units, gocheck.HasLen, 0)
	c.Assert(err, gocheck.NotNil)
}

func (s *S) TestProvisionCollection(c *gocheck.C) {
	collection := collection()
	c.Assert(collection.Name, gocheck.Equals, s.collName)
}

func (s *S) TestProvisionSetCName(c *gocheck.C) {
	var p dockerProvisioner
	app := testing.NewFakeApp("myapp", "python", 1)
	rtesting.FakeRouter.AddBackend("myapp")
	rtesting.FakeRouter.AddRoute("myapp", "127.0.0.1")
	cname := "mycname.com"
	err := p.SetCName(app, cname)
	c.Assert(err, gocheck.IsNil)
	c.Assert(rtesting.FakeRouter.HasBackend(cname), gocheck.Equals, true)
	c.Assert(rtesting.FakeRouter.HasRoute(cname, "127.0.0.1"), gocheck.Equals, true)
}

func (s *S) TestProvisionUnsetCName(c *gocheck.C) {
	var p dockerProvisioner
	app := testing.NewFakeApp("myapp", "python", 1)
	rtesting.FakeRouter.AddBackend("myapp")
	rtesting.FakeRouter.AddRoute("myapp", "127.0.0.1")
	cname := "mycname.com"
	err := p.SetCName(app, cname)
	c.Assert(err, gocheck.IsNil)
	c.Assert(rtesting.FakeRouter.HasBackend(cname), gocheck.Equals, true)
	c.Assert(rtesting.FakeRouter.HasRoute(cname, "127.0.0.1"), gocheck.Equals, true)
	err = p.UnsetCName(app, cname)
	c.Assert(rtesting.FakeRouter.HasBackend(cname), gocheck.Equals, false)
	c.Assert(rtesting.FakeRouter.HasRoute(cname, "127.0.0.1"), gocheck.Equals, false)
}

func (s *S) TestProvisionerIsCNameManager(c *gocheck.C) {
	var p interface{}
	p = &dockerProvisioner{}
	_, ok := p.(provision.CNameManager)
	c.Assert(ok, gocheck.Equals, true)
}
