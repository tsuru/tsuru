// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"bytes"
	"sort"

	"github.com/fsouza/go-dockerclient"
	"github.com/tsuru/tsuru/action"
	rtesting "github.com/tsuru/tsuru/router/testing"
	"github.com/tsuru/tsuru/testing"
	"gopkg.in/mgo.v2/bson"
	"launchpad.net/gocheck"
)

func (s *S) TestInsertEmptyContainerInDBName(c *gocheck.C) {
	c.Assert(insertEmptyContainerInDB.Name, gocheck.Equals, "insert-empty-container")
}

func (s *S) TestInsertEmptyContainerInDBForward(c *gocheck.C) {
	app := testing.NewFakeApp("myapp", "python", 1)
	args := runContainerActionsArgs{app: app, imageID: "image-id"}
	context := action.FWContext{Params: []interface{}{args}}
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
	client, err := docker.NewClient(s.server.URL())
	c.Assert(err, gocheck.IsNil)
	images, err := client.ListImages(docker.ListImagesOptions{All: true})
	c.Assert(err, gocheck.IsNil)
	cmds := []string{"ps", "-ef"}
	app := testing.NewFakeApp("myapp", "python", 1)
	cont := container{Name: "myName", AppName: app.GetName(), Type: app.GetPlatform(), Status: "created"}
	args := runContainerActionsArgs{app: app, imageID: images[0].ID, commands: cmds}
	context := action.FWContext{Previous: cont, Params: []interface{}{args}}
	r, err := createContainer.Forward(context)
	c.Assert(err, gocheck.IsNil)
	cont = r.(container)
	defer cont.remove()
	c.Assert(cont, gocheck.FitsTypeOf, container{})
	c.Assert(cont.ID, gocheck.Not(gocheck.Equals), "")
	c.Assert(cont.HostAddr, gocheck.Equals, "127.0.0.1")
	dcli, err := docker.NewClient(s.server.URL())
	c.Assert(err, gocheck.IsNil)
	cc, err := dcli.InspectContainer(cont.ID)
	c.Assert(err, gocheck.IsNil)
	c.Assert(cc.State.Running, gocheck.Equals, false)
}

func (s *S) TestCreateContainerBackward(c *gocheck.C) {
	dcli, err := docker.NewClient(s.server.URL())
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
	c.Assert(err, gocheck.FitsTypeOf, &docker.NoSuchContainer{})
}

func (s *S) TestAddNewRouteName(c *gocheck.C) {
	c.Assert(addNewRoutes.Name, gocheck.Equals, "add-new-routes")
}

func (s *S) TestAddNewRouteForward(c *gocheck.C) {
	app := testing.NewFakeApp("myapp", "python", 1)
	rtesting.FakeRouter.AddBackend(app.GetName())
	defer rtesting.FakeRouter.RemoveBackend(app.GetName())
	cont := container{ID: "ble-1", AppName: app.GetName()}
	cont2 := container{ID: "ble-2", AppName: app.GetName()}
	defer cont.remove()
	defer cont2.remove()
	args := changeUnitsPipelineArgs{
		app: app,
	}
	context := action.FWContext{Previous: []container{cont, cont2}, Params: []interface{}{args}}
	r, err := addNewRoutes.Forward(context)
	c.Assert(err, gocheck.IsNil)
	containers := r.([]container)
	hasRoute := rtesting.FakeRouter.HasRoute(app.GetName(), cont.getAddress())
	c.Assert(hasRoute, gocheck.Equals, true)
	hasRoute = rtesting.FakeRouter.HasRoute(app.GetName(), cont2.getAddress())
	c.Assert(hasRoute, gocheck.Equals, true)
	c.Assert(containers, gocheck.DeepEquals, []container{cont, cont2})
}

