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
	rtesting "github.com/tsuru/tsuru/router/testing"
	"github.com/tsuru/tsuru/safe"
	"github.com/tsuru/tsuru/testing"
	tsrTesting "github.com/tsuru/tsuru/testing"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"io/ioutil"
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

func (s *S) TestProvisionerRestart(c *gocheck.C) {
	var p dockerProvisioner
	app := testing.NewFakeApp("almah", "static", 1)
	container, err := s.newContainer(&newContainerOpts{AppName: app.GetName()})
	c.Assert(err, gocheck.IsNil)
	defer s.removeTestContainer(container)
	err = p.Start(app)
	c.Assert(err, gocheck.IsNil)
	dockerContainer, err := dCluster.InspectContainer(container.ID)
	c.Assert(err, gocheck.IsNil)
	c.Assert(dockerContainer.State.Running, gocheck.Equals, true)
	container.IP = ""
	container.HostPort = ""
	coll := collection()
	defer coll.Close()
	coll.Update(bson.M{"id": container.ID}, container)
	err = p.Restart(app)
	c.Assert(err, gocheck.IsNil)
	dockerContainer, err = dCluster.InspectContainer(container.ID)
	c.Assert(err, gocheck.IsNil)
	c.Assert(dockerContainer.State.Running, gocheck.Equals, true)
	container, err = getContainer(container.ID)
	c.Assert(err, gocheck.IsNil)
	expectedIP := dockerContainer.NetworkSettings.IPAddress
	expectedPort := dockerContainer.NetworkSettings.Ports["8888/tcp"][0].HostPort
	c.Assert(container.IP, gocheck.Equals, expectedIP)
	c.Assert(container.HostPort, gocheck.Equals, expectedPort)
	c.Assert(container.Status, gocheck.Equals, provision.StatusStarted.String())
	expectedSSHPort := dockerContainer.NetworkSettings.Ports["22/tcp"][0].HostPort
	c.Assert(container.SSHHostPort, gocheck.Equals, expectedSSHPort)
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
	setExecut(&etesting.FakeExecutor{})
	defer setExecut(nil)
	p := dockerProvisioner{}
	a := app.App{
		Name:     "otherapp",
		Platform: "python",
	}
	conn, err := db.Conn()
	defer conn.Close()
	err = conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer conn.Apps().Remove(bson.M{"name": a.Name})
	p.Provision(&a)
	defer p.Destroy(&a)
	w := safe.NewBuffer(make([]byte, 2048))
	var serviceBodies []string
	rollback := s.addServiceInstance(c, a.Name, func(w http.ResponseWriter, r *http.Request) {
		data, _ := ioutil.ReadAll(r.Body)
		serviceBodies = append(serviceBodies, string(data))
		w.WriteHeader(http.StatusOK)
	})
	defer rollback()
	err = app.Deploy(app.DeployOptions{
		App:          &a,
		Version:      "master",
		Commit:       "123",
		OutputStream: w,
	})
	c.Assert(err, gocheck.IsNil)
	units := a.Units()
	c.Assert(units, gocheck.HasLen, 1)
	c.Assert(serviceBodies, gocheck.HasLen, 1)
	c.Assert(serviceBodies[0], gocheck.Matches, ".*unit-host="+units[0].Ip)
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
	err = app.Deploy(app.DeployOptions{
		App:          &a,
		Version:      "master",
		Commit:       "123",
		OutputStream: &w,
	})

	c.Assert(err, gocheck.IsNil)
	defer p.Destroy(&a)
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

func (s *S) TestProvisionerAddUnitsWithErrorDoesntLeaveLostUnits(c *gocheck.C) {
	callCount := 0
	s.server.CustomHandler("/containers/create", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		s.server.DefaultHandler().ServeHTTP(w, r)
	}))
	defer s.server.CustomHandler("/containers/create", s.server.DefaultHandler())
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
	_, err = p.AddUnits(app, 3)
	c.Assert(err, gocheck.NotNil)
	count, err := coll.Find(bson.M{"appname": app.GetName()}).Count()
	c.Assert(err, gocheck.IsNil)
	c.Assert(count, gocheck.Equals, 1)
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

