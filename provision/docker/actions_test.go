// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"sort"
	"strings"

	"github.com/fsouza/go-dockerclient"
	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/router/routertest"
	"github.com/tsuru/tsuru/safe"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

func (s *S) TestInsertEmptyContainerInDBName(c *check.C) {
	c.Assert(insertEmptyContainerInDB.Name, check.Equals, "insert-empty-container")
}

func (s *S) TestInsertEmptyContainerInDBForward(c *check.C) {
	app := provisiontest.NewFakeApp("myapp", "python", 1)
	args := runContainerActionsArgs{
		app:           app,
		imageID:       "image-id",
		buildingImage: "next-image",
		provisioner:   s.p,
	}
	context := action.FWContext{Params: []interface{}{args}}
	r, err := insertEmptyContainerInDB.Forward(context)
	c.Assert(err, check.IsNil)
	cont := r.(container)
	c.Assert(cont, check.FitsTypeOf, container{})
	c.Assert(cont.AppName, check.Equals, app.GetName())
	c.Assert(cont.Type, check.Equals, app.GetPlatform())
	c.Assert(cont.Name, check.Not(check.Equals), "")
	c.Assert(strings.HasPrefix(cont.Name, app.GetName()+"-"), check.Equals, true)
	c.Assert(cont.Name, check.HasLen, 26)
	c.Assert(cont.Status, check.Equals, "created")
	c.Assert(cont.Image, check.Equals, "image-id")
	c.Assert(cont.BuildingImage, check.Equals, "next-image")
	coll := s.p.collection()
	defer coll.Close()
	defer coll.Remove(bson.M{"name": cont.Name})
	var retrieved container
	err = coll.Find(bson.M{"name": cont.Name}).One(&retrieved)
	c.Assert(err, check.IsNil)
	c.Assert(retrieved.Name, check.Equals, cont.Name)
}

func (s *S) TestInsertEmptyContainerInDBBackward(c *check.C) {
	cont := container{Name: "myName"}
	coll := s.p.collection()
	defer coll.Close()
	err := coll.Insert(&cont)
	c.Assert(err, check.IsNil)
	context := action.BWContext{FWResult: cont, Params: []interface{}{runContainerActionsArgs{
		provisioner: s.p,
	}}}
	insertEmptyContainerInDB.Backward(context)
	err = coll.Find(bson.M{"name": cont.Name}).One(&cont)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "not found")
}

func (s *S) TestUpdateContainerInDBName(c *check.C) {
	c.Assert(updateContainerInDB.Name, check.Equals, "update-database-container")
}

func (s *S) TestUpdateContainerInDBForward(c *check.C) {
	cont := container{Name: "myName"}
	coll := s.p.collection()
	defer coll.Close()
	err := coll.Insert(cont)
	c.Assert(err, check.IsNil)
	cont.ID = "myID"
	context := action.FWContext{Previous: cont, Params: []interface{}{runContainerActionsArgs{
		provisioner: s.p,
	}}}
	r, err := updateContainerInDB.Forward(context)
	c.Assert(r, check.FitsTypeOf, container{})
	retrieved, err := s.p.getContainer(cont.ID)
	c.Assert(err, check.IsNil)
	c.Assert(retrieved.ID, check.Equals, cont.ID)
}

func (s *S) TestCreateContainerName(c *check.C) {
	c.Assert(createContainer.Name, check.Equals, "create-container")
}

func (s *S) TestCreateContainerForward(c *check.C) {
	err := s.newFakeImage(s.p, "tsuru/python", nil)
	c.Assert(err, check.IsNil)
	client, err := docker.NewClient(s.server.URL())
	c.Assert(err, check.IsNil)
	images, err := client.ListImages(docker.ListImagesOptions{All: true})
	c.Assert(err, check.IsNil)
	cmds := []string{"ps", "-ef"}
	app := provisiontest.NewFakeApp("myapp", "python", 1)
	cont := container{Name: "myName", AppName: app.GetName(), Type: app.GetPlatform(), Status: "created"}
	args := runContainerActionsArgs{
		app:         app,
		imageID:     images[0].ID,
		commands:    cmds,
		provisioner: s.p,
	}
	context := action.FWContext{Previous: cont, Params: []interface{}{args}}
	r, err := createContainer.Forward(context)
	c.Assert(err, check.IsNil)
	cont = r.(container)
	defer cont.remove(s.p)
	c.Assert(cont, check.FitsTypeOf, container{})
	c.Assert(cont.ID, check.Not(check.Equals), "")
	c.Assert(cont.HostAddr, check.Equals, "127.0.0.1")
	dcli, err := docker.NewClient(s.server.URL())
	c.Assert(err, check.IsNil)
	cc, err := dcli.InspectContainer(cont.ID)
	c.Assert(err, check.IsNil)
	c.Assert(cc.State.Running, check.Equals, false)
}

