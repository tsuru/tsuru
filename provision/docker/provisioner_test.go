// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"bytes"
	"fmt"
	"github.com/fsouza/go-dockerclient"
	dtesting "github.com/fsouza/go-dockerclient/testing"
	"github.com/tsuru/config"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/cmd"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/exec"
	etesting "github.com/tsuru/tsuru/exec/testing"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/queue"
	rtesting "github.com/tsuru/tsuru/router/testing"
	"github.com/tsuru/tsuru/testing"
	tsrTesting "github.com/tsuru/tsuru/testing"
	"labix.org/v2/mgo/bson"
	"launchpad.net/gocheck"
	"net"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strconv"
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
	var handler FakeSSHServer
	handler.output = "caad7bbd5411"
	server := httptest.NewServer(&handler)
	defer server.Close()
	host, port, _ := net.SplitHostPort(server.Listener.Addr().String())
	portNumber, _ := strconv.Atoi(port)
	config.Set("docker:ssh-agent-port", portNumber)
	defer config.Unset("docker:ssh-agent-port")
	var p dockerProvisioner
	app := testing.NewFakeApp("almah", "static", 1)
	newImage("tsuru/python", s.server.URL())
	cont, err := s.newContainer(&newContainerOpts{AppName: app.GetName()})
	c.Assert(err, gocheck.IsNil)
	defer s.removeTestContainer(cont)
	cont.HostAddr = host
	coll := collection()
	defer coll.Close()
	err = coll.Update(bson.M{"id": cont.ID}, cont)
	c.Assert(err, gocheck.IsNil)
	err = p.Restart(app)
	c.Assert(err, gocheck.IsNil)
	input := cmdInput{Cmd: "/var/lib/tsuru/restart"}
	body := handler.bodies[0]
	c.Assert(body, gocheck.DeepEquals, input)
	ip, _, _ := cont.networkInfo()
	path := fmt.Sprintf("/container/%s/cmd", ip)
	c.Assert(handler.requests[0].URL.Path, gocheck.DeepEquals, path)
}

