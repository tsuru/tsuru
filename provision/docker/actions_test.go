// Copyright 2012 tsuru authors. All rights reserved.
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
	"sync"

	"github.com/fsouza/go-dockerclient"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/app/image"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/docker/container"
	"github.com/tsuru/tsuru/provision/docker/types"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/router"
	"github.com/tsuru/tsuru/router/routertest"
	"github.com/tsuru/tsuru/safe"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

func (s *S) TestRunInContainers(c *check.C) {
	conts := []container.Container{
		{Container: types.Container{ID: "1"}}, {Container: types.Container{ID: "2"}}, {Container: types.Container{ID: "3"}}, {Container: types.Container{ID: "4"}},
	}
	var called []string
	var mtx sync.Mutex
	runFunc := func(cont *container.Container, ch chan *container.Container) error {
		mtx.Lock()
		defer mtx.Unlock()
		called = append(called, cont.ID)
		return nil
	}
	err := runInContainers(conts, runFunc, nil, true)
	c.Assert(err, check.IsNil)
	sort.Strings(called)
	c.Assert(called, check.DeepEquals, []string{"1", "2", "3", "4"})
}

func (s *S) TestRunInContainersOddMaxWorkers(c *check.C) {
	config.Set("docker:max-workers", 3)
	defer config.Unset("docker:max-workers")
	conts := []container.Container{
		{Container: types.Container{ID: "1"}}, {Container: types.Container{ID: "2"}}, {Container: types.Container{ID: "3"}}, {Container: types.Container{ID: "4"}},
	}
	var called []string
	var mtx sync.Mutex
	runFunc := func(cont *container.Container, ch chan *container.Container) error {
		mtx.Lock()
		defer mtx.Unlock()
		called = append(called, cont.ID)
		return nil
	}
	err := runInContainers(conts, runFunc, nil, true)
	c.Assert(err, check.IsNil)
	sort.Strings(called)
	c.Assert(called, check.DeepEquals, []string{"1", "2", "3", "4"})
}

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
	cont := r.(container.Container)
	c.Assert(cont, check.FitsTypeOf, container.Container{})
	c.Assert(cont.AppName, check.Equals, app.GetName())
	c.Assert(cont.Type, check.Equals, app.GetPlatform())
	c.Assert(cont.Name, check.Not(check.Equals), "")
	c.Assert(strings.HasPrefix(cont.Name, app.GetName()+"-"), check.Equals, true)
	c.Assert(cont.Name, check.HasLen, 26)
	c.Assert(cont.Status, check.Equals, "created")
	c.Assert(cont.Image, check.Equals, "image-id")
	c.Assert(cont.BuildingImage, check.Equals, "next-image")
	c.Assert(string(cont.MongoID), check.Not(check.Equals), "")
	coll := s.p.Collection()
	defer coll.Close()
	defer coll.Remove(bson.M{"name": cont.Name})
	var retrieved container.Container
	err = coll.Find(bson.M{"name": cont.Name}).One(&retrieved)
	c.Assert(err, check.IsNil)
	c.Assert(retrieved.Name, check.Equals, cont.Name)
}

func (s *S) TestInsertEmptyContainerInDBForDeployForward(c *check.C) {
	app := provisiontest.NewFakeApp("myapp", "python", 1)
	args := runContainerActionsArgs{
		app:           app,
		imageID:       "image-id",
		buildingImage: "next-image",
		provisioner:   s.p,
		isDeploy:      true,
	}
	context := action.FWContext{Params: []interface{}{args}}
	r, err := insertEmptyContainerInDB.Forward(context)
	c.Assert(err, check.IsNil)
	cont := r.(container.Container)
	c.Assert(cont, check.FitsTypeOf, container.Container{})
	c.Assert(cont.AppName, check.Equals, app.GetName())
	c.Assert(cont.Type, check.Equals, app.GetPlatform())
	c.Assert(cont.Name, check.Not(check.Equals), "")
	c.Assert(strings.HasPrefix(cont.Name, app.GetName()+"-"), check.Equals, true)
	c.Assert(cont.Name, check.HasLen, 26)
	c.Assert(cont.Status, check.Equals, "building")
	c.Assert(cont.Image, check.Equals, "image-id")
	c.Assert(cont.BuildingImage, check.Equals, "next-image")
	c.Assert(string(cont.MongoID), check.Not(check.Equals), "")
	coll := s.p.Collection()
	defer coll.Close()
	defer coll.Remove(bson.M{"name": cont.Name})
	var retrieved container.Container
	err = coll.Find(bson.M{"name": cont.Name}).One(&retrieved)
	c.Assert(err, check.IsNil)
	c.Assert(retrieved.Name, check.Equals, cont.Name)
}

func (s *S) TestInsertEmptyContainerInDBBackward(c *check.C) {
	cont := container.Container{Container: types.Container{MongoID: bson.NewObjectId()}}
	coll := s.p.Collection()
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
	cont := container.Container{Container: types.Container{Name: "myName"}}
	coll := s.p.Collection()
	defer coll.Close()
	err := coll.Insert(cont)
	c.Assert(err, check.IsNil)
	cont.ID = "myID"
	context := action.FWContext{Previous: cont, Params: []interface{}{runContainerActionsArgs{
		provisioner: s.p,
	}}}
	r, err := updateContainerInDB.Forward(context)
	c.Assert(err, check.IsNil)
	c.Assert(r, check.FitsTypeOf, container.Container{})
	retrieved, err := s.p.GetContainer(cont.ID)
	c.Assert(err, check.IsNil)
	c.Assert(retrieved.ID, check.Equals, cont.ID)
}

func (s *S) TestCreateContainerName(c *check.C) {
	c.Assert(createContainer.Name, check.Equals, "create-container")
}