func (s *S) TestCreateContainerBackward(c *check.C) {
	dcli, err := docker.NewClient(s.server.URL())
	c.Assert(err, check.IsNil)
	err = s.newFakeImage(s.p, "tsuru/python", nil)
	c.Assert(err, check.IsNil)
	defer dcli.RemoveImage("tsuru/python")
	conta, err := s.newContainer(nil, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(conta)
	cont := *conta
	args := runContainerActionsArgs{
		provisioner: s.p,
	}
	context := action.BWContext{FWResult: cont, Params: []interface{}{args}}
	createContainer.Backward(context)
	_, err = dcli.InspectContainer(cont.ID)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.FitsTypeOf, &docker.NoSuchContainer{})
}

func (s *S) TestAddNewRouteName(c *check.C) {
	c.Assert(addNewRoutes.Name, check.Equals, "add-new-routes")
}

func (s *S) TestAddNewRouteForward(c *check.C) {
	app := provisiontest.NewFakeApp("myapp", "python", 1)
	routertest.FakeRouter.AddBackend(app.GetName())
	defer routertest.FakeRouter.RemoveBackend(app.GetName())
	cont := container{ID: "ble-1", AppName: app.GetName()}
	cont2 := container{ID: "ble-2", AppName: app.GetName()}
	defer cont.remove(s.p)
	defer cont2.remove(s.p)
	args := changeUnitsPipelineArgs{
		app:         app,
		provisioner: s.p,
	}
	context := action.FWContext{Previous: []container{cont, cont2}, Params: []interface{}{args}}
	r, err := addNewRoutes.Forward(context)
	c.Assert(err, check.IsNil)
	containers := r.([]container)
	hasRoute := routertest.FakeRouter.HasRoute(app.GetName(), cont.getAddress())
	c.Assert(hasRoute, check.Equals, true)
	hasRoute = routertest.FakeRouter.HasRoute(app.GetName(), cont2.getAddress())
	c.Assert(hasRoute, check.Equals, true)
	c.Assert(containers, check.DeepEquals, []container{cont, cont2})
}

func (s *S) TestAddNewRouteForwardFailInMiddle(c *check.C) {
	app := provisiontest.NewFakeApp("myapp", "python", 1)
	routertest.FakeRouter.AddBackend(app.GetName())
	defer routertest.FakeRouter.RemoveBackend(app.GetName())
	cont := container{ID: "ble-1", AppName: app.GetName()}
	cont2 := container{ID: "ble-2", AppName: app.GetName()}
	defer cont.remove(s.p)
	defer cont2.remove(s.p)
	routertest.FakeRouter.FailForIp(cont2.getAddress())
	args := changeUnitsPipelineArgs{
		app:         app,
		provisioner: s.p,
	}
	context := action.FWContext{Previous: []container{cont, cont2}, Params: []interface{}{args}}
	_, err := addNewRoutes.Forward(context)
	c.Assert(err, check.Equals, routertest.ErrForcedFailure)
	hasRoute := routertest.FakeRouter.HasRoute(app.GetName(), cont.getAddress())
	c.Assert(hasRoute, check.Equals, false)
	hasRoute = routertest.FakeRouter.HasRoute(app.GetName(), cont2.getAddress())
	c.Assert(hasRoute, check.Equals, false)
}

func (s *S) TestAddNewRouteBackward(c *check.C) {
	app := provisiontest.NewFakeApp("myapp", "python", 1)
	routertest.FakeRouter.AddBackend(app.GetName())
	defer routertest.FakeRouter.RemoveBackend(app.GetName())
	cont := container{ID: "ble-1", AppName: app.GetName()}
	cont2 := container{ID: "ble-2", AppName: app.GetName()}
	defer cont.remove(s.p)
	defer cont2.remove(s.p)
	err := routertest.FakeRouter.AddRoute(app.GetName(), cont.getAddress())
	c.Assert(err, check.IsNil)
	err = routertest.FakeRouter.AddRoute(app.GetName(), cont2.getAddress())
	c.Assert(err, check.IsNil)
	args := changeUnitsPipelineArgs{
		app:         app,
		provisioner: s.p,
	}
	context := action.BWContext{FWResult: []container{cont, cont2}, Params: []interface{}{args}}
	addNewRoutes.Backward(context)
	hasRoute := routertest.FakeRouter.HasRoute(app.GetName(), cont.getAddress())
	c.Assert(hasRoute, check.Equals, false)
	hasRoute = routertest.FakeRouter.HasRoute(app.GetName(), cont2.getAddress())
	c.Assert(hasRoute, check.Equals, false)
}

func (s *S) TestRemoveOldRoutesName(c *check.C) {
	c.Assert(removeOldRoutes.Name, check.Equals, "remove-old-routes")
}

