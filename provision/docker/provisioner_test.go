// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"bytes"
	"fmt"
	"github.com/globocom/docker-cluster/cluster"
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
	"net/http"
	"net/http/httptest"
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
		ID:      id,
		AppName: app.GetName(),
		Type:    app.GetPlatform(),
		IP:      "10.10.10.10",
	}
	err := collection().Insert(cont)
	c.Assert(err, gocheck.IsNil)
	defer collection().RemoveId(cont.ID)
	err = p.Restart(app)
	c.Assert(err, gocheck.IsNil)
	args := []string{
		cont.IP, "-l", s.sshUser, "-o", "StrictHostKeyChecking no",
		"--", "/var/lib/tsuru/restart",
	}
	c.Assert(fexec.ExecutedCmd("ssh", args), gocheck.Equals, true)
}

func (s *S) TestDeployShouldCallDockerCreate(c *gocheck.C) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/containers/") {
			w.Write([]byte(inspectOut))
		}
		if strings.Contains(r.URL.Path, "/commit") {
			w.Write([]byte(`{"Id":"i-1"}`))
		}
	}))
	defer server.Close()
	oldCluster := dockerCluster
	var err error
	dockerCluster, err = cluster.New(
		cluster.Node{ID: "server", Address: server.URL},
	)
	c.Assert(err, gocheck.IsNil)
	defer func() {
		dockerCluster = oldCluster
	}()
	fexec := &etesting.FakeExecutor{
		Output: map[string][][]byte{
			"*": {[]byte("c-1"), []byte("c-2")},
		},
	}
	setExecut(fexec)
	defer setExecut(nil)
	p := dockerProvisioner{}
	app := testing.NewFakeApp("cribcaged", "python", 1)
	p.Provision(app)
	defer p.Destroy(app)
	w := &bytes.Buffer{}
	err = p.Deploy(app, "master", w)
	defer p.Destroy(app)
	defer s.conn.Collection(s.collName).RemoveId("c-1")
	defer s.conn.Collection(s.collName).RemoveId("c-2")
	c.Assert(err, gocheck.IsNil)
	runCmds, err := runCmds("i-1")
	args := runCmds[1:]
	c.Assert(fexec.ExecutedCmd("docker", args), gocheck.Equals, true)
}

// func (s *S) TestDeployShouldReplaceAllContainers(c *gocheck.C) {
// 	var p dockerProvisioner
// 	s.conn.Collection(s.collName).Insert(
// 		container{ID: "app/0", AppName: "app"},
// 		container{ID: "app/1", AppName: "app"},
// 	)
// 	defer s.conn.Collection(s.collName).RemoveAll(bson.M{"appname": "app"})
// 	app := testing.NewFakeApp("app", "python", 0)
// 	p.Provision(app)
// 	defer p.Destroy(app)
// 	app.AddUnit(&testing.FakeUnit{Name: "app/0"})
// 	app.AddUnit(&testing.FakeUnit{Name: "app/1"})
// 	out := `{
// 	"NetworkSettings": {
// 		"IpAddress": "10.10.10.%d",
// 		"IpPrefixLen": 8,
// 		"Gateway": "10.65.41.1",
// 		"PortMapping": {"8888": "37574"}
// 	}
// }`
// 	fexec := &etesting.FakeExecutor{
// 		Output: map[string][][]byte{
// 			"*":            {[]byte("c-59"), []byte("c-60"), []byte("c-61")},
// 			"commit c-59":  {[]byte("xxkkccdd")},
// 			"inspect c-59": {[]byte(fmt.Sprintf(out, 100))},
// 			"inspect c-60": {[]byte(fmt.Sprintf(out, 1))},
// 			"inspect c-61": {[]byte(fmt.Sprintf(out, 2))},
// 		},
// 	}
// 	setExecut(fexec)
// 	defer setExecut(nil)
// 	var w bytes.Buffer
// 	err := p.Deploy(app, "master", &w)
// 	c.Assert(err, gocheck.IsNil)
// 	defer p.Destroy(app)
// 	defer s.conn.Collection(s.collName).RemoveId("c-59")
// 	defer s.conn.Collection(s.collName).RemoveId("c-60")
// 	defer s.conn.Collection(s.collName).RemoveId("c-61")
// 	c.Assert(app.ProvisionUnits(), gocheck.HasLen, 0)
// 	commands := fexec.GetCommands("ssh")
// 	c.Assert(commands, gocheck.HasLen, 4)
// }