func (s *S) TestAddNewRouteForwardFailInMiddle(c *gocheck.C) {
	app := testing.NewFakeApp("myapp", "python", 1)
	rtesting.FakeRouter.AddBackend(app.GetName())
	defer rtesting.FakeRouter.RemoveBackend(app.GetName())
	cont := container{ID: "ble-1", AppName: app.GetName()}
	cont2 := container{ID: "ble-2", AppName: app.GetName()}
	defer cont.remove()
	defer cont2.remove()
	rtesting.FakeRouter.FailForIp(cont2.getAddress())
	args := changeUnitsPipelineArgs{
		app: app,
	}
	context := action.FWContext{Previous: []container{cont, cont2}, Params: []interface{}{args}}
	_, err := addNewRoutes.Forward(context)
	c.Assert(err, gocheck.Equals, rtesting.ErrForcedFailure)
	hasRoute := rtesting.FakeRouter.HasRoute(app.GetName(), cont.getAddress())
	c.Assert(hasRoute, gocheck.Equals, false)
	hasRoute = rtesting.FakeRouter.HasRoute(app.GetName(), cont2.getAddress())
	c.Assert(hasRoute, gocheck.Equals, false)
}

func (s *S) TestAddNewRouteBackward(c *gocheck.C) {
	app := testing.NewFakeApp("myapp", "python", 1)
	rtesting.FakeRouter.AddBackend(app.GetName())
	defer rtesting.FakeRouter.RemoveBackend(app.GetName())
	cont := container{ID: "ble-1", AppName: app.GetName()}
	cont2 := container{ID: "ble-2", AppName: app.GetName()}
	defer cont.remove()
	defer cont2.remove()
	err := rtesting.FakeRouter.AddRoute(app.GetName(), cont.getAddress())
	c.Assert(err, gocheck.IsNil)
	err = rtesting.FakeRouter.AddRoute(app.GetName(), cont2.getAddress())
	c.Assert(err, gocheck.IsNil)
	args := changeUnitsPipelineArgs{
		app: app,
	}
	context := action.BWContext{FWResult: []container{cont, cont2}, Params: []interface{}{args}}
	addNewRoutes.Backward(context)
	hasRoute := rtesting.FakeRouter.HasRoute(app.GetName(), cont.getAddress())
	c.Assert(hasRoute, gocheck.Equals, false)
	hasRoute = rtesting.FakeRouter.HasRoute(app.GetName(), cont2.getAddress())
	c.Assert(hasRoute, gocheck.Equals, false)
}

func (s *S) TestRemoveOldRoutesName(c *gocheck.C) {
	c.Assert(removeOldRoutes.Name, gocheck.Equals, "remove-old-routes")
}

func (s *S) TestRemoveOldRoutesForward(c *gocheck.C) {
	app := testing.NewFakeApp("myapp", "python", 1)
	rtesting.FakeRouter.AddBackend(app.GetName())
	defer rtesting.FakeRouter.RemoveBackend(app.GetName())
	cont := container{ID: "ble-1", AppName: app.GetName()}
	cont2 := container{ID: "ble-2", AppName: app.GetName()}
	defer cont.remove()
	defer cont2.remove()
	err := rtesting.FakeRouter.AddRoute(app.GetName(), cont.getAddress())
	c.Assert(err, gocheck.IsNil)
	err = rtesting.FakeRouter.AddRoute(app.GetName(), cont2.getAddress())
	c.Assert(err, gocheck.IsNil)
	args := changeUnitsPipelineArgs{
		app:      app,
		toRemove: []container{cont, cont2},
	}
	context := action.FWContext{Previous: []container{}, Params: []interface{}{args}}
	r, err := removeOldRoutes.Forward(context)
	c.Assert(err, gocheck.IsNil)
	hasRoute := rtesting.FakeRouter.HasRoute(app.GetName(), cont.getAddress())
	c.Assert(hasRoute, gocheck.Equals, false)
	hasRoute = rtesting.FakeRouter.HasRoute(app.GetName(), cont2.getAddress())
	c.Assert(hasRoute, gocheck.Equals, false)
	containers := r.([]container)
	c.Assert(containers, gocheck.DeepEquals, []container{})
}

