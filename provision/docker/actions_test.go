// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	dockerClient "github.com/fsouza/go-dockerclient"
	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/provision"
	rtesting "github.com/tsuru/tsuru/router/testing"
	"github.com/tsuru/tsuru/testing"
	"labix.org/v2/mgo/bson"
	"launchpad.net/gocheck"
	"time"
)

func (s *S) TestInsertEmptyContainerInDBName(c *gocheck.C) {
	c.Assert(insertEmptyContainerInDB.Name, gocheck.Equals, "insert-empty-container")
}

func (s *S) TestInsertEmptyContainerInDBForward(c *gocheck.C) {
	app := testing.NewFakeApp("myapp", "python", 1)
	context := action.FWContext{Params: []interface{}{app, "image-id"}}
	r, err := insertEmptyContainerInDB.Forward(context)
	c.Assert(err, gocheck.IsNil)
	cont := r.(container)
	c.Assert(cont, gocheck.FitsTypeOf, container{})
	c.Assert(cont.AppName, gocheck.Equals, app.GetName())
	c.Assert(cont.Type, gocheck.Equals, app.GetPlatform())
	c.Assert(cont.Name, gocheck.Not(gocheck.Equals), "")
	c.Assert(cont.Name, gocheck.HasLen, 20)
	c.Assert(cont.Status, gocheck.Equals, "created")
	c.Assert(cont.Image, gocheck.Equals, "image-id")
	coll := collection()
	defer coll.Close()
	defer coll.Remove(bson.M{"name": cont.Name})
	var retrieved container
	err = coll.Find(bson.M{"name": cont.Name}).One(&retrieved)
	c.Assert(err, gocheck.IsNil)
	c.Assert(retrieved.Name, gocheck.Equals, cont.Name)
}

func (s *S) TestInsertEmptyContainerInDBBackward(c *gocheck.C) {
	cont := container{Name: "myName"}
	coll := collection()
	defer coll.Close()
	err := coll.Insert(&cont)
	c.Assert(err, gocheck.IsNil)
	context := action.BWContext{FWResult: cont}
	insertEmptyContainerInDB.Backward(context)
	err = coll.Find(bson.M{"name": cont.Name}).One(&cont)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "not found")
}

func (s *S) TestUpdateContainerInDBName(c *gocheck.C) {
	c.Assert(updateContainerInDB.Name, gocheck.Equals, "update-database-container")
}

func (s *S) TestUpdateContainerInDBForward(c *gocheck.C) {
	cont := container{Name: "myName"}
	coll := collection()
	defer coll.Close()
	err := coll.Insert(cont)
	c.Assert(err, gocheck.IsNil)
	cont.ID = "myID"
	context := action.FWContext{Previous: cont}
	r, err := updateContainerInDB.Forward(context)
	c.Assert(r, gocheck.FitsTypeOf, container{})
	retrieved, err := getContainer(cont.ID)
	c.Assert(err, gocheck.IsNil)
	c.Assert(retrieved.ID, gocheck.Equals, cont.ID)
}

func (s *S) TestCreateContainerName(c *gocheck.C) {
	c.Assert(createContainer.Name, gocheck.Equals, "create-container")
}

func (s *S) TestCreateContainerForward(c *gocheck.C) {
	err := newImage("tsuru/python", s.server.URL())
	c.Assert(err, gocheck.IsNil)
	client, err := dockerClient.NewClient(s.server.URL())
	c.Assert(err, gocheck.IsNil)
	images, err := client.ListImages(true)
	c.Assert(err, gocheck.IsNil)
	cmds := []string{"ps", "-ef"}
	app := testing.NewFakeApp("myapp", "python", 1)
	cont := container{Name: "myName", AppName: app.GetName(), Type: app.GetPlatform(), Status: "created"}
	context := action.FWContext{Previous: cont, Params: []interface{}{app, images[0].ID, cmds}}
	r, err := createContainer.Forward(context)
	c.Assert(err, gocheck.IsNil)
	cont = r.(container)
	defer cont.remove()
	c.Assert(cont, gocheck.FitsTypeOf, container{})
	c.Assert(cont.ID, gocheck.Not(gocheck.Equals), "")
	c.Assert(cont.HostAddr, gocheck.Equals, "127.0.0.1")
	dcli, err := dockerClient.NewClient(s.server.URL())
	c.Assert(err, gocheck.IsNil)
	cc, err := dcli.InspectContainer(cont.ID)
	c.Assert(err, gocheck.IsNil)
	c.Assert(cc.State.Running, gocheck.Equals, false)
}