func (s *S) TestCreateContainerForward(c *check.C) {
	config.Set("docker:user", "ubuntu")
	defer config.Unset("docker:user")
	err := newFakeImage(s.p, "tsuru/python", nil)
	c.Assert(err, check.IsNil)
	client, err := docker.NewClient(s.server.URL())
	c.Assert(err, check.IsNil)
	images, err := client.ListImages(docker.ListImagesOptions{All: true})
	c.Assert(err, check.IsNil)
	cmds := []string{"ps", "-ef"}
	app := provisiontest.NewFakeApp("myapp", "python", 1)
	cont := container.Container{Container: types.Container{Name: "myName", AppName: app.GetName(), Type: app.GetPlatform(), Status: "created"}}
	args := runContainerActionsArgs{
		app:           app,
		imageID:       images[0].ID,
		commands:      cmds,
		provisioner:   s.p,
		buildingImage: images[0].ID,
		isDeploy:      true,
	}
	context := action.FWContext{Previous: cont, Params: []interface{}{args}}
	r, err := createContainer.Forward(context)
	c.Assert(err, check.IsNil)
	cont = r.(container.Container)
	defer cont.Remove(s.p)
	c.Assert(cont, check.FitsTypeOf, container.Container{})
	c.Assert(cont.ID, check.Not(check.Equals), "")
	c.Assert(cont.HostAddr, check.Equals, "127.0.0.1")
	dcli, err := docker.NewClient(s.server.URL())
	c.Assert(err, check.IsNil)
	cc, err := dcli.InspectContainer(cont.ID)
	c.Assert(err, check.IsNil)
	c.Assert(cc.State.Running, check.Equals, false)
	c.Assert(cc.Config.User, check.Equals, "ubuntu")
	args = runContainerActionsArgs{
		app:         app,
		imageID:     images[0].ID,
		commands:    cmds,
		provisioner: s.p,
	}
	optsPull := docker.PullImageOptions{Repository: images[0].ID, OutputStream: nil}
	err = s.p.Cluster().PullImage(optsPull, docker.AuthConfiguration{})
	c.Assert(err, check.IsNil)
	cont = container.Container{Container: types.Container{Name: "myName2", AppName: app.GetName(), Type: app.GetPlatform(), Status: "created"}}
	context = action.FWContext{Previous: cont, Params: []interface{}{args}}
	r, err = createContainer.Forward(context)
	c.Assert(err, check.IsNil)
	cont = r.(container.Container)
	defer cont.Remove(s.p)
	cc, err = dcli.InspectContainer(cont.ID)
	c.Assert(err, check.IsNil)
	c.Assert(cc.Config.User, check.Equals, "")
}