func (s *S) TestRemoveOldRoutesForwardFailInMiddle(c *gocheck.C) {
	app := testing.NewFakeApp("myapp", "python", 1)
	rtesting.FakeRouter.AddBackend(app.GetName())
	defer rtesting.FakeRouter.RemoveBackend(app.GetName())
	cont := container{ID: "ble-1", AppName: app.GetName()}
	cont2 := container{ID: "ble-2", AppName: app.GetName()}
	defer cont.remove()
	defer cont2.remove()
	err := rtesting.FakeRouter.AddRoute(app.GetName(), cont.getAddress())
	c.Assert(err, gocheck.IsNil)
	err = rtesting.FakeRouter.AddRoute(app.GetName(), cont2.getAddress())
	c.Assert(err, gocheck.IsNil)
	rtesting.FakeRouter.FailForIp(cont2.getAddress())
	args := changeUnitsPipelineArgs{
		app:      app,
		toRemove: []container{cont, cont2},
	}
	context := action.FWContext{Previous: []container{}, Params: []interface{}{args}}
	_, err = removeOldRoutes.Forward(context)
	c.Assert(err, gocheck.Equals, rtesting.ErrForcedFailure)
	hasRoute := rtesting.FakeRouter.HasRoute(app.GetName(), cont.getAddress())
	c.Assert(hasRoute, gocheck.Equals, true)
	hasRoute = rtesting.FakeRouter.HasRoute(app.GetName(), cont2.getAddress())
	c.Assert(hasRoute, gocheck.Equals, true)
}

func (s *S) TestRemoveOldRoutesBackward(c *gocheck.C) {
	app := testing.NewFakeApp("myapp", "python", 1)
	rtesting.FakeRouter.AddBackend(app.GetName())
	defer rtesting.FakeRouter.RemoveBackend(app.GetName())
	cont := container{ID: "ble-1", AppName: app.GetName()}
	cont2 := container{ID: "ble-2", AppName: app.GetName()}
	defer cont.remove()
	defer cont2.remove()
	args := changeUnitsPipelineArgs{
		app:      app,
		toRemove: []container{cont, cont2},
	}
	context := action.BWContext{Params: []interface{}{args}}
	removeOldRoutes.Backward(context)
	hasRoute := rtesting.FakeRouter.HasRoute(app.GetName(), cont.getAddress())
	c.Assert(hasRoute, gocheck.Equals, true)
	hasRoute = rtesting.FakeRouter.HasRoute(app.GetName(), cont2.getAddress())
	c.Assert(hasRoute, gocheck.Equals, true)
}

func (s *S) TestSetNetworkInfoName(c *gocheck.C) {
	c.Assert(setNetworkInfo.Name, gocheck.Equals, "set-network-info")
}

func (s *S) TestSetNetworkInfoForward(c *gocheck.C) {
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
	c.Assert(cont.SSHHostPort, gocheck.Not(gocheck.Equals), "")
}

func (s *S) TestSetImage(c *gocheck.C) {
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
	conta, err := s.newContainer(nil)
	c.Assert(err, gocheck.IsNil)
	defer s.removeTestContainer(conta)
	cont := *conta
	context := action.FWContext{Previous: cont, Params: []interface{}{runContainerActionsArgs{}}}
	r, err := startContainer.Forward(context)
	c.Assert(err, gocheck.IsNil)
	cont = r.(container)
	c.Assert(cont, gocheck.FitsTypeOf, container{})
}