func (s *S) TestDeployRemoveContainersEvenWhenTheyreNotInTheAppsCollection(c *gocheck.C) {
	var called bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.Write([]byte(`{"Id":"someimageid"}`))
	}))
	defer server.Close()
	oldCluster := dockerCluster
	var err error
	dockerCluster, err = cluster.New(
		cluster.Node{ID: "server", Address: server.URL},
	)
	c.Assert(err, gocheck.IsNil)
	defer func() {
		dockerCluster = oldCluster
	}()
	var p dockerProvisioner
	s.conn.Collection(s.collName).Insert(
		container{ID: "app/0", AppName: "app"},
		container{ID: "app/1", AppName: "app"},
	)
	defer s.conn.Collection(s.collName).RemoveAll(bson.M{"appname": "app"})
	app := testing.NewFakeApp("app", "python", 0)
	p.Provision(app)
	defer p.Destroy(app)
	fexec := &etesting.FakeExecutor{
		Output: map[string][][]byte{
			"*": {[]byte("c-60"), []byte("c-61")},
		},
	}
	setExecut(fexec)
	defer setExecut(nil)
	var w bytes.Buffer
	err = p.Deploy(app, "master", &w)
	defer p.Destroy(app)
	n, err := s.conn.Collection(s.collName).Find(bson.M{"appname": "app"}).Count()
	c.Assert(err, gocheck.IsNil)
	c.Assert(n, gocheck.Equals, 2)
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
	err := p.Deploy(app, "master", &buf)
	c.Assert(err, gocheck.NotNil)
}

// func (s *S) TestDeployFailureSecondStep(c *gocheck.C) {
// 	var (
// 		p   dockerProvisioner
// 		buf bytes.Buffer
// 	)
// 	app := testing.NewFakeApp("app", "python", 0)
// 	app.AddUnit(&testing.FakeUnit{Name: "app/0"})
// 	p.Provision(app)
// 	defer p.Destroy(app)
// 	defer s.conn.Collection(s.collName).RemoveAll(bson.M{"appname": app.GetName()})
// 	output := `{
// 	"NetworkSettings": {
// 		"IpAddress": "10.10.10.%d",
// 		"IpPrefixLen": 8,
// 		"Gateway": "10.65.41.1",
// 		"PortMapping": {"8888": "37574"}
// 	}
// }`
// 	fexec := etesting.FailLaterExecutor{
// 		Succeeds: 3,
// 		FakeExecutor: etesting.FakeExecutor{
// 			Output: map[string][][]byte{
// 				/* "*":              {[]byte("c-0955")}, */
// 				"inspect c-0955": {[]byte(output)},
// 			},
// 		},
// 	}
// 	setExecut(&fexec)
// 	defer setExecut(nil)
// 	err := p.Deploy(app, "master", &buf)
// 	c.Assert(err, gocheck.NotNil)
// 	/* c.Assert(buf.String(), gocheck.Equals, "c-0955") */
// 	c.Assert(fexec.ExecutedCmd("docker", []string{"rm", "c-0955"}), gocheck.Equals, true)
// 	c.Assert(app.ProvisionUnits(), gocheck.HasLen, 1)
// }

func (s *S) TestProvisionerDestroy(c *gocheck.C) {
	err := s.newImage()
	c.Assert(err, gocheck.IsNil)
	cont, err := s.newContainer()
	c.Assert(err, gocheck.IsNil)
	app := testing.NewFakeApp(cont.AppName, "python", 1)
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
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/containers/") {
			w.Write([]byte(inspectOut))
		}
		if strings.Contains(r.URL.Path, "/commit") {
			w.Write([]byte(`{"Id":"someimageid"}`))
		}
	}))
	defer server.Close()
	oldCluster := dockerCluster
	var err error
	dockerCluster, err = cluster.New(
		cluster.Node{ID: "server", Address: server.URL},
	)
	c.Assert(err, gocheck.IsNil)
	defer func() {
		dockerCluster = oldCluster
	}()
	idDeploy := "123"
	idStart := "456"
	app := testing.NewFakeApp("myapp", "python", 1)
	cmds, err := deployCmds(app, "master")
	c.Assert(err, gocheck.IsNil)
	deployCmds := strings.Join(cmds[1:], " ")
	runCmd, err := runCmds("someimageid")
	c.Assert(err, gocheck.IsNil)
	startCmds := strings.Join(runCmd[1:], " ")
	out := map[string][][]byte{
		deployCmds: {[]byte(idDeploy)},
		startCmds:  {[]byte(idStart)},
	}
	fexec := &etesting.FakeExecutor{Output: out}
	setExecut(fexec)
	defer setExecut(nil)
	var p dockerProvisioner
	p.Provision(app)
	defer p.Destroy(app)
	w := &bytes.Buffer{}
	err = p.Deploy(app, "master", w)
	c.Assert(err, gocheck.IsNil)
	defer p.Destroy(app)
	defer s.conn.Collection(s.collName).RemoveId(idDeploy)
	defer s.conn.Collection(s.collName).RemoveId(idStart)
	addr, err := p.Addr(app)
	c.Assert(err, gocheck.IsNil)
	r, err := getRouter()
	c.Assert(err, gocheck.IsNil)
	expected, err := r.Addr("myapp")
	c.Assert(err, gocheck.IsNil)
	c.Assert(addr, gocheck.Equals, expected)
}