func (s *S) TestCreateContainerBackward(c *gocheck.C) {
	dcli, err := dockerClient.NewClient(s.server.URL())
	c.Assert(err, gocheck.IsNil)
	err = newImage("tsuru/python", s.server.URL())
	c.Assert(err, gocheck.IsNil)
	defer dcli.RemoveImage("tsuru/python")
	conta, err := s.newContainer(nil)
	c.Assert(err, gocheck.IsNil)
	defer s.removeTestContainer(conta)
	cont := *conta
	context := action.BWContext{FWResult: cont}
	createContainer.Backward(context)
	_, err = dcli.InspectContainer(cont.ID)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err, gocheck.FitsTypeOf, &dockerClient.NoSuchContainer{})
}

func (s *S) TestAddRouteName(c *gocheck.C) {
	c.Assert(addRoute.Name, gocheck.Equals, "add-route")
}

func (s *S) TestAddRouteForward(c *gocheck.C) {
	app := testing.NewFakeApp("myapp", "python", 1)
	rtesting.FakeRouter.AddBackend(app.GetName())
	defer rtesting.FakeRouter.RemoveBackend(app.GetName())
	cont := container{ID: "ble", AppName: app.GetName()}
	defer cont.remove()
	context := action.FWContext{Previous: cont}
	r, err := addRoute.Forward(context)
	c.Assert(err, gocheck.IsNil)
	cont = r.(container)
	hasRoute := rtesting.FakeRouter.HasRoute(app.GetName(), cont.getAddress())
	c.Assert(hasRoute, gocheck.Equals, true)
	c.Assert(cont, gocheck.FitsTypeOf, container{})
}

func (s *S) TestSetNetworkInfoName(c *gocheck.C) {
	c.Assert(setNetworkInfo.Name, gocheck.Equals, "set-network-info")
}

func (s *S) TestSetNetworkInfoForward(c *gocheck.C) {
	err := newImage("tsuru/python", s.server.URL())
	c.Assert(err, gocheck.IsNil)
	conta, err := s.newContainer(nil)
	c.Assert(err, gocheck.IsNil)
	defer s.removeTestContainer(conta)
	cont := *conta
	context := action.FWContext{Previous: cont}
	r, err := setNetworkInfo.Forward(context)
	c.Assert(err, gocheck.IsNil)
	cont = r.(container)
	c.Assert(cont, gocheck.FitsTypeOf, container{})
	c.Assert(cont.IP, gocheck.Not(gocheck.Equals), "")
	c.Assert(cont.HostPort, gocheck.Not(gocheck.Equals), "")
}

func (s *S) TestSetImage(c *gocheck.C) {
	err := newImage("tsuru/python", s.server.URL())
	c.Assert(err, gocheck.IsNil)
	conta, err := s.newContainer(nil)
	c.Assert(err, gocheck.IsNil)
	defer s.removeTestContainer(conta)
	cont := *conta
	context := action.FWContext{Previous: cont}
	r, err := setNetworkInfo.Forward(context)
	c.Assert(err, gocheck.IsNil)
	cont = r.(container)
	c.Assert(cont, gocheck.FitsTypeOf, container{})
	c.Assert(cont.HostPort, gocheck.Not(gocheck.Equals), "")
}

func (s *S) TestStartContainerForward(c *gocheck.C) {
	err := newImage("tsuru/python", s.server.URL())
	c.Assert(err, gocheck.IsNil)
	conta, err := s.newContainer(nil)
	c.Assert(err, gocheck.IsNil)
	defer s.removeTestContainer(conta)
	cont := *conta
	context := action.FWContext{Previous: cont}
	r, err := startContainer.Forward(context)
	c.Assert(err, gocheck.IsNil)
	cont = r.(container)
	c.Assert(cont, gocheck.FitsTypeOf, container{})
}