func (s *S) TestStartContainerBackward(c *gocheck.C) {
	dcli, err := docker.NewClient(s.server.URL())
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

func (s *S) TestProvisionAddUnitsToHostName(c *gocheck.C) {
	c.Assert(provisionAddUnitsToHost.Name, gocheck.Equals, "provision-add-units-to-host")
}

func (s *S) TestProvisionAddUnitsToHostForward(c *gocheck.C) {
	cluster, err := s.startMultipleServersCluster()
	c.Assert(err, gocheck.IsNil)
	defer s.stopMultipleServersCluster(cluster)
	err = newImage("tsuru/app-myapp-2", s.server.URL())
	c.Assert(err, gocheck.IsNil)
	var p dockerProvisioner
	app := testing.NewFakeApp("myapp-2", "python", 0)
	defer p.Destroy(app)
	p.Provision(app)
	coll := collection()
	defer coll.Close()
	coll.Insert(container{ID: "container-id", AppName: app.GetName(), Version: "container-version", Image: "tsuru/python"})
	defer coll.RemoveAll(bson.M{"appname": app.GetName()})
	args := changeUnitsPipelineArgs{
		app:        app,
		toHost:     "localhost",
		unitsToAdd: 2,
	}
	context := action.FWContext{Params: []interface{}{args}}
	result, err := provisionAddUnitsToHost.Forward(context)
	c.Assert(err, gocheck.IsNil)
	containers := result.([]container)
	c.Assert(containers, gocheck.HasLen, 2)
	c.Assert(containers[0].HostAddr, gocheck.Equals, "localhost")
	c.Assert(containers[1].HostAddr, gocheck.Equals, "localhost")
	count, err := coll.Find(bson.M{"appname": app.GetName()}).Count()
	c.Assert(err, gocheck.IsNil)
	c.Assert(count, gocheck.Equals, 3)
}

func (s *S) TestProvisionAddUnitsToHostForwardWithoutHost(c *gocheck.C) {
	cluster, err := s.startMultipleServersCluster()
	c.Assert(err, gocheck.IsNil)
	defer s.stopMultipleServersCluster(cluster)
	err = newImage("tsuru/app-myapp-2", s.server.URL())
	c.Assert(err, gocheck.IsNil)
	var p dockerProvisioner
	app := testing.NewFakeApp("myapp-2", "python", 0)
	defer p.Destroy(app)
	p.Provision(app)
	coll := collection()
	defer coll.Close()
	coll.Insert(container{ID: "container-id", AppName: app.GetName(), Version: "container-version", Image: "tsuru/python"})
	defer coll.RemoveAll(bson.M{"appname": app.GetName()})
	args := changeUnitsPipelineArgs{
		app:        app,
		unitsToAdd: 3,
	}
	context := action.FWContext{Params: []interface{}{args}}
	result, err := provisionAddUnitsToHost.Forward(context)
	c.Assert(err, gocheck.IsNil)
	containers := result.([]container)
	c.Assert(containers, gocheck.HasLen, 3)
	addrs := []string{containers[0].HostAddr, containers[1].HostAddr}
	sort.Strings(addrs)
	c.Assert(addrs[0], gocheck.Equals, "127.0.0.1")
	c.Assert(addrs[1], gocheck.Equals, "localhost")
	count, err := coll.Find(bson.M{"appname": app.GetName()}).Count()
	c.Assert(err, gocheck.IsNil)
	c.Assert(count, gocheck.Equals, 4)
}

func (s *S) TestProvisionAddUnitsToHostBackward(c *gocheck.C) {
	err := newImage("tsuru/python", s.server.URL())
	c.Assert(err, gocheck.IsNil)
	var p dockerProvisioner
	app := testing.NewFakeApp("myapp-xxx-1", "python", 0)
	defer p.Destroy(app)
	p.Provision(app)
	coll := collection()
	defer coll.Close()
	cont := container{ID: "container-id", AppName: app.GetName(), Version: "container-version", Image: "tsuru/python"}
	coll.Insert(cont)
	defer coll.RemoveAll(bson.M{"appname": app.GetName()})
	context := action.BWContext{FWResult: []container{cont}}
	provisionAddUnitsToHost.Backward(context)
	_, err = getContainer(cont.ID)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "not found")
}

func (s *S) TestProvisionRemoveOldUnitsName(c *gocheck.C) {
	c.Assert(provisionRemoveOldUnits.Name, gocheck.Equals, "provision-remove-old-units")
}