func (s *S) TestRemoveOldRoutesForward(c *check.C) {
	app := provisiontest.NewFakeApp("myapp", "python", 1)
	routertest.FakeRouter.AddBackend(app.GetName())
	defer routertest.FakeRouter.RemoveBackend(app.GetName())
	cont := container{ID: "ble-1", AppName: app.GetName()}
	cont2 := container{ID: "ble-2", AppName: app.GetName()}
	defer cont.remove(s.p)
	defer cont2.remove(s.p)
	err := routertest.FakeRouter.AddRoute(app.GetName(), cont.getAddress())
	c.Assert(err, check.IsNil)
	err = routertest.FakeRouter.AddRoute(app.GetName(), cont2.getAddress())
	c.Assert(err, check.IsNil)
	args := changeUnitsPipelineArgs{
		app:         app,
		toRemove:    []container{cont, cont2},
		provisioner: s.p,
	}
	context := action.FWContext{Previous: []container{}, Params: []interface{}{args}}
	r, err := removeOldRoutes.Forward(context)
	c.Assert(err, check.IsNil)
	hasRoute := routertest.FakeRouter.HasRoute(app.GetName(), cont.getAddress())
	c.Assert(hasRoute, check.Equals, false)
	hasRoute = routertest.FakeRouter.HasRoute(app.GetName(), cont2.getAddress())
	c.Assert(hasRoute, check.Equals, false)
	containers := r.([]container)
	c.Assert(containers, check.DeepEquals, []container{})
}

func (s *S) TestRemoveOldRoutesForwardFailInMiddle(c *check.C) {
	app := provisiontest.NewFakeApp("myapp", "python", 1)
	routertest.FakeRouter.AddBackend(app.GetName())
	defer routertest.FakeRouter.RemoveBackend(app.GetName())
	cont := container{ID: "ble-1", AppName: app.GetName()}
	cont2 := container{ID: "ble-2", AppName: app.GetName()}
	defer cont.remove(s.p)
	defer cont2.remove(s.p)
	err := routertest.FakeRouter.AddRoute(app.GetName(), cont.getAddress())
	c.Assert(err, check.IsNil)
	err = routertest.FakeRouter.AddRoute(app.GetName(), cont2.getAddress())
	c.Assert(err, check.IsNil)
	routertest.FakeRouter.FailForIp(cont2.getAddress())
	args := changeUnitsPipelineArgs{
		app:         app,
		toRemove:    []container{cont, cont2},
		provisioner: s.p,
	}
	context := action.FWContext{Previous: []container{}, Params: []interface{}{args}}
	_, err = removeOldRoutes.Forward(context)
	c.Assert(err, check.Equals, routertest.ErrForcedFailure)
	hasRoute := routertest.FakeRouter.HasRoute(app.GetName(), cont.getAddress())
	c.Assert(hasRoute, check.Equals, true)
	hasRoute = routertest.FakeRouter.HasRoute(app.GetName(), cont2.getAddress())
	c.Assert(hasRoute, check.Equals, true)
}

func (s *S) TestRemoveOldRoutesBackward(c *check.C) {
	app := provisiontest.NewFakeApp("myapp", "python", 1)
	routertest.FakeRouter.AddBackend(app.GetName())
	defer routertest.FakeRouter.RemoveBackend(app.GetName())
	cont := container{ID: "ble-1", AppName: app.GetName()}
	cont2 := container{ID: "ble-2", AppName: app.GetName()}
	defer cont.remove(s.p)
	defer cont2.remove(s.p)
	args := changeUnitsPipelineArgs{
		app:         app,
		toRemove:    []container{cont, cont2},
		provisioner: s.p,
	}
	context := action.BWContext{Params: []interface{}{args}}
	removeOldRoutes.Backward(context)
	hasRoute := routertest.FakeRouter.HasRoute(app.GetName(), cont.getAddress())
	c.Assert(hasRoute, check.Equals, true)
	hasRoute = routertest.FakeRouter.HasRoute(app.GetName(), cont2.getAddress())
	c.Assert(hasRoute, check.Equals, true)
}

func (s *S) TestSetNetworkInfoName(c *check.C) {
	c.Assert(setNetworkInfo.Name, check.Equals, "set-network-info")
}