func (s *S) TestCreateContainerBackward(c *check.C) {
	dcli, err := docker.NewClient(s.server.URL())
	c.Assert(err, check.IsNil)
	err = newFakeImage(s.p, "tsuru/python", nil)
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

func (s *S) TestSetContainerIDName(c *check.C) {
	c.Assert(setContainerID.Name, check.Equals, "set-container-id")
}

func (s *S) TestSetContainerIDForward(c *check.C) {
	cont := container.Container{Container: types.Container{Name: "myName"}}
	coll := s.p.Collection()
	defer coll.Close()
	err := coll.Insert(cont)
	c.Assert(err, check.IsNil)
	cont.ID = "cont-id"
	context := action.FWContext{Previous: cont, Params: []interface{}{runContainerActionsArgs{
		provisioner: s.p,
	}}}
	r, err := setContainerID.Forward(context)
	c.Assert(err, check.IsNil)
	c.Assert(r, check.FitsTypeOf, container.Container{})
	retrieved, err := s.p.GetContainer(cont.ID)
	c.Assert(err, check.IsNil)
	c.Assert(retrieved.ID, check.Equals, cont.ID)
}

func (s *S) TestAddNewRouteName(c *check.C) {
	c.Assert(addNewRoutes.Name, check.Equals, "add-new-routes")
}

func (s *S) TestAddNewRouteForward(c *check.C) {
	app := provisiontest.NewFakeApp("myapp", "python", 1)
	imageName := "tsuru/app-" + app.GetName()
	customData := map[string]interface{}{
		"processes": map[string]interface{}{
			"web":    "python myapi.py",
			"worker": "tail -f /dev/null",
		},
	}
	err := image.SaveImageCustomData(imageName, customData)
	c.Assert(err, check.IsNil)
	routertest.FakeRouter.AddBackend(app.GetName())
	defer routertest.FakeRouter.RemoveBackend(app.GetName())
	cont1 := container.Container{Container: types.Container{ID: "ble-1", AppName: app.GetName(), ProcessName: "web", HostAddr: "127.0.0.1", HostPort: "1234"}}
	cont2 := container.Container{Container: types.Container{ID: "ble-2", AppName: app.GetName(), ProcessName: "web", HostAddr: "127.0.0.2", HostPort: "4321"}}
	cont3 := container.Container{Container: types.Container{ID: "ble-3", AppName: app.GetName(), ProcessName: "worker", HostAddr: "127.0.0.3", HostPort: "8080"}}
	defer cont1.Remove(s.p)
	defer cont2.Remove(s.p)
	defer cont3.Remove(s.p)
	args := changeUnitsPipelineArgs{
		app:         app,
		provisioner: s.p,
		imageID:     imageName,
	}
	context := action.FWContext{Previous: []container.Container{cont1, cont2, cont3}, Params: []interface{}{args}}
	r, err := addNewRoutes.Forward(context)
	c.Assert(err, check.IsNil)
	containers := r.([]container.Container)
	hasRoute := routertest.FakeRouter.HasRoute(app.GetName(), cont1.Address().String())
	c.Assert(hasRoute, check.Equals, true)
	hasRoute = routertest.FakeRouter.HasRoute(app.GetName(), cont2.Address().String())
	c.Assert(hasRoute, check.Equals, true)
	hasRoute = routertest.FakeRouter.HasRoute(app.GetName(), cont3.Address().String())
	c.Assert(hasRoute, check.Equals, false)
	c.Assert(containers, check.HasLen, 3)
	c.Assert(containers[0].Routable, check.Equals, true)
	c.Assert(containers[0].ID, check.Equals, "ble-1")
	c.Assert(containers[1].Routable, check.Equals, true)
	c.Assert(containers[1].ID, check.Equals, "ble-2")
	c.Assert(containers[2].Routable, check.Equals, false)
	c.Assert(containers[2].ID, check.Equals, "ble-3")
}

func (s *S) TestAddNewRouteForwardNoWeb(c *check.C) {
	app := provisiontest.NewFakeApp("myapp", "python", 1)
	routertest.FakeRouter.AddBackend(app.GetName())
	defer routertest.FakeRouter.RemoveBackend(app.GetName())
	imageName := "tsuru/app-" + app.GetName()
	customData := map[string]interface{}{
		"processes": map[string]interface{}{
			"api": "python myapi.py",
		},
	}
	err := image.SaveImageCustomData(imageName, customData)
	c.Assert(err, check.IsNil)
	cont1 := container.Container{Container: types.Container{ID: "ble-1", AppName: app.GetName(), ProcessName: "api", HostAddr: "127.0.0.1", HostPort: "1234"}}
	cont2 := container.Container{Container: types.Container{ID: "ble-2", AppName: app.GetName(), ProcessName: "api", HostAddr: "127.0.0.2", HostPort: "4321"}}
	defer cont1.Remove(s.p)
	defer cont2.Remove(s.p)
	args := changeUnitsPipelineArgs{
		app:         app,
		provisioner: s.p,
		imageID:     imageName,
	}
	context := action.FWContext{Previous: []container.Container{cont1, cont2}, Params: []interface{}{args}}
	r, err := addNewRoutes.Forward(context)
	c.Assert(err, check.IsNil)
	containers := r.([]container.Container)
	hasRoute := routertest.FakeRouter.HasRoute(app.GetName(), cont1.Address().String())
	c.Assert(hasRoute, check.Equals, true)
	hasRoute = routertest.FakeRouter.HasRoute(app.GetName(), cont2.Address().String())
	c.Assert(hasRoute, check.Equals, true)
	c.Assert(containers, check.HasLen, 2)
	c.Assert(containers[0].Routable, check.Equals, true)
	c.Assert(containers[0].ID, check.Equals, "ble-1")
	c.Assert(containers[1].Routable, check.Equals, true)
	c.Assert(containers[1].ID, check.Equals, "ble-2")
}

func (s *S) TestAddNewRouteForwardFailInMiddle(c *check.C) {
	app := provisiontest.NewFakeApp("myapp", "python", 1)
	routertest.FakeRouter.AddBackend(app.GetName())
	defer routertest.FakeRouter.RemoveBackend(app.GetName())
	cont := container.Container{Container: types.Container{ID: "ble-1", AppName: app.GetName(), ProcessName: "", HostAddr: "addr1", HostPort: "4321"}}
	cont2 := container.Container{Container: types.Container{ID: "ble-2", AppName: app.GetName(), ProcessName: "", HostAddr: "addr2", HostPort: "8080"}}
	defer cont.Remove(s.p)
	defer cont2.Remove(s.p)
	routertest.FakeRouter.FailForIp(cont2.Address().String())
	args := changeUnitsPipelineArgs{
		app:         app,
		provisioner: s.p,
	}
	prevContainers := []container.Container{cont, cont2}
	context := action.FWContext{Previous: prevContainers, Params: []interface{}{args}}
	_, err := addNewRoutes.Forward(context)
	c.Assert(err, check.Equals, routertest.ErrForcedFailure)
	hasRoute := routertest.FakeRouter.HasRoute(app.GetName(), cont.Address().String())
	c.Assert(hasRoute, check.Equals, false)
	hasRoute = routertest.FakeRouter.HasRoute(app.GetName(), cont2.Address().String())
	c.Assert(hasRoute, check.Equals, false)
	c.Assert(prevContainers[0].Routable, check.Equals, true)
	c.Assert(prevContainers[0].ID, check.Equals, "ble-1")
	c.Assert(prevContainers[1].Routable, check.Equals, true)
	c.Assert(prevContainers[1].ID, check.Equals, "ble-2")
}

func (s *S) TestAddNewRouteForwardDoesNotAddWhenHostPortIsZero(c *check.C) {
	app := provisiontest.NewFakeApp("myapp", "python", 1)
	routertest.FakeRouter.AddBackend(app.GetName())
	defer routertest.FakeRouter.RemoveBackend(app.GetName())
	cont := container.Container{Container: types.Container{ID: "ble-1", AppName: app.GetName(), ProcessName: "", HostAddr: "addr1", HostPort: "0"}}
	cont2 := container.Container{Container: types.Container{ID: "ble-2", AppName: app.GetName(), ProcessName: "", HostAddr: "addr2", HostPort: "4321"}}
	defer cont.Remove(s.p)
	defer cont2.Remove(s.p)
	args := changeUnitsPipelineArgs{
		app:         app,
		provisioner: s.p,
	}
	prevContainers := []container.Container{cont, cont2}
	context := action.FWContext{Previous: prevContainers, Params: []interface{}{args}}
	_, err := addNewRoutes.Forward(context)
	c.Assert(err, check.Equals, nil)
	hasRoute := routertest.FakeRouter.HasRoute(app.GetName(), cont.Address().String())
	c.Assert(hasRoute, check.Equals, false)
	hasRoute = routertest.FakeRouter.HasRoute(app.GetName(), cont2.Address().String())
	c.Assert(hasRoute, check.Equals, true)
}

func (s *S) TestAddNewRouteForwardDoesNotAddWhenHostPortIsEmpty(c *check.C) {
	app := provisiontest.NewFakeApp("myapp", "python", 1)
	routertest.FakeRouter.AddBackend(app.GetName())
	defer routertest.FakeRouter.RemoveBackend(app.GetName())
	cont := container.Container{Container: types.Container{ID: "ble-1", AppName: app.GetName(), ProcessName: "", HostAddr: "addr1", HostPort: ""}}
	cont2 := container.Container{Container: types.Container{ID: "ble-2", AppName: app.GetName(), ProcessName: "", HostAddr: "addr2", HostPort: "4321"}}
	defer cont.Remove(s.p)
	defer cont2.Remove(s.p)
	args := changeUnitsPipelineArgs{
		app:         app,
		provisioner: s.p,
	}
	prevContainers := []container.Container{cont, cont2}
	context := action.FWContext{Previous: prevContainers, Params: []interface{}{args}}
	_, err := addNewRoutes.Forward(context)
	c.Assert(err, check.Equals, nil)
	hasRoute := routertest.FakeRouter.HasRoute(app.GetName(), cont.Address().String())
	c.Assert(hasRoute, check.Equals, false)
	hasRoute = routertest.FakeRouter.HasRoute(app.GetName(), cont2.Address().String())
	c.Assert(hasRoute, check.Equals, true)
}

func (s *S) TestAddNewRouteBackward(c *check.C) {
	app := provisiontest.NewFakeApp("myapp", "python", 1)
	routertest.FakeRouter.AddBackend(app.GetName())
	defer routertest.FakeRouter.RemoveBackend(app.GetName())
	cont1 := container.Container{Container: types.Container{ID: "ble-1", AppName: app.GetName(), ProcessName: "web", HostAddr: "127.0.0.1", HostPort: "1234"}}
	cont2 := container.Container{Container: types.Container{ID: "ble-2", AppName: app.GetName(), ProcessName: "web", HostAddr: "127.0.0.2", HostPort: "4321"}}
	cont3 := container.Container{Container: types.Container{ID: "ble-3", AppName: app.GetName(), ProcessName: "worker", HostAddr: "127.0.0.3", HostPort: "8080"}}
	defer cont1.Remove(s.p)
	defer cont2.Remove(s.p)
	defer cont3.Remove(s.p)
	err := routertest.FakeRouter.AddRoutes(app.GetName(), []*url.URL{cont1.Address(), cont2.Address()})
	c.Assert(err, check.IsNil)
	args := changeUnitsPipelineArgs{
		app:         app,
		provisioner: s.p,
	}
	cont1.Routable = true
	cont2.Routable = true
	context := action.BWContext{FWResult: []container.Container{cont1, cont2, cont3}, Params: []interface{}{args}}
	addNewRoutes.Backward(context)
	hasRoute := routertest.FakeRouter.HasRoute(app.GetName(), cont1.Address().String())
	c.Assert(hasRoute, check.Equals, false)
	hasRoute = routertest.FakeRouter.HasRoute(app.GetName(), cont2.Address().String())
	c.Assert(hasRoute, check.Equals, false)
	hasRoute = routertest.FakeRouter.HasRoute(app.GetName(), cont3.Address().String())
	c.Assert(hasRoute, check.Equals, false)
}

func (s *S) TestSetRouterHealthcheckForward(c *check.C) {
	app := provisiontest.NewFakeApp("myapp", "python", 1)
	imageName := "tsuru/app-" + app.GetName()
	customData := map[string]interface{}{
		"healthcheck": map[string]interface{}{
			"path":          "/x/y",
			"status":        http.StatusCreated,
			"match":         "ignored",
			"use_in_router": true,
		},
	}
	err := image.SaveImageCustomData(imageName, customData)
	c.Assert(err, check.IsNil)
	routertest.FakeRouter.AddBackend(app.GetName())
	defer routertest.FakeRouter.RemoveBackend(app.GetName())
	args := changeUnitsPipelineArgs{
		app:         app,
		provisioner: s.p,
		imageID:     imageName,
	}
	cont1 := container.Container{Container: types.Container{ID: "ble-1", AppName: app.GetName(), ProcessName: "web", HostAddr: "127.0.0.1", HostPort: "1234"}}
	context := action.FWContext{Previous: []container.Container{cont1}, Params: []interface{}{args}}
	r, err := setRouterHealthcheck.Forward(context)
	c.Assert(err, check.IsNil)
	containers := r.([]container.Container)
	c.Assert(containers, check.HasLen, 1)
	hcData := routertest.FakeRouter.GetHealthcheck(app.GetName())
	c.Assert(hcData, check.DeepEquals, router.HealthcheckData{
		Path:   "/x/y",
		Status: http.StatusCreated,
	})
}

func (s *S) TestSetRouterHealthcheckForwardNoUseInRouter(c *check.C) {
	app := provisiontest.NewFakeApp("myapp", "python", 1)
	imageName := "tsuru/app-" + app.GetName()
	customData := map[string]interface{}{
		"healthcheck": map[string]interface{}{
			"path":   "/x/y",
			"status": http.StatusCreated,
			"match":  "ignored",
		},
	}
	err := image.SaveImageCustomData(imageName, customData)
	c.Assert(err, check.IsNil)
	routertest.FakeRouter.AddBackend(app.GetName())
	defer routertest.FakeRouter.RemoveBackend(app.GetName())
	args := changeUnitsPipelineArgs{
		app:         app,
		provisioner: s.p,
		imageID:     imageName,
	}
	cont1 := container.Container{Container: types.Container{ID: "ble-1", AppName: app.GetName(), ProcessName: "web", HostAddr: "127.0.0.1", HostPort: "1234"}}
	context := action.FWContext{Previous: []container.Container{cont1}, Params: []interface{}{args}}
	r, err := setRouterHealthcheck.Forward(context)
	c.Assert(err, check.IsNil)
	containers := r.([]container.Container)
	c.Assert(containers, check.HasLen, 1)
	hcData := routertest.FakeRouter.GetHealthcheck(app.GetName())
	c.Assert(hcData, check.DeepEquals, router.HealthcheckData{Path: "/"})
}

func (s *S) TestSetRouterHealthcheckBackward(c *check.C) {
	app := provisiontest.NewFakeApp("myapp", "python", 1)
	imageName := "tsuru/img1"
	customData := map[string]interface{}{
		"healthcheck": map[string]interface{}{
			"path":          "/x/y",
			"status":        http.StatusCreated,
			"match":         "ignored",
			"use_in_router": true,
		},
	}
	err := image.SaveImageCustomData(imageName, customData)
	c.Assert(err, check.IsNil)
	routertest.FakeRouter.AddBackend(app.GetName())
	defer routertest.FakeRouter.RemoveBackend(app.GetName())
	args := changeUnitsPipelineArgs{
		app:         app,
		provisioner: s.p,
		imageID:     imageName,
	}
	cont1 := container.Container{Container: types.Container{ID: "ble-1", AppName: app.GetName(), ProcessName: "web", HostAddr: "127.0.0.1", HostPort: "1234"}}
	context := action.FWContext{Previous: []container.Container{cont1}, Params: []interface{}{args}}
	_, err = setRouterHealthcheck.Forward(context)
	c.Assert(err, check.IsNil)
	hcData := routertest.FakeRouter.GetHealthcheck(app.GetName())
	c.Assert(hcData, check.DeepEquals, router.HealthcheckData{
		Path:   "/x/y",
		Status: http.StatusCreated,
	})
	bwcontext := action.BWContext{Params: []interface{}{args}}
	setRouterHealthcheck.Backward(bwcontext)
	hcData = routertest.FakeRouter.GetHealthcheck(app.GetName())
	c.Assert(hcData, check.DeepEquals, router.HealthcheckData{Path: "/"})
}

func (s *S) TestRemoveOldRoutesName(c *check.C) {
	c.Assert(removeOldRoutes.Name, check.Equals, "remove-old-routes")
}

func (s *S) TestRemoveOldRoutesForward(c *check.C) {
	app := provisiontest.NewFakeApp("myapp", "python", 1)
	imageName := "tsuru/app-" + app.GetName()
	customData := map[string]interface{}{
		"processes": map[string]interface{}{
			"web":    "python myapi.py",
			"worker": "tail -f /dev/null",
		},
	}
	err := image.SaveImageCustomData(imageName, customData)
	c.Assert(err, check.IsNil)
	routertest.FakeRouter.AddBackend(app.GetName())
	defer routertest.FakeRouter.RemoveBackend(app.GetName())
	cont1 := container.Container{Container: types.Container{ID: "ble-1", AppName: app.GetName(), ProcessName: "web", HostAddr: "127.0.0.1", HostPort: "1234"}}
	cont2 := container.Container{Container: types.Container{ID: "ble-2", AppName: app.GetName(), ProcessName: "web", HostAddr: "127.0.0.2", HostPort: "4321"}}
	cont3 := container.Container{Container: types.Container{ID: "ble-3", AppName: app.GetName(), ProcessName: "worker", HostAddr: "127.0.0.3", HostPort: "8080"}}
	defer cont1.Remove(s.p)
	defer cont2.Remove(s.p)
	defer cont3.Remove(s.p)
	err = routertest.FakeRouter.AddRoutes(app.GetName(), []*url.URL{cont1.Address(), cont2.Address()})
	c.Assert(err, check.IsNil)
	args := changeUnitsPipelineArgs{
		app:         app,
		toRemove:    []container.Container{cont1, cont2, cont3},
		provisioner: s.p,
	}
	context := action.FWContext{Previous: []container.Container{}, Params: []interface{}{args}}
	r, err := removeOldRoutes.Forward(context)
	c.Assert(err, check.IsNil)
	hasRoute := routertest.FakeRouter.HasRoute(app.GetName(), cont1.Address().String())
	c.Assert(hasRoute, check.Equals, false)
	hasRoute = routertest.FakeRouter.HasRoute(app.GetName(), cont2.Address().String())
	c.Assert(hasRoute, check.Equals, false)
	containers := r.([]container.Container)
	c.Assert(containers, check.DeepEquals, []container.Container{})
	c.Assert(args.toRemove[0].Routable, check.Equals, true)
	c.Assert(args.toRemove[1].Routable, check.Equals, true)
	c.Assert(args.toRemove[2].Routable, check.Equals, false)
}

func (s *S) TestRemoveOldRoutesForwardNoImageData(c *check.C) {
	app := provisiontest.NewFakeApp("myapp", "python", 1)
	err := image.AppendAppImageName(app.GetName(), "img1")
	c.Assert(err, check.IsNil)
	err = image.PullAppImageNames(app.GetName(), []string{"img1"})
	c.Assert(err, check.IsNil)
	routertest.FakeRouter.AddBackend(app.GetName())
	defer routertest.FakeRouter.RemoveBackend(app.GetName())
	cont1 := container.Container{Container: types.Container{ID: "ble-1", AppName: app.GetName(), ProcessName: "", HostAddr: "127.0.0.1", HostPort: ""}}
	args := changeUnitsPipelineArgs{
		app:         app,
		toRemove:    []container.Container{cont1},
		provisioner: s.p,
	}
	context := action.FWContext{Previous: []container.Container{}, Params: []interface{}{args}}
	r, err := removeOldRoutes.Forward(context)
	c.Assert(err, check.IsNil)
	hasRoute := routertest.FakeRouter.HasRoute(app.GetName(), cont1.Address().String())
	c.Assert(hasRoute, check.Equals, false)
	containers := r.([]container.Container)
	c.Assert(containers, check.DeepEquals, []container.Container{})
	c.Assert(args.toRemove[0].Routable, check.Equals, false)
}

func (s *S) TestRemoveOldRoutesForwardFailInMiddle(c *check.C) {
	app := provisiontest.NewFakeApp("myapp", "python", 1)
	imageName := "tsuru/app-" + app.GetName()
	customData := map[string]interface{}{
		"processes": map[string]interface{}{
			"web":    "python myapi.py",
			"worker": "tail -f /dev/null",
		},
	}
	err := image.SaveImageCustomData(imageName, customData)
	c.Assert(err, check.IsNil)
	routertest.FakeRouter.AddBackend(app.GetName())
	defer routertest.FakeRouter.RemoveBackend(app.GetName())
	cont := container.Container{Container: types.Container{ID: "ble-1", AppName: app.GetName(), ProcessName: "web", HostAddr: "addr1", HostPort: "1234"}}
	cont2 := container.Container{Container: types.Container{ID: "ble-2", AppName: app.GetName(), ProcessName: "web", HostAddr: "addr2", HostPort: "1234"}}
	defer cont.Remove(s.p)
	defer cont2.Remove(s.p)
	err = routertest.FakeRouter.AddRoutes(app.GetName(), []*url.URL{cont.Address(), cont2.Address()})
	c.Assert(err, check.IsNil)
	routertest.FakeRouter.FailForIp(cont2.Address().String())
	args := changeUnitsPipelineArgs{
		app:         app,
		toRemove:    []container.Container{cont, cont2},
		provisioner: s.p,
	}
	context := action.FWContext{Previous: []container.Container{}, Params: []interface{}{args}}
	_, err = removeOldRoutes.Forward(context)
	c.Assert(err, check.Equals, routertest.ErrForcedFailure)
	hasRoute := routertest.FakeRouter.HasRoute(app.GetName(), cont.Address().String())
	c.Assert(hasRoute, check.Equals, true)
	hasRoute = routertest.FakeRouter.HasRoute(app.GetName(), cont2.Address().String())
	c.Assert(hasRoute, check.Equals, true)
	c.Assert(args.toRemove[0].Routable, check.Equals, true)
	c.Assert(args.toRemove[1].Routable, check.Equals, true)
}

func (s *S) TestRemoveOldRoutesBackward(c *check.C) {
	app := provisiontest.NewFakeApp("myapp", "python", 1)
	routertest.FakeRouter.AddBackend(app.GetName())
	defer routertest.FakeRouter.RemoveBackend(app.GetName())
	cont := container.Container{Container: types.Container{ID: "ble-1", AppName: app.GetName(), ProcessName: "web"}}
	cont2 := container.Container{Container: types.Container{ID: "ble-2", AppName: app.GetName(), ProcessName: "web"}}
	defer cont.Remove(s.p)
	defer cont2.Remove(s.p)
	cont.Routable = true
	cont2.Routable = true
	args := changeUnitsPipelineArgs{
		app:         app,
		toRemove:    []container.Container{cont, cont2},
		provisioner: s.p,
	}
	context := action.BWContext{Params: []interface{}{args}}
	removeOldRoutes.Backward(context)
	hasRoute := routertest.FakeRouter.HasRoute(app.GetName(), cont.Address().String())
	c.Assert(hasRoute, check.Equals, true)
	hasRoute = routertest.FakeRouter.HasRoute(app.GetName(), cont2.Address().String())
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
	cont = r.(container.Container)
	c.Assert(cont, check.FitsTypeOf, container.Container{})
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
	cont = r.(container.Container)
	c.Assert(cont, check.FitsTypeOf, container.Container{})
	c.Assert(cont.HostPort, check.Not(check.Equals), "")
}

func (s *S) TestStartContainerForward(c *check.C) {
	conta, err := s.newContainer(nil, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(conta)
	cont := *conta
	context := action.FWContext{Previous: cont, Params: []interface{}{runContainerActionsArgs{
		provisioner: s.p,
		app:         provisiontest.NewFakeApp("myapp", "python", 1),
	}}}
	r, err := startContainer.Forward(context)
	c.Assert(err, check.IsNil)
	cont = r.(container.Container)
	c.Assert(cont, check.FitsTypeOf, container.Container{})
}

func (s *S) TestStartContainerBackward(c *check.C) {
	dcli, err := docker.NewClient(s.server.URL())
	c.Assert(err, check.IsNil)
	err = newFakeImage(s.p, "tsuru/python", nil)
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
	app := provisiontest.NewFakeApp("myapp-2", "python", 0)
	defer p.Destroy(app)
	p.Provision(app)
	coll := p.Collection()
	defer coll.Close()
	coll.Insert(container.Container{Container: types.Container{ID: "container-id", AppName: app.GetName(), Version: "container-version", Image: "tsuru/python"}})
	defer coll.RemoveAll(bson.M{"appname": app.GetName()})
	imageID, err := image.AppNewImageName(app.GetName())
	c.Assert(err, check.IsNil)
	err = newFakeImage(p, imageID, nil)
	c.Assert(err, check.IsNil)
	args := changeUnitsPipelineArgs{
		app:         app,
		toHost:      "localhost",
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 2}},
		imageID:     imageID,
		provisioner: p,
	}
	context := action.FWContext{Params: []interface{}{args}}
	result, err := provisionAddUnitsToHost.Forward(context)
	c.Assert(err, check.IsNil)
	containers := result.([]container.Container)
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
	app := provisiontest.NewFakeApp("myapp-2", "python", 0)
	defer p.Destroy(app)
	p.Provision(app)
	coll := p.Collection()
	defer coll.Close()
	imageID, err := image.AppNewImageName(app.GetName())
	c.Assert(err, check.IsNil)
	err = newFakeImage(p, imageID, nil)
	c.Assert(err, check.IsNil)
	args := changeUnitsPipelineArgs{
		app:         app,
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 3}},
		imageID:     imageID,
		provisioner: p,
	}
	context := action.FWContext{Params: []interface{}{args}}
	result, err := provisionAddUnitsToHost.Forward(context)
	c.Assert(err, check.IsNil)
	containers := result.([]container.Container)
	c.Assert(containers, check.HasLen, 3)
	addrs := []string{containers[0].HostAddr, containers[1].HostAddr, containers[2].HostAddr}
	sort.Strings(addrs)
	isValid := reflect.DeepEqual(addrs, []string{"127.0.0.1", "localhost", "localhost"}) ||
		reflect.DeepEqual(addrs, []string{"127.0.0.1", "127.0.0.1", "localhost"})
	if !isValid {
		clusterNodes, _ := p.Cluster().UnfilteredNodes()
		c.Fatalf("Expected multiple hosts, got: %#v\nAvailable nodes: %#v", containers, clusterNodes)
	}
	count, err := coll.Find(bson.M{"appname": app.GetName()}).Count()
	c.Assert(err, check.IsNil)
	c.Assert(count, check.Equals, 3)
}