func (s *S) TestProvisionRemoveOldUnitsForward(c *gocheck.C) {
	cont, err := s.newContainer(nil)
	c.Assert(err, gocheck.IsNil)
	defer rtesting.FakeRouter.RemoveBackend(cont.AppName)
	client, err := docker.NewClient(s.server.URL())
	c.Assert(err, gocheck.IsNil)
	err = client.StartContainer(cont.ID, nil)
	c.Assert(err, gocheck.IsNil)
	app := testing.NewFakeApp(cont.AppName, "python", 0)
	unit := cont.asUnit(app)
	app.BindUnit(&unit)
	args := changeUnitsPipelineArgs{
		app:      app,
		toRemove: []container{*cont},
	}
	context := action.FWContext{Params: []interface{}{args}, Previous: []container{}}
	result, err := provisionRemoveOldUnits.Forward(context)
	c.Assert(err, gocheck.IsNil)
	resultContainers := result.([]container)
	c.Assert(resultContainers, gocheck.DeepEquals, []container{})
	_, err = getContainer(cont.ID)
	c.Assert(err, gocheck.NotNil)
	c.Assert(app.HasBind(&unit), gocheck.Equals, false)
}

func (s *S) TestFollowLogsAndCommitName(c *gocheck.C) {
	c.Assert(followLogsAndCommit.Name, gocheck.Equals, "follow-logs-and-commit")
}

func (s *S) TestFollowLogsAndCommitForward(c *gocheck.C) {
	go s.stopContainers(1)
	err := newImage("tsuru/python", s.server.URL())
	c.Assert(err, gocheck.IsNil)
	app := testing.NewFakeApp("myapp", "python", 1)
	cont := container{AppName: "mightyapp", ID: "myid123"}
	err = cont.create(runContainerActionsArgs{app: app, imageID: "tsuru/python", commands: []string{"foo"}})
	c.Assert(err, gocheck.IsNil)
	var buf bytes.Buffer
	args := runContainerActionsArgs{writer: &buf}
	context := action.FWContext{Params: []interface{}{args}, Previous: cont}
	imageId, err := followLogsAndCommit.Forward(context)
	c.Assert(err, gocheck.IsNil)
	c.Assert(imageId, gocheck.Equals, "tsuru/app-mightyapp")
	c.Assert(buf.String(), gocheck.Not(gocheck.Equals), "")
	var dbCont container
	coll := collection()
	defer coll.Close()
	err = coll.Find(bson.M{"id": cont.ID}).One(&dbCont)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "not found")
	_, err = dockerCluster().InspectContainer(cont.ID)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Matches, "No such container.*")
	err = dockerCluster().RemoveImage("tsuru/app-mightyapp")
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestFollowLogsAndCommitForwardNonZeroStatus(c *gocheck.C) {
	go s.stopContainers(1)
	err := newImage("tsuru/python", s.server.URL())
	c.Assert(err, gocheck.IsNil)
	app := testing.NewFakeApp("myapp", "python", 1)
	cont := container{AppName: "mightyapp"}
	err = cont.create(runContainerActionsArgs{app: app, imageID: "tsuru/python", commands: []string{"foo"}})
	c.Assert(err, gocheck.IsNil)
	err = s.server.MutateContainer(cont.ID, docker.State{ExitCode: 1})
	c.Assert(err, gocheck.IsNil)
	var buf bytes.Buffer
	args := runContainerActionsArgs{writer: &buf}
	context := action.FWContext{Params: []interface{}{args}, Previous: cont}
	imageId, err := followLogsAndCommit.Forward(context)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "Exit status 1")
	c.Assert(imageId, gocheck.IsNil)
}

func (s *S) TestFollowLogsAndCommitForwardWaitFailure(c *gocheck.C) {
	s.server.PrepareFailure("failed to wait for the container", "/containers/.*/wait")
	defer s.server.ResetFailure("failed to wait for the container")
	err := newImage("tsuru/python", s.server.URL())
	c.Assert(err, gocheck.IsNil)
	app := testing.NewFakeApp("myapp", "python", 1)
	cont := container{AppName: "mightyapp"}
	err = cont.create(runContainerActionsArgs{app: app, imageID: "tsuru/python", commands: []string{"foo"}})
	c.Assert(err, gocheck.IsNil)
	var buf bytes.Buffer
	args := runContainerActionsArgs{writer: &buf}
	context := action.FWContext{Params: []interface{}{args}, Previous: cont}
	imageId, err := followLogsAndCommit.Forward(context)
	c.Assert(err, gocheck.ErrorMatches, `.*failed to wait for the container\n$`)
	c.Assert(imageId, gocheck.IsNil)
}