func (s *S) TestSetNetworkInfoForward(c *check.C) {
	conta, err := s.newContainer(nil, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(conta)
	cont := *conta
	context := action.FWContext{Previous: cont, Params: []interface{}{runContainerActionsArgs{
		provisioner: s.p,
	}}}
	r, err := setNetworkInfo.Forward(context)
	c.Assert(err, check.IsNil)
	cont = r.(container)
	c.Assert(cont, check.FitsTypeOf, container{})
	c.Assert(cont.IP, check.Not(check.Equals), "")
	c.Assert(cont.HostPort, check.Not(check.Equals), "")
}

func (s *S) TestSetImage(c *check.C) {
	conta, err := s.newContainer(nil, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(conta)
	cont := *conta
	context := action.FWContext{Previous: cont, Params: []interface{}{runContainerActionsArgs{
		provisioner: s.p,
	}}}
	r, err := setNetworkInfo.Forward(context)
	c.Assert(err, check.IsNil)
	cont = r.(container)
	c.Assert(cont, check.FitsTypeOf, container{})
	c.Assert(cont.HostPort, check.Not(check.Equals), "")
}

func (s *S) TestStartContainerForward(c *check.C) {
	conta, err := s.newContainer(nil, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(conta)
	cont := *conta
	context := action.FWContext{Previous: cont, Params: []interface{}{runContainerActionsArgs{
		provisioner: s.p,
	}}}
	r, err := startContainer.Forward(context)
	c.Assert(err, check.IsNil)
	cont = r.(container)
	c.Assert(cont, check.FitsTypeOf, container{})
}

func (s *S) TestStartContainerBackward(c *check.C) {
	dcli, err := docker.NewClient(s.server.URL())
	c.Assert(err, check.IsNil)
	err = s.newFakeImage(s.p, "tsuru/python", nil)
	c.Assert(err, check.IsNil)
	defer dcli.RemoveImage("tsuru/python")
	conta, err := s.newContainer(nil, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(conta)
	cont := *conta
	err = dcli.StartContainer(cont.ID, nil)
	c.Assert(err, check.IsNil)
	context := action.BWContext{FWResult: cont, Params: []interface{}{runContainerActionsArgs{
		provisioner: s.p,
	}}}
	startContainer.Backward(context)
	cc, err := dcli.InspectContainer(cont.ID)
	c.Assert(err, check.IsNil)
	c.Assert(cc.State.Running, check.Equals, false)
}

func (s *S) TestProvisionAddUnitsToHostName(c *check.C) {
	c.Assert(provisionAddUnitsToHost.Name, check.Equals, "provision-add-units-to-host")
}

func (s *S) TestProvisionAddUnitsToHostForward(c *check.C) {
	p, err := s.startMultipleServersCluster()
	c.Assert(err, check.IsNil)
	defer s.stopMultipleServersCluster(p)
	app := provisiontest.NewFakeApp("myapp-2", "python", 0)
	defer p.Destroy(app)
	p.Provision(app)
	coll := p.collection()
	defer coll.Close()
	coll.Insert(container{ID: "container-id", AppName: app.GetName(), Version: "container-version", Image: "tsuru/python"})
	defer coll.RemoveAll(bson.M{"appname": app.GetName()})
	imageId, err := appNewImageName(app.GetName())
	c.Assert(err, check.IsNil)
	err = s.newFakeImage(p, imageId, nil)
	c.Assert(err, check.IsNil)
	args := changeUnitsPipelineArgs{
		app:         app,
		toHost:      "localhost",
		toAdd:       map[string]int{"web": 2},
		imageId:     imageId,
		provisioner: p,
	}
	context := action.FWContext{Params: []interface{}{args}}
	result, err := provisionAddUnitsToHost.Forward(context)
	c.Assert(err, check.IsNil)
	containers := result.([]container)
	c.Assert(containers, check.HasLen, 2)
	c.Assert(containers[0].HostAddr, check.Equals, "localhost")
	c.Assert(containers[1].HostAddr, check.Equals, "localhost")
	count, err := coll.Find(bson.M{"appname": app.GetName()}).Count()
	c.Assert(err, check.IsNil)
	c.Assert(count, check.Equals, 3)
}

func (s *S) TestProvisionAddUnitsToHostForwardWithoutHost(c *check.C) {
	p, err := s.startMultipleServersCluster()
	c.Assert(err, check.IsNil)
	defer s.stopMultipleServersCluster(p)
	app := provisiontest.NewFakeApp("myapp-2", "python", 0)
	defer p.Destroy(app)
	p.Provision(app)
	coll := p.collection()
	defer coll.Close()
	imageId, err := appNewImageName(app.GetName())
	c.Assert(err, check.IsNil)
	err = s.newFakeImage(p, imageId, nil)
	c.Assert(err, check.IsNil)
	args := changeUnitsPipelineArgs{
		app:         app,
		toAdd:       map[string]int{"web": 3},
		imageId:     imageId,
		provisioner: p,
	}
	context := action.FWContext{Params: []interface{}{args}}
	result, err := provisionAddUnitsToHost.Forward(context)
	c.Assert(err, check.IsNil)
	containers := result.([]container)
	c.Assert(containers, check.HasLen, 3)
	addrs := []string{containers[0].HostAddr, containers[1].HostAddr, containers[2].HostAddr}
	sort.Strings(addrs)
	isValid := reflect.DeepEqual(addrs, []string{"127.0.0.1", "localhost", "localhost"}) ||
		reflect.DeepEqual(addrs, []string{"127.0.0.1", "127.0.0.1", "localhost"})
	if !isValid {
		clusterNodes, _ := p.getCluster().UnfilteredNodes()
		c.Fatalf("Expected multiple hosts, got: %#v\nAvailable nodes: %#v", containers, clusterNodes)
	}
	count, err := coll.Find(bson.M{"appname": app.GetName()}).Count()
	c.Assert(err, check.IsNil)
	c.Assert(count, check.Equals, 3)
}

func (s *S) TestProvisionAddUnitsToHostBackward(c *check.C) {
	err := s.newFakeImage(s.p, "tsuru/python", nil)
	c.Assert(err, check.IsNil)
	app := provisiontest.NewFakeApp("myapp-xxx-1", "python", 0)
	defer s.p.Destroy(app)
	s.p.Provision(app)
	coll := s.p.collection()
	defer coll.Close()
	cont := container{ID: "container-id", AppName: app.GetName(), Version: "container-version", Image: "tsuru/python"}
	coll.Insert(cont)
	defer coll.RemoveAll(bson.M{"appname": app.GetName()})
	args := changeUnitsPipelineArgs{
		provisioner: s.p,
	}
	context := action.BWContext{FWResult: []container{cont}, Params: []interface{}{args}}
	provisionAddUnitsToHost.Backward(context)
	_, err = s.p.getContainer(cont.ID)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "not found")
}

func (s *S) TestProvisionRemoveOldUnitsName(c *check.C) {
	c.Assert(provisionRemoveOldUnits.Name, check.Equals, "provision-remove-old-units")
}

func (s *S) TestProvisionRemoveOldUnitsForward(c *check.C) {
	cont, err := s.newContainer(nil, nil)
	c.Assert(err, check.IsNil)
	defer routertest.FakeRouter.RemoveBackend(cont.AppName)
	client, err := docker.NewClient(s.server.URL())
	c.Assert(err, check.IsNil)
	err = client.StartContainer(cont.ID, nil)
	c.Assert(err, check.IsNil)
	app := provisiontest.NewFakeApp(cont.AppName, "python", 0)
	unit := cont.asUnit(app)
	app.BindUnit(&unit)
	args := changeUnitsPipelineArgs{
		app:         app,
		toRemove:    []container{*cont},
		provisioner: s.p,
	}
	context := action.FWContext{Params: []interface{}{args}, Previous: []container{}}
	result, err := provisionRemoveOldUnits.Forward(context)
	c.Assert(err, check.IsNil)
	resultContainers := result.([]container)
	c.Assert(resultContainers, check.DeepEquals, []container{})
	_, err = s.p.getContainer(cont.ID)
	c.Assert(err, check.NotNil)
}

func (s *S) TestProvisionUnbindOldUnitsName(c *check.C) {
	c.Assert(provisionUnbindOldUnits.Name, check.Equals, "provision-unbind-old-units")
}

func (s *S) TestProvisionUnbindOldUnitsForward(c *check.C) {
	cont, err := s.newContainer(nil, nil)
	c.Assert(err, check.IsNil)
	defer routertest.FakeRouter.RemoveBackend(cont.AppName)
	client, err := docker.NewClient(s.server.URL())
	c.Assert(err, check.IsNil)
	err = client.StartContainer(cont.ID, nil)
	c.Assert(err, check.IsNil)
	app := provisiontest.NewFakeApp(cont.AppName, "python", 0)
	unit := cont.asUnit(app)
	app.BindUnit(&unit)
	args := changeUnitsPipelineArgs{
		app:         app,
		toRemove:    []container{*cont},
		provisioner: s.p,
	}
	context := action.FWContext{Params: []interface{}{args}, Previous: []container{}}
	result, err := provisionUnbindOldUnits.Forward(context)
	c.Assert(err, check.IsNil)
	resultContainers := result.([]container)
	c.Assert(resultContainers, check.DeepEquals, []container{})
	c.Assert(app.HasBind(&unit), check.Equals, false)
}

func (s *S) TestFollowLogsAndCommitName(c *check.C) {
	c.Assert(followLogsAndCommit.Name, check.Equals, "follow-logs-and-commit")
}

func (s *S) TestFollowLogsAndCommitForward(c *check.C) {
	go s.stopContainers(1)
	err := s.newFakeImage(s.p, "tsuru/python", nil)
	c.Assert(err, check.IsNil)
	app := provisiontest.NewFakeApp("mightyapp", "python", 1)
	nextImgName, err := appNewImageName(app.GetName())
	c.Assert(err, check.IsNil)
	cont := container{AppName: "mightyapp", ID: "myid123", BuildingImage: nextImgName}
	err = cont.create(runContainerActionsArgs{
		app:         app,
		imageID:     "tsuru/python",
		commands:    []string{"foo"},
		provisioner: s.p,
	})
	c.Assert(err, check.IsNil)
	buf := safe.NewBuffer(nil)
	args := runContainerActionsArgs{writer: buf, provisioner: s.p}
	context := action.FWContext{Params: []interface{}{args}, Previous: cont}
	imageId, err := followLogsAndCommit.Forward(context)
	c.Assert(err, check.IsNil)
	c.Assert(imageId, check.Equals, "tsuru/app-mightyapp:v1")
	c.Assert(buf.String(), check.Not(check.Equals), "")
	var dbCont container
	coll := s.p.collection()
	defer coll.Close()
	err = coll.Find(bson.M{"id": cont.ID}).One(&dbCont)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "not found")
	_, err = s.p.getCluster().InspectContainer(cont.ID)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Matches, "No such container.*")
	err = s.p.getCluster().RemoveImage("tsuru/app-mightyapp:v1")
	c.Assert(err, check.IsNil)
}

func (s *S) TestFollowLogsAndCommitForwardNonZeroStatus(c *check.C) {
	go s.stopContainers(1)
	err := s.newFakeImage(s.p, "tsuru/python", nil)
	c.Assert(err, check.IsNil)
	app := provisiontest.NewFakeApp("myapp", "python", 1)
	cont := container{AppName: "mightyapp"}
	err = cont.create(runContainerActionsArgs{
		app:         app,
		imageID:     "tsuru/python",
		commands:    []string{"foo"},
		provisioner: s.p,
	})
	c.Assert(err, check.IsNil)
	err = s.server.MutateContainer(cont.ID, docker.State{ExitCode: 1})
	c.Assert(err, check.IsNil)
	buf := safe.NewBuffer(nil)
	args := runContainerActionsArgs{writer: buf, provisioner: s.p}
	context := action.FWContext{Params: []interface{}{args}, Previous: cont}
	imageId, err := followLogsAndCommit.Forward(context)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Exit status 1")
	c.Assert(imageId, check.IsNil)
}

func (s *S) TestFollowLogsAndCommitForwardWaitFailure(c *check.C) {
	s.server.PrepareFailure("failed to wait for the container", "/containers/.*/wait")
	defer s.server.ResetFailure("failed to wait for the container")
	err := s.newFakeImage(s.p, "tsuru/python", nil)
	c.Assert(err, check.IsNil)
	app := provisiontest.NewFakeApp("myapp", "python", 1)
	cont := container{AppName: "mightyapp"}
	err = cont.create(runContainerActionsArgs{
		app:         app,
		imageID:     "tsuru/python",
		commands:    []string{"foo"},
		provisioner: s.p,
	})
	c.Assert(err, check.IsNil)
	buf := safe.NewBuffer(nil)
	args := runContainerActionsArgs{writer: buf, provisioner: s.p}
	context := action.FWContext{Params: []interface{}{args}, Previous: cont}
	imageId, err := followLogsAndCommit.Forward(context)
	c.Assert(err, check.ErrorMatches, `.*failed to wait for the container\n$`)
	c.Assert(imageId, check.IsNil)
}

func (s *S) TestBindAndHealthcheckName(c *check.C) {
	c.Assert(bindAndHealthcheck.Name, check.Equals, "bind-and-healthcheck")
}

func (s *S) TestBindAndHealthcheckForward(c *check.C) {
	appName := "my-fake-app"
	err := s.newFakeImage(s.p, "tsuru/app-"+appName, nil)
	c.Assert(err, check.IsNil)
	fakeApp := provisiontest.NewFakeApp(appName, "python", 0)
	s.p.Provision(fakeApp)
	defer s.p.Destroy(fakeApp)
	buf := safe.NewBuffer(nil)
	args := changeUnitsPipelineArgs{
		app:         fakeApp,
		provisioner: s.p,
		writer:      buf,
		toAdd:       map[string]int{"web": 2},
		imageId:     "tsuru/app-" + appName,
	}
	containers, err := addContainersWithHost(&args)
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 2)
	context := action.FWContext{Params: []interface{}{args}, Previous: containers}
	result, err := bindAndHealthcheck.Forward(context)
	c.Assert(err, check.IsNil)
	resultContainers := result.([]container)
	c.Assert(resultContainers, check.DeepEquals, containers)
	u1 := containers[0].asUnit(fakeApp)
	u2 := containers[1].asUnit(fakeApp)
	c.Assert(fakeApp.HasBind(&u1), check.Equals, true)
	c.Assert(fakeApp.HasBind(&u2), check.Equals, true)
}