func (s *S) TestProvisionAddUnitsToHostBackward(c *check.C) {
	err := newFakeImage(s.p, "tsuru/python", nil)
	c.Assert(err, check.IsNil)
	app := provisiontest.NewFakeApp("myapp-xxx-1", "python", 0)
	defer s.p.Destroy(app)
	s.p.Provision(app)
	coll := s.p.Collection()
	defer coll.Close()
	cont := container.Container{Container: types.Container{ID: "container-id", AppName: app.GetName(), Version: "container-version", Image: "tsuru/python"}}
	coll.Insert(cont)
	defer coll.RemoveAll(bson.M{"appname": app.GetName()})
	args := changeUnitsPipelineArgs{
		provisioner: s.p,
	}
	context := action.BWContext{FWResult: []container.Container{cont}, Params: []interface{}{args}}
	provisionAddUnitsToHost.Backward(context)
	_, err = s.p.GetContainer(cont.ID)
	c.Assert(err, check.NotNil)
	e, ok := err.(*provision.UnitNotFoundError)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.ID, check.Equals, cont.ID)
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
	unit := cont.AsUnit(app)
	app.BindUnit(&unit)
	args := changeUnitsPipelineArgs{
		app:         app,
		toRemove:    []container.Container{*cont},
		provisioner: s.p,
	}
	context := action.FWContext{Params: []interface{}{args}, Previous: []container.Container{}}
	result, err := provisionRemoveOldUnits.Forward(context)
	c.Assert(err, check.IsNil)
	resultContainers := result.([]container.Container)
	c.Assert(resultContainers, check.DeepEquals, []container.Container{})
	_, err = s.p.GetContainer(cont.ID)
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
	unit := cont.AsUnit(app)
	app.BindUnit(&unit)
	args := changeUnitsPipelineArgs{
		app:         app,
		toRemove:    []container.Container{*cont},
		provisioner: s.p,
	}
	context := action.FWContext{Params: []interface{}{args}, Previous: []container.Container{}}
	result, err := provisionUnbindOldUnits.Forward(context)
	c.Assert(err, check.IsNil)
	resultContainers := result.([]container.Container)
	c.Assert(resultContainers, check.DeepEquals, []container.Container{})
	c.Assert(app.HasBind(&unit), check.Equals, false)
}