func (s *S) stopContainers(n uint) {
	client, err := docker.NewClient(s.server.URL())
	if err != nil {
		return
	}
	for n > 0 {
		opts := docker.ListContainersOptions{All: false}
		containers, err := client.ListContainers(opts)
		if err != nil {
			return
		}
		if len(containers) > 0 {
			for _, cont := range containers {
				if cont.ID != "" {
					client.StopContainer(cont.ID, 1)
					n--
				}
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func (s *S) TestDeploy(c *gocheck.C) {
	h := &tsrTesting.TestHandler{}
	gandalfServer := tsrTesting.StartGandalfTestServer(h)
	defer gandalfServer.Close()
	go s.stopContainers(1)
	err := newImage("tsuru/python", s.server.URL())
	c.Assert(err, gocheck.IsNil)
	fexec := &etesting.FakeExecutor{}
	setExecut(fexec)
	defer setExecut(nil)
	p := dockerProvisioner{}
	app.Provisioner = &p
	a := app.App{
		Name:     "otherapp",
		Platform: "python",
		Units:    []app.Unit{{Name: "i-0800", State: "started"}},
	}
	conn, err := db.Conn()
	defer conn.Close()
	err = conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer conn.Apps().Remove(bson.M{"name": a.Name})
	p.Provision(&a)
	defer p.Destroy(&a)
	w := writer{b: make([]byte, 2048)}
	err = app.DeployApp(&a, "master", "123", &w)
	c.Assert(err, gocheck.IsNil)
	w.b = nil
	time.Sleep(6e9)
	q, err := getQueue()
	for _, u := range a.ProvisionedUnits() {
		message, err := q.Get(1e6)
		c.Assert(err, gocheck.IsNil)
		c.Assert(message.Action, gocheck.Equals, app.BindService)
		c.Assert(message.Args[0], gocheck.Equals, a.GetName())
		c.Assert(message.Args[1], gocheck.Equals, u.GetName())
	}
}

func getQueue() (queue.Q, error) {
	queueName := "tsuru-app"
	qfactory, err := queue.Factory()
	if err != nil {
		return nil, err
	}
	return qfactory.Get(queueName)
}

func (s *S) TestDeployEnqueuesBindService(c *gocheck.C) {
	h := &tsrTesting.TestHandler{}
	gandalfServer := tsrTesting.StartGandalfTestServer(h)
	defer gandalfServer.Close()
	go s.stopContainers(1)
	err := newImage("tsuru/python", s.server.URL())
	c.Assert(err, gocheck.IsNil)
	setExecut(&etesting.FakeExecutor{})
	defer setExecut(nil)
	p := dockerProvisioner{}
	app.Provisioner = &p
	a := app.App{
		Name:     "otherapp",
		Platform: "python",
		Units:    []app.Unit{{Name: "i-0800", State: "started"}},
	}
	conn, err := db.Conn()
	defer conn.Close()
	err = conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer conn.Apps().Remove(bson.M{"name": a.Name})
	p.Provision(&a)
	defer p.Destroy(&a)
	w := writer{b: make([]byte, 2048)}
	err = app.DeployApp(&a, "master", "123", &w)
	c.Assert(err, gocheck.IsNil)
	defer p.Destroy(&a)
	q, err := getQueue()
	c.Assert(err, gocheck.IsNil)
	for _, u := range a.ProvisionedUnits() {
		message, err := q.Get(1e6)
		c.Assert(err, gocheck.IsNil)
		c.Assert(message.Action, gocheck.Equals, app.BindService)
		c.Assert(message.Args[0], gocheck.Equals, a.GetName())
		c.Assert(message.Args[1], gocheck.Equals, u.GetName())
	}
}

type writer struct {
	b   []byte
	cur int
}

func (w *writer) Write(c []byte) (int, error) {
	copy(w.b[w.cur:], c)
	w.cur += len(c)
	return len(c), nil
}

func (s *S) TestDeployRemoveContainersEvenWhenTheyreNotInTheAppsCollection(c *gocheck.C) {
	h := &tsrTesting.TestHandler{}
	gandalfServer := tsrTesting.StartGandalfTestServer(h)
	defer gandalfServer.Close()
	go s.stopContainers(3)
	err := newImage("tsuru/python", s.server.URL())
	c.Assert(err, gocheck.IsNil)
	cont1, err := s.newContainer(nil)
	defer s.removeTestContainer(cont1)
	c.Assert(err, gocheck.IsNil)
	cont2, err := s.newContainer(nil)
	defer s.removeTestContainer(cont2)
	c.Assert(err, gocheck.IsNil)
	defer rtesting.FakeRouter.RemoveBackend(cont1.AppName)
	var p dockerProvisioner
	a := app.App{
		Name:     "otherapp",
		Platform: "python",
		Units:    []app.Unit{{Name: "i-0800", State: "started"}},
	}
	conn, err := db.Conn()
	defer conn.Close()
	err = conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer conn.Apps().Remove(bson.M{"name": a.Name})
	p.Provision(&a)
	defer p.Destroy(&a)
	fexec := &etesting.FakeExecutor{}
	setExecut(fexec)
	defer setExecut(nil)
	var w bytes.Buffer
	err = app.DeployApp(&a, "master", "123", &w)
	c.Assert(err, gocheck.IsNil)
	time.Sleep(1e9)
	defer p.Destroy(&a)
	q, err := getQueue()
	c.Assert(err, gocheck.IsNil)
	for _, u := range a.ProvisionedUnits() {
		message, err := q.Get(1e6)
		c.Assert(err, gocheck.IsNil)
		c.Assert(message.Action, gocheck.Equals, app.BindService)
		c.Assert(message.Args[0], gocheck.Equals, a.GetName())
		c.Assert(message.Args[1], gocheck.Equals, u.GetName())
	}
	coll := collection()
	defer coll.Close()
	n, err := coll.Find(bson.M{"appname": cont1.AppName}).Count()
	c.Assert(err, gocheck.IsNil)
	c.Assert(n, gocheck.Equals, 2)
}

func (s *S) TestProvisionerDestroy(c *gocheck.C) {
	err := newImage("tsuru/python", s.server.URL())
	c.Assert(err, gocheck.IsNil)
	cont, err := s.newContainer(nil)
	c.Assert(err, gocheck.IsNil)
	app := testing.NewFakeApp(cont.AppName, "python", 1)
	var p dockerProvisioner
	p.Provision(app)
	c.Assert(p.Destroy(app), gocheck.IsNil)
	ok := make(chan bool, 1)
	go func() {
		coll := collection()
		defer coll.Close()
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
	err := newImage("tsuru/python", s.server.URL())
	c.Assert(err, gocheck.IsNil)
	cont, err := s.newContainer(nil)
	c.Assert(err, gocheck.IsNil)
	defer s.removeTestContainer(cont)
	app := testing.NewFakeApp(cont.AppName, "python", 1)
	var p dockerProvisioner
	addr, err := p.Addr(app)
	c.Assert(err, gocheck.IsNil)
	r, err := getRouter()
	c.Assert(err, gocheck.IsNil)
	expected, err := r.Addr(cont.AppName)
	c.Assert(err, gocheck.IsNil)
	c.Assert(addr, gocheck.Equals, expected)
}

func (s *S) TestProvisionerAddUnits(c *gocheck.C) {
	err := newImage("tsuru/python", s.server.URL())
	c.Assert(err, gocheck.IsNil)
	var p dockerProvisioner
	app := testing.NewFakeApp("myapp", "python", 0)
	p.Provision(app)
	defer p.Destroy(app)
	coll := collection()
	defer coll.Close()
	coll.Insert(container{ID: "c-89320", AppName: app.GetName(), Version: "a345fe", Image: "tsuru/python"})
	defer coll.RemoveId(bson.M{"id": "c-89320"})
	units, err := p.AddUnits(app, 3)
	c.Assert(err, gocheck.IsNil)
	defer coll.RemoveAll(bson.M{"appname": app.GetName()})
	c.Assert(units, gocheck.HasLen, 3)
	count, err := coll.Find(bson.M{"appname": app.GetName()}).Count()
	c.Assert(err, gocheck.IsNil)
	c.Assert(count, gocheck.Equals, 4)
}

func (s *S) TestProvisionerAddZeroUnits(c *gocheck.C) {
	var p dockerProvisioner
	units, err := p.AddUnits(nil, 0)
	c.Assert(units, gocheck.IsNil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "Cannot add 0 units")
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

func (s *S) TestProvisionerAddUnitsWithHost(c *gocheck.C) {
	cluster, err := s.startMultipleServersCluster()
	c.Assert(err, gocheck.IsNil)
	defer s.stopMultipleServersCluster(cluster)
	err = newImage("tsuru/python", s.server.URL())
	c.Assert(err, gocheck.IsNil)
	var p dockerProvisioner
	app := testing.NewFakeApp("myapp", "python", 0)
	p.Provision(app)
	defer p.Destroy(app)
	coll := collection()
	defer coll.Close()
	coll.Insert(container{ID: "xxxfoo", AppName: app.GetName(), Version: "123987", Image: "tsuru/python"})
	defer coll.RemoveId(bson.M{"id": "xxxfoo"})
	units, err := addUnitsWithHost(app, 1, "localhost")
	c.Assert(err, gocheck.IsNil)
	defer coll.RemoveAll(bson.M{"appname": app.GetName()})
	c.Assert(units, gocheck.HasLen, 1)
	c.Assert(units[0].Ip, gocheck.Equals, "localhost")
	count, err := coll.Find(bson.M{"appname": app.GetName()}).Count()
	c.Assert(err, gocheck.IsNil)
	c.Assert(count, gocheck.Equals, 2)
}

func (s *S) TestProvisionerRemoveUnit(c *gocheck.C) {
	err := newImage("tsuru/python", s.server.URL())
	c.Assert(err, gocheck.IsNil)
	container, err := s.newContainer(nil)
	c.Assert(err, gocheck.IsNil)
	defer rtesting.FakeRouter.RemoveBackend(container.AppName)
	client, err := docker.NewClient(s.server.URL())
	c.Assert(err, gocheck.IsNil)
	err = client.StartContainer(container.ID, nil)
	c.Assert(err, gocheck.IsNil)
	app := testing.NewFakeApp(container.AppName, "python", 0)
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
	err := newImage("tsuru/python", s.server.URL())
	c.Assert(err, gocheck.IsNil)
	container, err := s.newContainer(nil)
	c.Assert(err, gocheck.IsNil)
	defer s.removeTestContainer(container)
	var p dockerProvisioner
	err = p.RemoveUnit(testing.NewFakeApp("hisapp", "python", 1), container.ID)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "Unit does not belong to this app")
	_, err = getContainer(container.ID)
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestRemoveUnitInSameHostAsAnotherUnitShouldEnqueueAnotherBind(c *gocheck.C) {
	err := newImage("tsuru/python", s.server.URL())
	c.Assert(err, gocheck.IsNil)
	c1, err := s.newContainer(nil)
	c.Assert(err, gocheck.IsNil)
	c2, err := s.newContainer(nil)
	c.Assert(err, gocheck.IsNil)
	defer rtesting.FakeRouter.RemoveBackend(c1.AppName)
	client, err := docker.NewClient(s.server.URL())
	c.Assert(err, gocheck.IsNil)
	err = client.StartContainer(c1.ID, nil)
	c.Assert(err, gocheck.IsNil)
	a := testing.NewFakeApp(c1.AppName, "python", 0)
	var p dockerProvisioner
	err = p.RemoveUnit(a, c1.ID)
	c.Assert(err, gocheck.IsNil)
	_, err = getContainer(c1.ID)
	c.Assert(err, gocheck.NotNil)
	_, err = getContainer(c2.ID)
	c.Assert(err, gocheck.IsNil)
	q, err := getQueue()
	c.Assert(err, gocheck.IsNil)
	message, err := q.Get(1e6)
	c.Assert(err, gocheck.IsNil)
	expected := &queue.Message{Action: app.BindService, Args: []string{a.GetName(), c2.ID}}
	c.Assert(message, gocheck.DeepEquals, expected)
}

func (s *S) TestProvisionerExecuteCommand(c *gocheck.C) {
	var handler FakeSSHServer
	handler.output = ". .."
	server := httptest.NewServer(&handler)
	defer server.Close()
	host, port, _ := net.SplitHostPort(server.Listener.Addr().String())
	portNumber, _ := strconv.Atoi(port)
	config.Set("docker:ssh-agent-port", portNumber)
	defer config.Unset("docker:ssh-agent-port")
	app := testing.NewFakeApp("starbreaker", "python", 1)
	container, err := s.newContainer(&newContainerOpts{AppName: app.GetName()})
	c.Assert(err, gocheck.IsNil)
	defer s.removeTestContainer(container)
	container.HostAddr = host
	coll := collection()
	defer coll.Close()
	coll.Update(bson.M{"id": container.ID}, container)
	var stdout, stderr bytes.Buffer
	var p dockerProvisioner
	err = p.ExecuteCommand(&stdout, &stderr, app, "ls", "-ar")
	c.Assert(err, gocheck.IsNil)
	c.Assert(stderr.Bytes(), gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, ". ..")
	body := handler.bodies[0]
	input := cmdInput{Cmd: "ls", Args: []string{"-ar"}}
	c.Assert(body, gocheck.DeepEquals, input)
	ip, _, _ := container.networkInfo()
	path := fmt.Sprintf("/container/%s/cmd", ip)
	c.Assert(handler.requests[0].URL.Path, gocheck.DeepEquals, path)
}

func (s *S) TestProvisionerExecuteCommandMultipleContainers(c *gocheck.C) {
	var handler FakeSSHServer
	handler.output = ". .."
	server := httptest.NewServer(&handler)
	defer server.Close()
	host, port, _ := net.SplitHostPort(server.Listener.Addr().String())
	portNumber, _ := strconv.Atoi(port)
	config.Set("docker:ssh-agent-port", portNumber)
	defer config.Unset("docker:ssh-agent-port")
	app := testing.NewFakeApp("starbreaker", "python", 1)
	container1, err := s.newContainer(&newContainerOpts{AppName: app.GetName()})
	c.Assert(err, gocheck.IsNil)
	defer s.removeTestContainer(container1)
	container1.HostAddr = host
	coll := collection()
	defer coll.Close()
	coll.Update(bson.M{"id": container1.ID}, container1)
	container2, err := s.newContainer(&newContainerOpts{AppName: app.GetName()})
	c.Assert(err, gocheck.IsNil)
	defer s.removeTestContainer(container2)
	container2.HostAddr = host
	coll.Update(bson.M{"id": container2.ID}, container2)
	var stdout, stderr bytes.Buffer
	var p dockerProvisioner
	err = p.ExecuteCommand(&stdout, &stderr, app, "ls", "-ar")
	c.Assert(err, gocheck.IsNil)
	c.Assert(stderr.Bytes(), gocheck.IsNil)
	input := cmdInput{Cmd: "ls", Args: []string{"-ar"}}
	c.Assert(handler.bodies, gocheck.DeepEquals, []cmdInput{input, input})
	ip1, _, _ := container1.networkInfo()
	ip2, _, _ := container2.networkInfo()
	path1 := fmt.Sprintf("/container/%s/cmd", ip1)
	path2 := fmt.Sprintf("/container/%s/cmd", ip2)
	c.Assert(handler.requests[0].URL.Path, gocheck.Equals, path1)
	c.Assert(handler.requests[1].URL.Path, gocheck.Equals, path2)
}

func (s *S) TestProvisionerExecuteCommandNoContainers(c *gocheck.C) {
	var p dockerProvisioner
	app := testing.NewFakeApp("almah", "static", 2)
	var buf bytes.Buffer
	err := p.ExecuteCommand(&buf, &buf, app, "ls", "-lh")
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "No containers for this app")
}

func (s *S) TestProvisionCollection(c *gocheck.C) {
	collection := collection()
	defer collection.Close()
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
	var _ provision.CNameManager = &dockerProvisioner{}
}

func (s *S) TestCommands(c *gocheck.C) {
	var p dockerProvisioner
	expected := []cmd.Command{
		addNodeToSchedulerCmd{},
		removeNodeFromSchedulerCmd{},
		listNodesInTheSchedulerCmd{},
		&sshAgentCmd{},
	}
	c.Assert(p.Commands(), gocheck.DeepEquals, expected)
}

func (s *S) TestProvisionerIsCommandable(c *gocheck.C) {
	var _ cmd.Commandable = &dockerProvisioner{}
}

func (s *S) TestSwap(c *gocheck.C) {
	var p dockerProvisioner
	app1 := testing.NewFakeApp("app1", "python", 1)
	app2 := testing.NewFakeApp("app2", "python", 1)
	rtesting.FakeRouter.AddBackend(app1.GetName())
	rtesting.FakeRouter.AddRoute(app1.GetName(), "127.0.0.1")
	rtesting.FakeRouter.AddBackend(app2.GetName())
	rtesting.FakeRouter.AddRoute(app2.GetName(), "127.0.0.2")
	err := p.Swap(app1, app2)
	c.Assert(err, gocheck.IsNil)
	c.Assert(rtesting.FakeRouter.HasBackend(app1.GetName()), gocheck.Equals, true)
	c.Assert(rtesting.FakeRouter.HasBackend(app2.GetName()), gocheck.Equals, true)
	c.Assert(rtesting.FakeRouter.HasRoute(app2.GetName(), "127.0.0.1"), gocheck.Equals, true)
	c.Assert(rtesting.FakeRouter.HasRoute(app1.GetName(), "127.0.0.2"), gocheck.Equals, true)
}

func (s *S) TestExecuteCommandOnce(c *gocheck.C) {
	var handler FakeSSHServer
	handler.output = ". .."
	server := httptest.NewServer(&handler)
	defer server.Close()
	host, port, _ := net.SplitHostPort(server.Listener.Addr().String())
	portNumber, _ := strconv.Atoi(port)
	config.Set("docker:ssh-agent-port", portNumber)
	defer config.Unset("docker:ssh-agent-port")
	app := testing.NewFakeApp("almah", "static", 1)
	p := dockerProvisioner{}
	container, err := s.newContainer(&newContainerOpts{AppName: app.GetName()})
	c.Assert(err, gocheck.IsNil)
	defer s.removeTestContainer(container)
	container.HostAddr = host
	coll := collection()
	defer coll.Close()
	coll.Update(bson.M{"id": container.ID}, container)
	var stdout, stderr bytes.Buffer
	err = p.ExecuteCommandOnce(&stdout, &stderr, app, "ls", "-lh")
	c.Assert(err, gocheck.IsNil)
	c.Assert(stderr.Bytes(), gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, ". ..")
	body := handler.bodies[0]
	input := cmdInput{Cmd: "ls", Args: []string{"-lh"}}
	c.Assert(body, gocheck.DeepEquals, input)
}

func (s *S) TestExecuteCommandOnceWithoutContainers(c *gocheck.C) {
	app := testing.NewFakeApp("almah", "static", 2)
	p := dockerProvisioner{}
	var stdout, stderr bytes.Buffer
	err := p.ExecuteCommandOnce(&stdout, &stderr, app, "ls", "-lh")
	c.Assert(err, gocheck.Not(gocheck.IsNil))
}

func (s *S) TestDeployPipeline(c *gocheck.C) {
	p := dockerProvisioner{}
	c.Assert(p.DeployPipeline(), gocheck.NotNil)
}

func (s *S) TestProvisionerStart(c *gocheck.C) {
	var p dockerProvisioner
	app := testing.NewFakeApp("almah", "static", 1)
	container, err := s.newContainer(&newContainerOpts{AppName: app.GetName()})
	c.Assert(err, gocheck.IsNil)
	defer s.removeTestContainer(container)
	dcli, err := docker.NewClient(s.server.URL())
	c.Assert(err, gocheck.IsNil)
	dockerContainer, err := dcli.InspectContainer(container.ID)
	c.Assert(err, gocheck.IsNil)
	c.Assert(dockerContainer.State.Running, gocheck.Equals, false)
	err = p.Start(app)
	c.Assert(err, gocheck.IsNil)
	dockerContainer, err = dcli.InspectContainer(container.ID)
	c.Assert(err, gocheck.IsNil)
	c.Assert(dockerContainer.State.Running, gocheck.Equals, true)
}

func (s *S) TestProvisionerStop(c *gocheck.C) {
	dcli, _ := docker.NewClient(s.server.URL())
	app := testing.NewFakeApp("almah", "static", 2)
	p := dockerProvisioner{}
	container, err := s.newContainer(&newContainerOpts{AppName: app.GetName()})
	c.Assert(err, gocheck.IsNil)
	defer s.removeTestContainer(container)
	err = dcli.StartContainer(container.ID, nil)
	c.Assert(err, gocheck.IsNil)
	dockerContainer, err := dcli.InspectContainer(container.ID)
	c.Assert(err, gocheck.IsNil)
	c.Assert(dockerContainer.State.Running, gocheck.Equals, true)
	err = p.Stop(app)
	c.Assert(err, gocheck.IsNil)
	dockerContainer, err = dcli.InspectContainer(container.ID)
	c.Assert(err, gocheck.IsNil)
	c.Assert(dockerContainer.State.Running, gocheck.Equals, false)
}

func (s *S) TestProvisionerStopSkipAlreadyStoppedContainers(c *gocheck.C) {
	dcli, _ := docker.NewClient(s.server.URL())
	app := testing.NewFakeApp("almah", "static", 2)
	p := dockerProvisioner{}
	container, err := s.newContainer(&newContainerOpts{AppName: app.GetName()})
	c.Assert(err, gocheck.IsNil)
	defer s.removeTestContainer(container)
	err = dcli.StartContainer(container.ID, nil)
	c.Assert(err, gocheck.IsNil)
	dockerContainer, err := dcli.InspectContainer(container.ID)
	c.Assert(err, gocheck.IsNil)
	c.Assert(dockerContainer.State.Running, gocheck.Equals, true)
	container2, err := s.newContainer(&newContainerOpts{AppName: app.GetName()})
	c.Assert(err, gocheck.IsNil)
	defer s.removeTestContainer(container2)
	err = dcli.StartContainer(container2.ID, nil)
	c.Assert(err, gocheck.IsNil)
	err = dcli.StopContainer(container2.ID, 1)
	c.Assert(err, gocheck.IsNil)
	container2.setStatus(provision.StatusStopped.String())
	dockerContainer2, err := dcli.InspectContainer(container2.ID)
	c.Assert(err, gocheck.IsNil)
	c.Assert(dockerContainer2.State.Running, gocheck.Equals, false)
	err = p.Stop(app)
	c.Assert(err, gocheck.IsNil)
	dockerContainer, err = dcli.InspectContainer(container.ID)
	c.Assert(err, gocheck.IsNil)
	c.Assert(dockerContainer.State.Running, gocheck.Equals, false)
	dockerContainer2, err = dcli.InspectContainer(container2.ID)
	c.Assert(err, gocheck.IsNil)
	c.Assert(dockerContainer2.State.Running, gocheck.Equals, false)
}

func (s *S) TestProvisionerPlatformAdd(c *gocheck.C) {
	var requests []*http.Request
	server, err := dtesting.NewServer("127.0.0.1:0", func(r *http.Request) {
		requests = append(requests, r)
	})
	c.Assert(err, gocheck.IsNil)
	defer server.Stop()
	config.Set("docker:registry", "localhost:3030")
	defer config.Unset("docker:registry")
	var storage mapStorage
	storage.StoreImage("localhost:3030/base", "server0")
	cmutex.Lock()
	oldDockerCluster := dCluster
	dCluster, _ = cluster.New(nil, &storage,
		cluster.Node{ID: "server0", Address: server.URL()})
	cmutex.Unlock()
	defer func() {
		cmutex.Lock()
		dCluster = oldDockerCluster
		cmutex.Unlock()
	}()
	p := dockerProvisioner{}
	err = p.PlatformAdd("test", "http://localhost/Dockerfile")
	c.Assert(err, gocheck.IsNil)
	c.Assert(requests, gocheck.HasLen, 2)
	queryString := requests[0].URL.Query()
	c.Assert(queryString.Get("t"), gocheck.Equals, assembleImageName("test"))
	c.Assert(queryString.Get("remote"), gocheck.Equals, "http://localhost/Dockerfile")
}

func (s *S) TestProvisionerPlatformAddWithoutArgs(c *gocheck.C) {
	p := dockerProvisioner{}
	err := p.PlatformAdd("test", "")
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "Dockerfile is required.")
}