func (s *S) TestBindAndHealthcheckDontHealtcheckForErroredApps(c *check.C) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	dbApp := &app.App{Name: "myapp"}
	err = conn.Apps().Insert(dbApp)
	c.Assert(err, check.IsNil)
	defer conn.Apps().Remove(bson.M{"name": dbApp.Name})
	imageName := "tsuru/app-" + dbApp.Name
	customData := map[string]interface{}{
		"healthcheck": map[string]interface{}{
			"path":   "/x/y",
			"status": http.StatusOK,
		},
		"procfile": "web: python myapp.py",
	}
	err = s.newFakeImage(s.p, imageName, customData)
	c.Assert(err, check.IsNil)
	fakeApp := provisiontest.NewFakeApp(dbApp.Name, "python", 0)
	s.p.Provision(fakeApp)
	defer s.p.Destroy(fakeApp)
	buf := safe.NewBuffer(nil)
	contOpts := newContainerOpts{
		Status: "error",
	}
	oldContainer, err := s.newContainer(&contOpts, nil)
	c.Assert(err, check.IsNil)
	args := changeUnitsPipelineArgs{
		app:         fakeApp,
		provisioner: s.p,
		writer:      buf,
		toAdd:       map[string]int{"web": 2},
		imageId:     "tsuru/app-" + dbApp.Name,
		toRemove:    []container{*oldContainer},
	}
	containers, err := addContainersWithHost(&args)
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 2)
	url, _ := url.Parse(server.URL)
	host, port, _ := net.SplitHostPort(url.Host)
	containers[0].HostAddr = host
	containers[0].HostPort = port
	containers[1].HostAddr = host
	containers[1].HostPort = port
	context := action.FWContext{Params: []interface{}{args}, Previous: containers}
	result, err := bindAndHealthcheck.Forward(context)
	c.Assert(err, check.IsNil)
	resultContainers := result.([]container)
	c.Assert(resultContainers, check.DeepEquals, containers)
	u1 := containers[0].asUnit(fakeApp)
	u2 := containers[1].asUnit(fakeApp)
	c.Assert(fakeApp.HasBind(&u1), check.Equals, true)
	c.Assert(fakeApp.HasBind(&u2), check.Equals, true)
}