func (s *S) TestFollowLogsAndCommitName(c *check.C) {
	c.Assert(followLogsAndCommit.Name, check.Equals, "follow-logs-and-commit")
}

func (s *S) TestFollowLogsAndCommitForward(c *check.C) {
	err := newFakeImage(s.p, "tsuru/python", nil)
	c.Assert(err, check.IsNil)
	app := provisiontest.NewFakeApp("mightyapp", "python", 1)
	nextImgName, err := image.AppNewImageName(app.GetName())
	c.Assert(err, check.IsNil)
	cont := container.Container{Container: types.Container{AppName: "mightyapp", ID: "myid123", BuildingImage: nextImgName}}
	err = cont.Create(&container.CreateArgs{
		App:         app,
		ImageID:     "tsuru/python",
		Commands:    []string{"foo"},
		Provisioner: s.p,
	})
	c.Assert(err, check.IsNil)
	buf := safe.NewBuffer(nil)
	args := runContainerActionsArgs{writer: buf, provisioner: s.p}
	context := action.FWContext{Params: []interface{}{args}, Previous: cont}
	imageID, err := followLogsAndCommit.Forward(context)
	c.Assert(err, check.IsNil)
	c.Assert(imageID, check.Equals, "tsuru/app-mightyapp:v1")
	c.Assert(buf.String(), check.Not(check.Equals), "")
	var dbCont container.Container
	coll := s.p.Collection()
	defer coll.Close()
	err = coll.Find(bson.M{"id": cont.ID}).One(&dbCont)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "not found")
	_, err = s.p.Cluster().InspectContainer(cont.ID)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Matches, "No such container.*")
	err = s.p.Cluster().RemoveImage("tsuru/app-mightyapp:v1")
	c.Assert(err, check.IsNil)
}