func (s *S) TestStartContainerBackward(c *gocheck.C) {
	dcli, err := dockerClient.NewClient(s.server.URL())
	c.Assert(err, gocheck.IsNil)
	err = newImage("tsuru/python", s.server.URL())
	c.Assert(err, gocheck.IsNil)
	defer dcli.RemoveImage("tsuru/python")
	conta, err := s.newContainer(nil)
	c.Assert(err, gocheck.IsNil)
	defer s.removeTestContainer(conta)
	cont := *conta
	err = dcli.StartContainer(cont.ID, nil)
	c.Assert(err, gocheck.IsNil)
	context := action.BWContext{FWResult: cont}
	startContainer.Backward(context)
	cc, err := dcli.InspectContainer(cont.ID)
	c.Assert(err, gocheck.IsNil)
	c.Assert(cc.State.Running, gocheck.Equals, false)
}

func (s *S) TestInjectEnvironsName(c *gocheck.C) {
	c.Assert(injectEnvirons.Name, gocheck.Equals, "inject-environs")
}

func (s *S) TestInjectEnvironsForward(c *gocheck.C) {
	app := testing.NewFakeApp("myapp", "python", 1)
	context := action.FWContext{Params: []interface{}{app}}
	_, err := injectEnvirons.Forward(context)
	c.Assert(err, gocheck.IsNil)
	time.Sleep(6e9)
	c.Assert(app.GetCommands(), gocheck.DeepEquals, []string{"serialize", "restart"})
}

func (s *S) TestInjectEnvironsParams(c *gocheck.C) {
	ctx := action.FWContext{Params: []interface{}{""}}
	_, err := injectEnvirons.Forward(ctx)
	c.Assert(err.Error(), gocheck.Equals, "First parameter must be a provision.App.")
}

func (s *S) TestSaveUnitsName(c *gocheck.C) {
	c.Assert(saveUnits.Name, gocheck.Equals, "save-units")
}

func (s *S) TestSaveUnitsForward(c *gocheck.C) {
	a := app.App{
		Name:     "otherapp",
		Platform: "zend",
	}
	conn, err := db.Conn()
	c.Assert(err, gocheck.IsNil)
	defer conn.Close()
	err = conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer conn.Apps().Remove(bson.M{"name": a.Name})
	container := container{
		ID:       "id",
		Type:     "python",
		HostAddr: "",
		AppName:  a.Name,
	}
	coll := collection()
	c.Assert(err, gocheck.IsNil)
	coll.Insert(&container)
	context := action.FWContext{Params: []interface{}{&a}}
	_, err = saveUnits.Forward(context)
	c.Assert(err, gocheck.IsNil)
	app, err := app.GetByName(a.Name)
	c.Assert(err, gocheck.IsNil)
	c.Assert(app.Units[0].Name, gocheck.Equals, "id")
}

func (s *S) TestSaveUnitsForwardShouldMaintainData(c *gocheck.C) {
	a := app.App{
		Name:     "otherapp",
		Platform: "zend",
		Deploys:  10,
	}
	conn, err := db.Conn()
	c.Assert(err, gocheck.IsNil)
	defer conn.Close()
	err = conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	a.Deploys = 0
	defer conn.Apps().Remove(bson.M{"name": a.Name})
	container := container{
		ID:       "id",
		Type:     "python",
		HostAddr: "",
		AppName:  a.Name,
	}
	coll := collection()
	c.Assert(err, gocheck.IsNil)
	coll.Insert(&container)
	context := action.FWContext{Params: []interface{}{&a}}
	_, err = saveUnits.Forward(context)
	c.Assert(err, gocheck.IsNil)
	app, err := app.GetByName(a.Name)
	c.Assert(err, gocheck.IsNil)
	c.Assert(app.Units[0].Name, gocheck.Equals, "id")
	c.Assert(int(app.Deploys), gocheck.Equals, 10)
}

func (s *S) TestSaveUnitsParams(c *gocheck.C) {
	context := action.FWContext{Params: []interface{}{""}}
	_, err := saveUnits.Forward(context)
	c.Assert(err.Error(), gocheck.Equals, "First parameter must be a *app.App.")
}

func (s *S) TestbindServiceName(c *gocheck.C) {
	c.Assert(bindService.Name, gocheck.Equals, "bind-service")
}