func (s *S) TestBindAndHealthcheckDontHealtcheckForStoppedApps(c *check.C) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	dbApp := &app.App{Name: "myapp"}
	err = conn.Apps().Insert(dbApp)
	c.Assert(err, check.IsNil)
	defer conn.Apps().Remove(bson.M{"name": dbApp.Name})
	imageName := "tsuru/app-" + dbApp.Name
	customData := map[string]interface{}{
		"healthcheck": map[string]interface{}{
			"path":   "/x/y",
			"status": http.StatusOK,
		},
		"procfile": "web: python myapp.py",
	}
	err = s.newFakeImage(s.p, imageName, customData)
	c.Assert(err, check.IsNil)
	fakeApp := provisiontest.NewFakeApp(dbApp.Name, "python", 0)
	s.p.Provision(fakeApp)
	defer s.p.Destroy(fakeApp)
	buf := safe.NewBuffer(nil)
	contOpts := newContainerOpts{
		Status: "stopped",
	}
	oldContainer, err := s.newContainer(&contOpts, nil)
	c.Assert(err, check.IsNil)
	args := changeUnitsPipelineArgs{
		app:         fakeApp,
		provisioner: s.p,
		writer:      buf,
		toAdd:       map[string]int{"web": 2},
		imageId:     "tsuru/app-" + dbApp.Name,
		toRemove:    []container{*oldContainer},
	}
	containers, err := addContainersWithHost(&args)
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 2)
	url, _ := url.Parse(server.URL)
	host, port, _ := net.SplitHostPort(url.Host)
	containers[0].HostAddr = host
	containers[0].HostPort = port
	containers[1].HostAddr = host
	containers[1].HostPort = port
	context := action.FWContext{Params: []interface{}{args}, Previous: containers}
	result, err := bindAndHealthcheck.Forward(context)
	c.Assert(err, check.IsNil)
	resultContainers := result.([]container)
	c.Assert(resultContainers, check.DeepEquals, containers)
	u1 := containers[0].asUnit(fakeApp)
	u2 := containers[1].asUnit(fakeApp)
	c.Assert(fakeApp.HasBind(&u1), check.Equals, true)
	c.Assert(fakeApp.HasBind(&u2), check.Equals, true)
}