func (s *S) TestFollowLogsAndCommitForwardNonZeroStatus(c *check.C) {
	err := newFakeImage(s.p, "tsuru/python", nil)
	c.Assert(err, check.IsNil)
	app := provisiontest.NewFakeApp("myapp", "python", 1)
	cont := container.Container{Container: types.Container{AppName: "mightyapp"}}
	err = cont.Create(&container.CreateArgs{
		App:         app,
		ImageID:     "tsuru/python",
		Commands:    []string{"foo"},
		Provisioner: s.p,
	})
	c.Assert(err, check.IsNil)
	err = s.server.MutateContainer(cont.ID, docker.State{ExitCode: 1})
	c.Assert(err, check.IsNil)
	buf := safe.NewBuffer(nil)
	args := runContainerActionsArgs{writer: buf, provisioner: s.p}
	context := action.FWContext{Params: []interface{}{args}, Previous: cont}
	imageID, err := followLogsAndCommit.Forward(context)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Exit status 1")
	c.Assert(imageID, check.IsNil)
}

func (s *S) TestFollowLogsAndCommitForwardWaitFailure(c *check.C) {
	s.server.PrepareFailure("failed to wait for the container", "/containers/.*/wait")
	defer s.server.ResetFailure("failed to wait for the container")
	err := newFakeImage(s.p, "tsuru/python", nil)
	c.Assert(err, check.IsNil)
	app := provisiontest.NewFakeApp("myapp", "python", 1)
	cont := container.Container{Container: types.Container{AppName: "mightyapp"}}
	err = cont.Create(&container.CreateArgs{
		App:         app,
		ImageID:     "tsuru/python",
		Commands:    []string{"foo"},
		Provisioner: s.p,
	})
	c.Assert(err, check.IsNil)
	err = cont.Start(&container.StartArgs{
		Provisioner: s.p,
		App:         app,
	})
	c.Assert(err, check.IsNil)
	err = cont.Stop(s.p)
	c.Assert(err, check.IsNil)
	buf := safe.NewBuffer(nil)
	args := runContainerActionsArgs{writer: buf, provisioner: s.p}
	context := action.FWContext{Params: []interface{}{args}, Previous: cont}
	imageID, err := followLogsAndCommit.Forward(context)
	c.Assert(err, check.ErrorMatches, `.*failed to wait for the container\n$`)
	c.Assert(imageID, check.IsNil)
}