func (s *S) TestProvisionerAddUnits(c *gocheck.C) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if strings.Contains(r.URL.Path, "/containers/") {
			out := `{
				"NetworkSettings": {
					"IpAddress": "10.10.10.%d",
					"IpPrefixLen": 8,
					"Gateway": "10.65.41.1",
					"PortMapping": {"8888": "37574"}
				}
			}`
			if strings.Contains(r.URL.Path, "300") {
				w.Write([]byte(fmt.Sprintf(out, 1)))
			}
			if strings.Contains(r.URL.Path, "301") {
				w.Write([]byte(fmt.Sprintf(out, 2)))
			}
			if strings.Contains(r.URL.Path, "302") {
				w.Write([]byte(fmt.Sprintf(out, 3)))
			}
		}
		if strings.Contains(r.URL.Path, "/commit") {
			w.Write([]byte(`{"Id":"someimageid"}`))
		}
	}))
	defer server.Close()
	oldCluster := dockerCluster
	var err error
	dockerCluster, err = cluster.New(
		cluster.Node{ID: "server", Address: server.URL},
	)
	c.Assert(err, gocheck.IsNil)
	defer func() {
		dockerCluster = oldCluster
	}()
	runCmds, err := runCmds("tsuru/python")
	c.Assert(err, gocheck.IsNil)
	runCmd := strings.Join(runCmds[1:], " ")
	fexec := etesting.FakeExecutor{
		Output: map[string][][]byte{
			runCmd: {[]byte("c-300"), []byte("c-301"), []byte("c-302")},
			"*":    {[]byte("ok sir")},
		},
	}
	setExecut(&fexec)
	defer setExecut(nil)
	var p dockerProvisioner
	app := testing.NewFakeApp("myapp", "python", 0)
	p.Provision(app)
	defer p.Destroy(app)
	s.conn.Collection(s.collName).Insert(container{ID: "c-89320", AppName: app.GetName(), Version: "a345fe"})
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
	count, err := s.conn.Collection(s.collName).Find(bson.M{"appname": app.GetName()}).Count()
	c.Assert(err, gocheck.IsNil)
	c.Assert(count, gocheck.Equals, 4)
	c.Assert(calls, gocheck.Equals, 6)
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
	s.conn.Collection(s.collName).Insert(container{ID: "c-89320", AppName: app.GetName()})
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
	var called int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called++
		if strings.Contains(r.URL.Path, "/containers/") {
			w.Write([]byte(inspectOut))
		}
		if strings.Contains(r.URL.Path, "/commit") {
			w.Write([]byte(`{"Id":"someimageid"}`))
		}
	}))
	defer server.Close()
	oldCluster := dockerCluster
	var err error
	dockerCluster, err = cluster.New(
		cluster.Node{ID: "server", Address: server.URL},
	)
	c.Assert(err, gocheck.IsNil)
	defer func() {
		dockerCluster = oldCluster
	}()
	app := testing.NewFakeApp("myapp", "python", 0)
	fexec := &etesting.FakeExecutor{
		Output: map[string][][]byte{
			"*": {[]byte("c-10")},
		},
	}
	setExecut(fexec)
	defer setExecut(nil)
	cmds, err := deployCmds(app, "version")
	c.Assert(err, gocheck.IsNil)
	container, err := newContainer(app, cmds)
	c.Assert(err, gocheck.IsNil)
	defer container.remove()
	var p dockerProvisioner
	err = p.RemoveUnit(app, container.ID)
	c.Assert(err, gocheck.IsNil)
	_, err = getContainer(container.ID)
	c.Assert(err, gocheck.NotNil)
}