func (s *S) TestProvisionerRemoveUnits(c *gocheck.C) {
	err := newImage("tsuru/python", s.server.URL())
	c.Assert(err, gocheck.IsNil)
	container1, err := s.newContainer(&newContainerOpts{Status: "building"})
	c.Assert(err, gocheck.IsNil)
	defer rtesting.FakeRouter.RemoveBackend(container1.AppName)
	container2, err := s.newContainer(&newContainerOpts{Status: "building"})
	c.Assert(err, gocheck.IsNil)
	defer rtesting.FakeRouter.RemoveBackend(container2.AppName)
	container3, err := s.newContainer(&newContainerOpts{Status: "started"})
	c.Assert(err, gocheck.IsNil)
	defer s.removeTestContainer(container3)
	client, err := docker.NewClient(s.server.URL())
	c.Assert(err, gocheck.IsNil)
	err = client.StartContainer(container1.ID, nil)
	c.Assert(err, gocheck.IsNil)
	err = client.StartContainer(container2.ID, nil)
	c.Assert(err, gocheck.IsNil)
	app := testing.NewFakeApp(container1.AppName, "python", 0)
	var p dockerProvisioner
	err = p.RemoveUnits(app, 2)
	c.Assert(err, gocheck.IsNil)
	_, err = getContainer(container1.ID)
	c.Assert(err, gocheck.NotNil)
	_, err = getContainer(container2.ID)
	c.Assert(err, gocheck.NotNil)
}

func (s *S) TestProvisionerRemoveUnitsPriorityOrder(c *gocheck.C) {
	err := newImage("tsuru/python", s.server.URL())
	c.Assert(err, gocheck.IsNil)
	container, err := s.newContainer(nil)
	c.Assert(err, gocheck.IsNil)
	defer rtesting.FakeRouter.RemoveBackend(container.AppName)
	app := testing.NewFakeApp(container.AppName, "python", 0)
	var p dockerProvisioner
	_, err = p.AddUnits(app, 3)
	c.Assert(err, gocheck.IsNil)
	err = p.RemoveUnits(app, 1)
	c.Assert(err, gocheck.IsNil)
	_, err = getContainer(container.ID)
	c.Assert(err, gocheck.NotNil)
	c.Assert(p.Units(app), gocheck.HasLen, 3)
}

func (s *S) TestProvisionerRemoveUnitsNotFound(c *gocheck.C) {
	var p dockerProvisioner
	err := p.RemoveUnits(nil, 1)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "remove units: app should not be nil")
}

func (s *S) TestProvisionerRemoveUnitsZeroUnits(c *gocheck.C) {
	var p dockerProvisioner
	err := p.RemoveUnits(testing.NewFakeApp("something", "python", 0), 0)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "remove units: units must be at least 1")
}

func (s *S) TestProvisionerRemoveUnitsTooManyUnits(c *gocheck.C) {
	err := newImage("tsuru/python", s.server.URL())
	c.Assert(err, gocheck.IsNil)
	container, err := s.newContainer(nil)
	c.Assert(err, gocheck.IsNil)
	defer rtesting.FakeRouter.RemoveBackend(container.AppName)
	app := testing.NewFakeApp(container.AppName, "python", 0)
	var p dockerProvisioner
	_, err = p.AddUnits(app, 2)
	c.Assert(err, gocheck.IsNil)
	err = p.RemoveUnits(app, 3)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "remove units: cannot remove all units from app")
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
	err = p.RemoveUnit(provision.Unit{AppName: app.GetName(), Name: container.ID})
	c.Assert(err, gocheck.IsNil)
	_, err = getContainer(container.ID)
	c.Assert(err, gocheck.NotNil)
}

func (s *S) TestProvisionerRemoveUnitNotFound(c *gocheck.C) {
	var p dockerProvisioner
	err := p.RemoveUnit(provision.Unit{Name: "wat de reu"})
	c.Assert(err, gocheck.Equals, mgo.ErrNotFound)
}

func (s *S) TestProvisionerSetUnitStatus(c *gocheck.C) {
	err := newImage("tsuru/python", s.server.URL())
	c.Assert(err, gocheck.IsNil)
	opts := newContainerOpts{Status: provision.StatusStarted.String(), AppName: "someapp"}
	container, err := s.newContainer(&opts)
	c.Assert(err, gocheck.IsNil)
	defer s.removeTestContainer(container)
	var p dockerProvisioner
	err = p.SetUnitStatus(provision.Unit{Name: container.ID, AppName: container.AppName}, provision.StatusError)
	c.Assert(err, gocheck.IsNil)
	container, err = getContainerPartialId(container.ID)
	c.Assert(err, gocheck.IsNil)
	c.Assert(container.Status, gocheck.Equals, provision.StatusError.String())
}