func (s *S) TestBindAndHealthcheckForwardHealthcheckError(c *check.C) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	dbApp := &app.App{Name: "myapp"}
	err = conn.Apps().Insert(dbApp)
	c.Assert(err, check.IsNil)
	defer conn.Apps().Remove(bson.M{"name": dbApp.Name})
	imageName := "tsuru/app-" + dbApp.Name
	customData := map[string]interface{}{
		"healthcheck": map[string]interface{}{
			"path":   "/x/y",
			"status": http.StatusOK,
		},
		"procfile": "web: python start_app.py",
	}
	err = s.newFakeImage(s.p, imageName, customData)
	c.Assert(err, check.IsNil)
	fakeApp := provisiontest.NewFakeApp(dbApp.Name, "python", 0)
	s.p.Provision(fakeApp)
	defer s.p.Destroy(fakeApp)
	buf := safe.NewBuffer(nil)
	args := changeUnitsPipelineArgs{
		app:         fakeApp,
		provisioner: s.p,
		writer:      buf,
		toAdd:       map[string]int{"web": 2},
		imageId:     "tsuru/app-" + dbApp.Name,
	}
	containers, err := addContainersWithHost(&args)
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 2)
	url, _ := url.Parse(server.URL)
	host, port, _ := net.SplitHostPort(url.Host)
	containers[0].HostAddr = host
	containers[0].HostPort = port
	containers[1].HostAddr = host
	containers[1].HostPort = port
	context := action.FWContext{Params: []interface{}{args}, Previous: containers}
	_, err = bindAndHealthcheck.Forward(context)
	c.Assert(err, check.ErrorMatches, `healthcheck fail\(.*?\): wrong status code, expected 200, got: 404`)
	u1 := containers[0].asUnit(fakeApp)
	u2 := containers[1].asUnit(fakeApp)
	c.Assert(fakeApp.HasBind(&u1), check.Equals, false)
	c.Assert(fakeApp.HasBind(&u2), check.Equals, false)
}