func (s *S) TestProvisionerRemoveUnitNotFound(c *gocheck.C) {
	var p dockerProvisioner
	err := p.RemoveUnit(nil, "not-found")
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "not found")
}

func (s *S) TestProvisionerRemoveUnitNotInApp(c *gocheck.C) {
	var called bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if strings.Contains(r.URL.Path, "/containers/") {
			w.Write([]byte(inspectOut))
		}
	}))
	defer server.Close()
	oldCluster := dockerCluster
	var err error
	dockerCluster, err = cluster.New(
		cluster.Node{ID: "server", Address: server.URL},
	)
	c.Assert(err, gocheck.IsNil)
	defer func() {
		dockerCluster = oldCluster
	}()
	app := testing.NewFakeApp("myapp", "python", 0)
	fexec := &etesting.FakeExecutor{
		Output: map[string][][]byte{
			"*": {[]byte("c-10")},
		},
	}
	setExecut(fexec)
	defer setExecut(nil)
	cmds, err := deployCmds(app, "version")
	c.Assert(err, gocheck.IsNil)
	container, err := newContainer(app, cmds)
	c.Assert(err, gocheck.IsNil)
	defer container.remove()
	var p dockerProvisioner
	err = p.RemoveUnit(testing.NewFakeApp("hisapp", "python", 1), container.ID)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "Unit does not belong to this app")
	_, err = getContainer(container.ID)
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestProvisionerExecuteCommand(c *gocheck.C) {
	fexec := &etesting.FakeExecutor{
		Output: map[string][][]byte{"*": {[]byte(". ..")}},
	}
	setExecut(fexec)
	defer setExecut(nil)
	app := testing.NewFakeApp("starbreaker", "python", 1)
	container := container{ID: "c-036", AppName: "starbreaker", Type: "python", IP: "10.10.10.1"}
	err := s.conn.Collection(s.collName).Insert(container)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Collection(s.collName).Remove(bson.M{"_id": container.ID})
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
		container{ID: "c-036", AppName: "starbreaker", Type: "python", IP: "10.10.10.1"},
		container{ID: "c-037", AppName: "starbreaker", Type: "python", IP: "10.10.10.2"},
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
	container := container{ID: "c-036", AppName: "starbreaker", Type: "python", IP: "10.10.10.1"}
	err := s.conn.Collection(s.collName).Insert(container)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Collection(s.collName).Remove(bson.M{"_id": container.ID})
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
	var calls int
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
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if strings.Contains(r.URL.Path, "/containers/") {
			if strings.Contains(r.URL.Path, "/containers/9930c24f1c4f") {
				w.Write([]byte(c2Output))
			}
			if strings.Contains(r.URL.Path, "/containers/9930c24f1c5f") {
				w.Write([]byte(c1Output))
			}
		}
		if strings.Contains(r.URL.Path, "/commit") {
			w.Write([]byte(`{"Id":"i-1"}`))
		}
	}))
	defer server.Close()
	oldCluster := dockerCluster
	dockerCluster, err = cluster.New(
		cluster.Node{ID: "server", Address: server.URL},
	)
	c.Assert(err, gocheck.IsNil)
	defer func() {
		dockerCluster = oldCluster
	}()
	err = collection().Insert(
		container{
			ID: "9930c24f1c5f", AppName: "ashamed", Type: "python",
			Port: listenPort, Status: "running", IP: "127.0.0.1",
			HostPort: "90293",
		},
		container{
			ID: "9930c24f1c4f", AppName: "make-up", Type: "python",
			Port: "8889", Status: "running", IP: "127.0.0.4",
			HostPort: "90295",
		},
		container{ID: "9930c24f1c6f", AppName: "make-up", Type: "python", Port: "9090", Status: "error"},
		container{ID: "9930c24f1c7f", AppName: "make-up", Type: "python", Port: "9090", Status: "created"},
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
		"ps -q": {[]byte(psOutput)},
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
	c.Assert(cont.IP, gocheck.Equals, "127.0.0.1")
	c.Assert(cont.HostPort, gocheck.Equals, "90294")
	c.Assert(fexec.ExecutedCmd("ssh-keygen", []string{"-R", "127.0.0.4"}), gocheck.Equals, true)
	c.Assert(rtesting.FakeRouter.HasRoute("make-up", "http://"+s.hostAddr+":90295"), gocheck.Equals, false)
	c.Assert(rtesting.FakeRouter.HasRoute("make-up", "http://"+s.hostAddr+":90294"), gocheck.Equals, true)
	c.Assert(calls, gocheck.Equals, 4)
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