func (s *S) TestProvisionerSetUnitStatusWrongApp(c *gocheck.C) {
	err := newImage("tsuru/python", s.server.URL())
	c.Assert(err, gocheck.IsNil)
	opts := newContainerOpts{Status: provision.StatusStarted.String(), AppName: "someapp"}
	container, err := s.newContainer(&opts)
	c.Assert(err, gocheck.IsNil)
	defer s.removeTestContainer(container)
	var p dockerProvisioner
	err = p.SetUnitStatus(provision.Unit{Name: container.ID, AppName: container.AppName + "a"}, provision.StatusError)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "wrong app name")
	container, err = getContainerPartialId(container.ID)
	c.Assert(err, gocheck.IsNil)
	c.Assert(container.Status, gocheck.Equals, provision.StatusStarted.String())
}

func (s *S) TestProvisionerSetUnitStatusUnitNotFound(c *gocheck.C) {
	var p dockerProvisioner
	err := p.SetUnitStatus(provision.Unit{Name: "mycontainer", AppName: "myapp"}, provision.StatusError)
	c.Assert(err, gocheck.Equals, mgo.ErrNotFound)
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
	info, _ := container.networkInfo()
	path := fmt.Sprintf("/container/%s/cmd", info.IP)
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
	info1, _ := container1.networkInfo()
	info2, _ := container2.networkInfo()
	path1 := fmt.Sprintf("/container/%s/cmd", info1.IP)
	path2 := fmt.Sprintf("/container/%s/cmd", info2.IP)
	c.Assert(handler.requests[0].URL.Path, gocheck.Equals, path1)
	c.Assert(handler.requests[1].URL.Path, gocheck.Equals, path2)
}

func (s *S) TestProvisionerExecuteCommandNoContainers(c *gocheck.C) {
	var p dockerProvisioner
	app := testing.NewFakeApp("almah", "static", 2)
	var buf bytes.Buffer
	err := p.ExecuteCommand(&buf, &buf, app, "ls", "-lh")
	c.Assert(err, gocheck.Equals, provision.ErrEmptyApp)
}

func (s *S) TestProvisionerExecuteCommandOnceNoContainers(c *gocheck.C) {
	var p dockerProvisioner
	app := testing.NewFakeApp("almah", "static", 2)
	var buf bytes.Buffer
	err := p.ExecuteCommandOnce(&buf, &buf, app, "ls", "-lh")
	c.Assert(err, gocheck.Equals, provision.ErrEmptyApp)
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
		&sshAgentCmd{},
	}
	c.Assert(p.Commands(), gocheck.DeepEquals, expected)
}

func (s *S) TestProvisionerIsCommandable(c *gocheck.C) {
	var _ cmd.Commandable = &dockerProvisioner{}
}

func (s *S) TestAdminCommands(c *gocheck.C) {
	expected := []cmd.Command{
		&moveContainerCmd{},
		&moveContainersCmd{},
		&rebalanceContainersCmd{},
		&addNodeToSchedulerCmd{},
		&removeNodeFromSchedulerCmd{},
		listNodesInTheSchedulerCmd{},
		addPoolToSchedulerCmd{},
		removePoolFromSchedulerCmd{},
		listPoolsInTheSchedulerCmd{},
		addTeamsToPoolCmd{},
		removeTeamsFromPoolCmd{},
		fixContainersCmd{},
		sshToContainerCmd{},
	}
	var p dockerProvisioner
	c.Assert(p.AdminCommands(), gocheck.DeepEquals, expected)
}

func (s *S) TestProvisionerIsAdminCommandable(c *gocheck.C) {
	var _ cmd.AdminCommandable = &dockerProvisioner{}
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
	container, err = getContainer(container.ID)
	c.Assert(err, gocheck.IsNil)
	expectedIP := dockerContainer.NetworkSettings.IPAddress
	expectedPort := dockerContainer.NetworkSettings.Ports["8888/tcp"][0].HostPort
	c.Assert(container.IP, gocheck.Equals, expectedIP)
	c.Assert(container.HostPort, gocheck.Equals, expectedPort)
	c.Assert(container.Status, gocheck.Equals, provision.StatusStarted.String())
	expectedSSHPort := dockerContainer.NetworkSettings.Ports["22/tcp"][0].HostPort
	c.Assert(container.SSHHostPort, gocheck.Equals, expectedSSHPort)
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
	server, err := dtesting.NewServer("127.0.0.1:0", nil, func(r *http.Request) {
		requests = append(requests, r)
	})
	c.Assert(err, gocheck.IsNil)
	defer server.Stop()
	config.Set("docker:registry", "localhost:3030")
	defer config.Unset("docker:registry")
	var storage cluster.MapStorage
	storage.StoreImage("localhost:3030/base", server.URL())
	cmutex.Lock()
	oldDockerCluster := dCluster
	dCluster, _ = cluster.New(nil, &storage,
		cluster.Node{Address: server.URL()})
	cmutex.Unlock()
	defer func() {
		cmutex.Lock()
		dCluster = oldDockerCluster
		cmutex.Unlock()
	}()
	args := make(map[string]string)
	args["dockerfile"] = "http://localhost/Dockerfile"
	p := dockerProvisioner{}
	err = p.PlatformAdd("test", args, bytes.NewBuffer(nil))
	c.Assert(err, gocheck.IsNil)
	c.Assert(requests, gocheck.HasLen, 2)
	queryString := requests[0].URL.Query()
	c.Assert(queryString.Get("t"), gocheck.Equals, assembleImageName("test"))
	c.Assert(queryString.Get("remote"), gocheck.Equals, "http://localhost/Dockerfile")
}