func (s *S) TestBindAndHealthcheckName(c *check.C) {
	c.Assert(bindAndHealthcheck.Name, check.Equals, "bind-and-healthcheck")
}

func (s *S) TestBindAndHealthcheckForward(c *check.C) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/x/y" {
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()
	appName := "my-fake-app"
	customData := map[string]interface{}{
		"healthcheck": map[string]interface{}{
			"path":   "/x/y",
			"status": http.StatusOK,
		},
		"processes": map[string]interface{}{
			"web":    "python myapp.py",
			"worker": "python myworker.py",
		},
	}
	err := newFakeImage(s.p, "tsuru/app-"+appName, customData)
	c.Assert(err, check.IsNil)
	fakeApp := provisiontest.NewFakeApp(appName, "python", 0)
	s.p.Provision(fakeApp)
	defer s.p.Destroy(fakeApp)
	buf := safe.NewBuffer(nil)
	args := changeUnitsPipelineArgs{
		app:         fakeApp,
		provisioner: s.p,
		writer:      buf,
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 2}, "worker": {Quantity: 1}},
		imageID:     "tsuru/app-" + appName,
	}
	containers, err := addContainersWithHost(&args)
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 3)
	url, _ := url.Parse(server.URL)
	host, port, _ := net.SplitHostPort(url.Host)
	for i := range containers {
		if containers[i].ProcessName == "web" {
			containers[i].HostAddr = host
			containers[i].HostPort = port
		}
	}
	context := action.FWContext{Params: []interface{}{args}, Previous: containers}
	result, err := bindAndHealthcheck.Forward(context)
	c.Assert(err, check.IsNil)
	resultContainers := result.([]container.Container)
	c.Assert(resultContainers, check.DeepEquals, containers)
	u1 := containers[0].AsUnit(fakeApp)
	u2 := containers[1].AsUnit(fakeApp)
	c.Assert(fakeApp.HasBind(&u1), check.Equals, true)
	c.Assert(fakeApp.HasBind(&u2), check.Equals, true)
}