func (s *S) TestbindServiceForward(c *gocheck.C) {
	a := testing.NewFakeApp("cribcaged", "python", 1)
	context := action.FWContext{Params: []interface{}{a}}
	_, err := bindService.Forward(context)
	c.Assert(err, gocheck.IsNil)
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

func (s *S) TestbindServiceParams(c *gocheck.C) {
	context := action.FWContext{Params: []interface{}{""}}
	_, err := bindService.Forward(context)
	c.Assert(err.Error(), gocheck.Equals, "First parameter must be a provision.App.")
}

func (s *S) TestProvisionAddUnitToHostName(c *gocheck.C) {
	c.Assert(provisionAddUnitToHost.Name, gocheck.Equals, "provision-add-unit-to-host")
}

func (s *S) TestProvisionAddUnitToHostForward(c *gocheck.C) {
	cluster, nodes, err := s.startMultipleServersCluster()
	c.Assert(err, gocheck.IsNil)
	defer s.stopMultipleServersCluster(cluster, nodes)
	err = newImage("tsuru/python", s.server.URL())
	c.Assert(err, gocheck.IsNil)
	var p dockerProvisioner
	app := testing.NewFakeApp("myapp-2", "python", 0)
	defer p.Destroy(app)
	p.Provision(app)
	coll := collection()
	defer coll.Close()
	coll.Insert(container{ID: "container-id", AppName: app.GetName(), Version: "container-version", Image: "tsuru/python"})
	defer coll.RemoveAll(bson.M{"appname": app.GetName()})
	context := action.FWContext{Params: []interface{}{app, "localhost"}}
	result, err := provisionAddUnitToHost.Forward(context)
	c.Assert(err, gocheck.IsNil)
	unit := result.(provision.Unit)
	c.Assert(unit.Ip, gocheck.Equals, "localhost")
	count, err := coll.Find(bson.M{"appname": app.GetName()}).Count()
	c.Assert(err, gocheck.IsNil)
	c.Assert(count, gocheck.Equals, 2)
}

func (s *S) TestProvisionAddUnitToHostBackward(c *gocheck.C) {
	err := newImage("tsuru/python", s.server.URL())
	c.Assert(err, gocheck.IsNil)
	container, err := s.newContainer(nil)
	c.Assert(err, gocheck.IsNil)
	defer s.removeTestContainer(container)
	app := testing.NewFakeApp(container.AppName, "python", 0)
	unit := provision.Unit{
		Name:    container.ID,
		AppName: app.GetName(),
		Type:    app.GetPlatform(),
		Ip:      container.HostAddr,
		Status:  provision.StatusBuilding,
	}
	context := action.BWContext{Params: []interface{}{app, "server"}, FWResult: unit}
	provisionAddUnitToHost.Backward(context)
	_, err = getContainer(container.ID)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "not found")
}

func (s *S) TestProvisionRemoveOldUnitName(c *gocheck.C) {
	c.Assert(provisionRemoveOldUnit.Name, gocheck.Equals, "provision-remove-old-unit")
}

func (s *S) TestProvisionRemoveOldUnitForward(c *gocheck.C) {
	err := newImage("tsuru/python", s.server.URL())
	c.Assert(err, gocheck.IsNil)
	container, err := s.newContainer(nil)
	c.Assert(err, gocheck.IsNil)
	defer rtesting.FakeRouter.RemoveBackend(container.AppName)
	client, err := dockerClient.NewClient(s.server.URL())
	c.Assert(err, gocheck.IsNil)
	err = client.StartContainer(container.ID, nil)
	c.Assert(err, gocheck.IsNil)
	app := testing.NewFakeApp(container.AppName, "python", 0)
	unit := provision.Unit{
		Name:    container.ID,
		AppName: app.GetName(),
		Type:    app.GetPlatform(),
		Ip:      container.HostAddr,
		Status:  provision.StatusBuilding,
	}
	context := action.FWContext{Params: []interface{}{app, "", *container}, Previous: unit}
	result, err := provisionRemoveOldUnit.Forward(context)
	c.Assert(err, gocheck.IsNil)
	retUnit := result.(provision.Unit)
	c.Assert(retUnit, gocheck.DeepEquals, unit)
	_, err = getContainer(container.ID)
	c.Assert(err, gocheck.NotNil)
}