func (s *S) TestProvisionerPlatformAddWithoutArgs(c *gocheck.C) {
	p := dockerProvisioner{}
	err := p.PlatformAdd("test", nil, nil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "Dockerfile is required.")
}

func (s *S) TestProvisionerPlatformAddShouldValidateArgs(c *gocheck.C) {
	args := make(map[string]string)
	args["dockerfile"] = "not_a_url"
	p := dockerProvisioner{}
	err := p.PlatformAdd("test", args, bytes.NewBuffer(nil))
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "dockerfile parameter should be an url.")
}

func (s *S) TestProvisionerUnits(c *gocheck.C) {
	app := app.App{Name: "myapplication"}
	coll := collection()
	defer coll.Close()
	err := coll.Insert(
		container{
			ID:       "9930c24f1c4f",
			AppName:  app.Name,
			Type:     "python",
			Status:   provision.StatusBuilding.String(),
			IP:       "127.0.0.4",
			HostPort: "9025",
		},
	)
	c.Assert(err, gocheck.IsNil)
	defer coll.RemoveAll(bson.M{"appname": app.Name})
	p := dockerProvisioner{}
	units := p.Units(&app)
	expected := []provision.Unit{
		{Name: "9930c24f1c4f", AppName: "myapplication", Type: "python", Status: provision.StatusBuilding},
	}
	c.Assert(units, gocheck.DeepEquals, expected)
}

func (s *S) TestProvisionerUnitsAppDoesNotExist(c *gocheck.C) {
	app := app.App{Name: "myapplication"}
	p := dockerProvisioner{}
	units := p.Units(&app)
	expected := []provision.Unit{}
	c.Assert(units, gocheck.DeepEquals, expected)
}

func (s *S) TestProvisionerUnitsStatus(c *gocheck.C) {
	app := app.App{Name: "myapplication"}
	coll := collection()
	defer coll.Close()
	err := coll.Insert(
		container{
			ID:       "9930c24f1c4f",
			AppName:  app.Name,
			Type:     "python",
			Status:   provision.StatusBuilding.String(),
			IP:       "127.0.0.4",
			HostPort: "9025",
		},
		container{
			ID:       "9930c24f1c4j",
			AppName:  app.Name,
			Type:     "python",
			Status:   provision.StatusDown.String(),
			IP:       "127.0.0.4",
			HostPort: "9025",
		},
	)
	c.Assert(err, gocheck.IsNil)
	defer coll.RemoveAll(bson.M{"appname": app.Name})
	p := dockerProvisioner{}
	units := p.Units(&app)
	expected := []provision.Unit{
		{Name: "9930c24f1c4f", AppName: "myapplication", Type: "python", Status: provision.StatusBuilding},
		{Name: "9930c24f1c4j", AppName: "myapplication", Type: "python", Status: provision.StatusDown},
	}
	c.Assert(units, gocheck.DeepEquals, expected)
}

func (s *S) TestProvisionerUnitsIp(c *gocheck.C) {
	app := app.App{Name: "myapplication"}
	coll := collection()
	defer coll.Close()
	err := coll.Insert(
		container{
			ID:       "9930c24f1c4f",
			AppName:  app.Name,
			Type:     "python",
			Status:   provision.StatusBuilding.String(),
			IP:       "127.0.0.4",
			HostPort: "9025",
			HostAddr: "127.0.0.1",
		},
	)
	c.Assert(err, gocheck.IsNil)
	defer coll.RemoveAll(bson.M{"appname": app.Name})
	p := dockerProvisioner{}
	units := p.Units(&app)
	expected := []provision.Unit{
		{Name: "9930c24f1c4f", AppName: "myapplication", Type: "python", Ip: "127.0.0.1", Status: provision.StatusBuilding},
	}
	c.Assert(units, gocheck.DeepEquals, expected)
}