func (s *S) TestBindAndHealthcheckDontHealtcheckForErroredApps(c *check.C) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()
	dbApp := &app.App{Name: "myapp"}
	err := s.conn.Apps().Insert(dbApp)
	c.Assert(err, check.IsNil)
	imageName := "tsuru/app-" + dbApp.Name
	customData := map[string]interface{}{
		"healthcheck": map[string]interface{}{
			"path":   "/x/y",
			"status": http.StatusOK,
		},
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	}
	err = newFakeImage(s.p, imageName, customData)
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
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 2}},
		imageID:     "tsuru/app-" + dbApp.Name,
		toRemove:    []container.Container{*oldContainer},
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
	resultContainers := result.([]container.Container)
	c.Assert(resultContainers, check.DeepEquals, containers)
	u1 := containers[0].AsUnit(fakeApp)
	u2 := containers[1].AsUnit(fakeApp)
	c.Assert(fakeApp.HasBind(&u1), check.Equals, true)
	c.Assert(fakeApp.HasBind(&u2), check.Equals, true)
}

func (s *S) TestBindAndHealthcheckDontHealtcheckForStoppedApps(c *check.C) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()
	dbApp := &app.App{Name: "myapp"}
	err := s.conn.Apps().Insert(dbApp)
	c.Assert(err, check.IsNil)
	imageName := "tsuru/app-" + dbApp.Name
	customData := map[string]interface{}{
		"healthcheck": map[string]interface{}{
			"path":   "/x/y",
			"status": http.StatusOK,
		},
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	}
	err = newFakeImage(s.p, imageName, customData)
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
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 2}},
		imageID:     "tsuru/app-" + dbApp.Name,
		toRemove:    []container.Container{*oldContainer},
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
	resultContainers := result.([]container.Container)
	c.Assert(resultContainers, check.DeepEquals, containers)
	u1 := containers[0].AsUnit(fakeApp)
	u2 := containers[1].AsUnit(fakeApp)
	c.Assert(fakeApp.HasBind(&u1), check.Equals, true)
	c.Assert(fakeApp.HasBind(&u2), check.Equals, true)
}

func (s *S) TestBindAndHealthcheckForwardHealthcheckError(c *check.C) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()
	dbApp := &app.App{Name: "myapp"}
	err := s.conn.Apps().Insert(dbApp)
	c.Assert(err, check.IsNil)
	imageName := "tsuru/app-" + dbApp.Name
	customData := map[string]interface{}{
		"healthcheck": map[string]interface{}{
			"path":   "/x/y",
			"status": http.StatusOK,
		},
		"processes": map[string]interface{}{
			"web": "python start_app.py",
		},
	}
	err = newFakeImage(s.p, imageName, customData)
	c.Assert(err, check.IsNil)
	fakeApp := provisiontest.NewFakeApp(dbApp.Name, "python", 0)
	s.p.Provision(fakeApp)
	defer s.p.Destroy(fakeApp)
	buf := safe.NewBuffer(nil)
	args := changeUnitsPipelineArgs{
		app:         fakeApp,
		provisioner: s.p,
		writer:      buf,
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 2}},
		imageID:     "tsuru/app-" + dbApp.Name,
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
	u1 := containers[0].AsUnit(fakeApp)
	u2 := containers[1].AsUnit(fakeApp)
	c.Assert(fakeApp.HasBind(&u1), check.Equals, false)
	c.Assert(fakeApp.HasBind(&u2), check.Equals, false)
}

func (s *S) TestBindAndHealthcheckForwardRestartError(c *check.C) {
	s.server.CustomHandler("/exec/.*/json", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ID":"id","ExitCode":9}`))
	}))
	dbApp := &app.App{Name: "myapp"}
	err := s.conn.Apps().Insert(dbApp)
	c.Assert(err, check.IsNil)
	imageName := "tsuru/app-" + dbApp.Name
	customData := map[string]interface{}{
		"hooks": map[string]interface{}{
			"restart": map[string]interface{}{
				"after": []string{"will fail"},
			},
		},
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	}
	err = newFakeImage(s.p, imageName, customData)
	c.Assert(err, check.IsNil)
	fakeApp := provisiontest.NewFakeApp(dbApp.Name, "python", 0)
	s.p.Provision(fakeApp)
	defer s.p.Destroy(fakeApp)
	buf := safe.NewBuffer(nil)
	args := changeUnitsPipelineArgs{
		app:         fakeApp,
		provisioner: s.p,
		writer:      buf,
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 2}},
		imageID:     "tsuru/app-" + dbApp.Name,
	}
	containers, err := addContainersWithHost(&args)
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 2)
	context := action.FWContext{Params: []interface{}{args}, Previous: containers}
	_, err = bindAndHealthcheck.Forward(context)
	c.Assert(err, check.ErrorMatches, `couldn't execute restart:after hook "will fail"\(.+?\): unexpected exit code: 9`)
	u1 := containers[0].AsUnit(fakeApp)
	u2 := containers[1].AsUnit(fakeApp)
	c.Assert(fakeApp.HasBind(&u1), check.Equals, false)
	c.Assert(fakeApp.HasBind(&u2), check.Equals, false)
}

func (s *S) TestBindAndHealthcheckBackward(c *check.C) {
	appName := "my-fake-app"
	err := newFakeImage(s.p, "tsuru/app-"+appName, nil)
	c.Assert(err, check.IsNil)
	fakeApp := provisiontest.NewFakeApp(appName, "python", 0)
	s.p.Provision(fakeApp)
	defer s.p.Destroy(fakeApp)
	buf := safe.NewBuffer(nil)
	args := changeUnitsPipelineArgs{
		app:         fakeApp,
		provisioner: s.p,
		writer:      buf,
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 2}},
		imageID:     "tsuru/app-" + appName,
	}
	containers, err := addContainersWithHost(&args)
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 2)
	context := action.BWContext{Params: []interface{}{args}, FWResult: containers}
	for _, c := range containers {
		u := c.AsUnit(fakeApp)
		fakeApp.BindUnit(&u)
	}
	bindAndHealthcheck.Backward(context)
	c.Assert(err, check.IsNil)
	u1 := containers[0].AsUnit(fakeApp)
	c.Assert(fakeApp.HasBind(&u1), check.Equals, false)
	u2 := containers[1].AsUnit(fakeApp)
	c.Assert(fakeApp.HasBind(&u2), check.Equals, false)
}