func (s *S) TestBindAndHealthcheckForwardRestartError(c *check.C) {
	s.server.CustomHandler("/exec/.*/json", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ID":"id","ExitCode":9}`))
	}))
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	dbApp := &app.App{Name: "myapp"}
	err = conn.Apps().Insert(dbApp)
	c.Assert(err, check.IsNil)
	defer conn.Apps().Remove(bson.M{"name": dbApp.Name})
	imageName := "tsuru/app-" + dbApp.Name
	customData := map[string]interface{}{
		"hooks": map[string]interface{}{
			"restart": map[string]interface{}{
				"after": []string{"will fail"},
			},
		},
		"procfile": "web: python myapp.py",
	}
	err = s.newFakeImage(s.p, imageName, customData)
	c.Assert(err, check.IsNil)
	fakeApp := provisiontest.NewFakeApp(dbApp.Name, "python", 0)
	s.p.Provision(fakeApp)
	defer s.p.Destroy(fakeApp)
	buf := safe.NewBuffer(nil)
	args := changeUnitsPipelineArgs{
		app:         fakeApp,
		provisioner: s.p,
		writer:      buf,
		toAdd:       map[string]int{"web": 2},
		imageId:     "tsuru/app-" + dbApp.Name,
	}
	containers, err := addContainersWithHost(&args)
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 2)
	context := action.FWContext{Params: []interface{}{args}, Previous: containers}
	_, err = bindAndHealthcheck.Forward(context)
	c.Assert(err, check.ErrorMatches, `couldn't execute restart:after hook "will fail"\(.+?\): unexpected exit code: 9`)
	u1 := containers[0].asUnit(fakeApp)
	u2 := containers[1].asUnit(fakeApp)
	c.Assert(fakeApp.HasBind(&u1), check.Equals, false)
	c.Assert(fakeApp.HasBind(&u2), check.Equals, false)
}

func (s *S) TestBindAndHealthcheckBackward(c *check.C) {
	appName := "my-fake-app"
	err := s.newFakeImage(s.p, "tsuru/app-"+appName, nil)
	c.Assert(err, check.IsNil)
	fakeApp := provisiontest.NewFakeApp(appName, "python", 0)
	s.p.Provision(fakeApp)
	defer s.p.Destroy(fakeApp)
	buf := safe.NewBuffer(nil)
	args := changeUnitsPipelineArgs{
		app:         fakeApp,
		provisioner: s.p,
		writer:      buf,
		toAdd:       map[string]int{"web": 2},
		imageId:     "tsuru/app-" + appName,
	}
	containers, err := addContainersWithHost(&args)
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 2)
	context := action.BWContext{Params: []interface{}{args}, FWResult: containers}
	for _, c := range containers {
		u := c.asUnit(fakeApp)
		fakeApp.BindUnit(&u)
	}
	bindAndHealthcheck.Backward(context)
	c.Assert(err, check.IsNil)
	u1 := containers[0].asUnit(fakeApp)
	c.Assert(fakeApp.HasBind(&u1), check.Equals, false)
	u2 := containers[1].asUnit(fakeApp)
	c.Assert(fakeApp.HasBind(&u2), check.Equals, false)
}
