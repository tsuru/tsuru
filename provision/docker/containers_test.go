// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"io/ioutil"
	"net/http"
	"strings"

	dtesting "github.com/fsouza/go-dockerclient/testing"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/safe"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

func (s *S) TestMoveContainers(c *check.C) {
	p, err := s.startMultipleServersCluster()
	c.Assert(err, check.IsNil)
	defer s.stopMultipleServersCluster(p)
	err = s.newFakeImage(p, "tsuru/app-myapp", nil)
	c.Assert(err, check.IsNil)
	appInstance := provisiontest.NewFakeApp("myapp", "python", 0)
	defer p.Destroy(appInstance)
	p.Provision(appInstance)
	coll := p.collection()
	defer coll.Close()
	coll.Insert(container{ID: "container-id", AppName: appInstance.GetName(), Version: "container-version", Image: "tsuru/python"})
	defer coll.RemoveAll(bson.M{"appname": appInstance.GetName()})
	imageId, err := appCurrentImageName(appInstance.GetName())
	c.Assert(err, check.IsNil)
	_, err = addContainersWithHost(&changeUnitsPipelineArgs{
		toHost:      "localhost",
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 2}},
		app:         appInstance,
		imageId:     imageId,
		provisioner: p,
	})
	c.Assert(err, check.IsNil)
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	appStruct := &app.App{
		Name: appInstance.GetName(),
	}
	err = conn.Apps().Insert(appStruct)
	c.Assert(err, check.IsNil)
	defer conn.Apps().Remove(bson.M{"name": appStruct.Name})
	buf := safe.NewBuffer(nil)
	err = p.moveContainers("localhost", "127.0.0.1", buf)
	c.Assert(err, check.IsNil)
	containers, err := p.listContainersByHost("localhost")
	c.Assert(len(containers), check.Equals, 0)
	containers, err = p.listContainersByHost("127.0.0.1")
	c.Assert(len(containers), check.Equals, 2)
	parts := strings.Split(buf.String(), "\n")
	c.Assert(parts[0], check.Matches, ".*Moving 2 units.*")
	c.Assert(parts[1], check.Matches, ".*Moving unit.*for.*myapp.*localhost.*127.0.0.1.*")
	c.Assert(parts[2], check.Matches, ".*Moving unit.*for.*myapp.*localhost.*127.0.0.1.*")
}

func (s *S) TestMoveContainersUnknownDest(c *check.C) {
	p, err := s.startMultipleServersCluster()
	c.Assert(err, check.IsNil)
	defer s.stopMultipleServersCluster(p)
	err = s.newFakeImage(p, "tsuru/app-myapp", nil)
	c.Assert(err, check.IsNil)
	appInstance := provisiontest.NewFakeApp("myapp", "python", 0)
	defer p.Destroy(appInstance)
	p.Provision(appInstance)
	coll := p.collection()
	defer coll.Close()
	coll.Insert(container{ID: "container-id", AppName: appInstance.GetName(), Version: "container-version", Image: "tsuru/python"})
	defer coll.RemoveAll(bson.M{"appname": appInstance.GetName()})
	imageId, err := appCurrentImageName(appInstance.GetName())
	c.Assert(err, check.IsNil)
	_, err = addContainersWithHost(&changeUnitsPipelineArgs{
		toHost:      "localhost",
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 2}},
		app:         appInstance,
		imageId:     imageId,
		provisioner: p,
	})
	c.Assert(err, check.IsNil)
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	appStruct := &app.App{
		Name: appInstance.GetName(),
	}
	err = conn.Apps().Insert(appStruct)
	c.Assert(err, check.IsNil)
	defer conn.Apps().Remove(bson.M{"name": appStruct.Name})
	buf := safe.NewBuffer(nil)
	err = p.moveContainers("localhost", "unknown", buf)
	c.Assert(err, check.Equals, containerMovementErr)
	parts := strings.Split(buf.String(), "\n")
	c.Assert(parts[0], check.Matches, ".*Moving 2 units.*")
	c.Assert(parts[3], check.Matches, "(?s).*Error moving unit.*Caused by:.*unknown.*not found")
	c.Assert(parts[4], check.Matches, "(?s).*Error moving unit.*Caused by:.*unknown.*not found")
}

func (s *S) TestMoveContainer(c *check.C) {
	p, err := s.startMultipleServersCluster()
	c.Assert(err, check.IsNil)
	defer s.stopMultipleServersCluster(p)
	err = s.newFakeImage(p, "tsuru/app-myapp", nil)
	c.Assert(err, check.IsNil)
	appInstance := provisiontest.NewFakeApp("myapp", "python", 0)
	defer p.Destroy(appInstance)
	p.Provision(appInstance)
	coll := p.collection()
	defer coll.Close()
	coll.Insert(container{ID: "container-id", AppName: appInstance.GetName(), Version: "container-version", Image: "tsuru/python"})
	defer coll.RemoveAll(bson.M{"appname": appInstance.GetName()})
	imageId, err := appCurrentImageName(appInstance.GetName())
	c.Assert(err, check.IsNil)
	addedConts, err := addContainersWithHost(&changeUnitsPipelineArgs{
		toHost:      "localhost",
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 2}},
		app:         appInstance,
		imageId:     imageId,
		provisioner: p,
	})
	c.Assert(err, check.IsNil)
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	appStruct := &app.App{
		Name: appInstance.GetName(),
	}
	err = conn.Apps().Insert(appStruct)
	c.Assert(err, check.IsNil)
	defer conn.Apps().Remove(bson.M{"name": appStruct.Name})
	buf := safe.NewBuffer(nil)
	var serviceBodies []string
	var serviceMethods []string
	rollback := s.addServiceInstance(c, appInstance.GetName(), []string{addedConts[0].ID}, func(w http.ResponseWriter, r *http.Request) {
		data, _ := ioutil.ReadAll(r.Body)
		serviceBodies = append(serviceBodies, string(data))
		serviceMethods = append(serviceMethods, r.Method)
		w.WriteHeader(http.StatusOK)
	})
	defer rollback()
	_, err = p.moveContainer(addedConts[0].ID[:6], "127.0.0.1", buf)
	c.Assert(err, check.IsNil)
	containers, err := p.listContainersByHost("localhost")
	c.Assert(len(containers), check.Equals, 1)
	containers, err = p.listContainersByHost("127.0.0.1")
	c.Assert(len(containers), check.Equals, 1)
	c.Assert(serviceBodies, check.HasLen, 2)
	c.Assert(serviceMethods, check.HasLen, 2)
	c.Assert(serviceMethods[0], check.Equals, "POST")
	c.Assert(serviceBodies[0], check.Matches, ".*unit-host=127.0.0.1")
	c.Assert(serviceMethods[1], check.Equals, "DELETE")
	c.Assert(serviceBodies[1], check.Matches, ".*unit-host=localhost")
}

func (s *S) TestRebalanceContainers(c *check.C) {
	p, err := s.startMultipleServersCluster()
	c.Assert(err, check.IsNil)
	defer s.stopMultipleServersCluster(p)
	err = s.newFakeImage(p, "tsuru/app-myapp", nil)
	c.Assert(err, check.IsNil)
	appInstance := provisiontest.NewFakeApp("myapp", "python", 0)
	defer p.Destroy(appInstance)
	p.Provision(appInstance)
	imageId, err := appCurrentImageName(appInstance.GetName())
	c.Assert(err, check.IsNil)
	_, err = addContainersWithHost(&changeUnitsPipelineArgs{
		toHost:      "localhost",
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 5}},
		app:         appInstance,
		imageId:     imageId,
		provisioner: p,
	})
	c.Assert(err, check.IsNil)
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	appStruct := &app.App{
		Name: appInstance.GetName(),
	}
	err = conn.Apps().Insert(appStruct)
	c.Assert(err, check.IsNil)
	defer conn.Apps().Remove(bson.M{"name": appStruct.Name})
	buf := safe.NewBuffer(nil)
	err = p.rebalanceContainers(buf, false)
	c.Assert(err, check.IsNil)
	c1, err := p.listContainersByHost("localhost")
	c.Assert(err, check.IsNil)
	c2, err := p.listContainersByHost("127.0.0.1")
	c.Assert(err, check.IsNil)
	c.Assert((len(c1) == 3 && len(c2) == 2) || (len(c1) == 2 && len(c2) == 3), check.Equals, true)
}

func (s *S) TestRebalanceContainersSegScheduler(c *check.C) {
	otherServer, err := dtesting.NewServer("localhost:0", nil, nil)
	c.Assert(err, check.IsNil)
	otherUrl := strings.Replace(otherServer.URL(), "127.0.0.1", "localhost", 1)
	p := &dockerProvisioner{}
	err = p.Initialize()
	c.Assert(err, check.IsNil)
	p.storage = &cluster.MapStorage{}
	p.scheduler = &segregatedScheduler{provisioner: p}
	p.cluster, err = cluster.New(p.scheduler, p.storage,
		cluster.Node{Address: s.server.URL(), Metadata: map[string]string{"pool": "pool1"}},
		cluster.Node{Address: otherUrl, Metadata: map[string]string{"pool": "pool1"}},
	)
	c.Assert(err, check.IsNil)
	err = provision.AddPool("pool1")
	c.Assert(err, check.IsNil)
	err = provision.AddTeamsToPool("pool1", []string{"team1"})
	c.Assert(err, check.IsNil)
	err = s.newFakeImage(p, "tsuru/app-myapp", nil)
	c.Assert(err, check.IsNil)
	appInstance := provisiontest.NewFakeApp("myapp", "python", 0)
	defer p.Destroy(appInstance)
	p.Provision(appInstance)
	imageId, err := appCurrentImageName(appInstance.GetName())
	c.Assert(err, check.IsNil)
	_, err = addContainersWithHost(&changeUnitsPipelineArgs{
		toHost:      "localhost",
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 5}},
		app:         appInstance,
		imageId:     imageId,
		provisioner: p,
	})
	c.Assert(err, check.IsNil)
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	appStruct := &app.App{
		Name:      appInstance.GetName(),
		TeamOwner: "team1",
		Pool:      "pool1",
	}
	err = conn.Apps().Insert(appStruct)
	c.Assert(err, check.IsNil)
	defer conn.Apps().Remove(bson.M{"name": appStruct.Name})
	c1, err := p.listContainersByHost("localhost")
	c.Assert(err, check.IsNil)
	c.Assert(c1, check.HasLen, 5)
	buf := safe.NewBuffer(nil)
	err = p.rebalanceContainers(buf, false)
	c.Assert(err, check.IsNil)
	c.Assert(p.scheduler.ignoredContainers, check.IsNil)
	c1, err = p.listContainersByHost("localhost")
	c.Assert(err, check.IsNil)
	c2, err := p.listContainersByHost("127.0.0.1")
	c.Assert(err, check.IsNil)
	c.Assert((len(c1) == 2 && len(c2) == 3) || (len(c1) == 3 && len(c2) == 2), check.Equals, true)
}

func (s *S) TestAppLocker(c *check.C) {
	appName := "myapp"
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	appDB := &app.App{Name: appName}
	defer conn.Apps().Remove(bson.M{"name": appName})
	err = conn.Apps().Insert(appDB)
	c.Assert(err, check.IsNil)
	locker := &appLocker{}
	hasLock := locker.lock(appName)
	c.Assert(hasLock, check.Equals, true)
	c.Assert(locker.refCount[appName], check.Equals, 1)
	appDB, err = app.GetByName(appName)
	c.Assert(err, check.IsNil)
	c.Assert(appDB.Lock.Locked, check.Equals, true)
	c.Assert(appDB.Lock.Owner, check.Equals, app.InternalAppName)
	c.Assert(appDB.Lock.Reason, check.Equals, "container-move")
	hasLock = locker.lock(appName)
	c.Assert(hasLock, check.Equals, true)
	c.Assert(locker.refCount[appName], check.Equals, 2)
	locker.unlock(appName)
	c.Assert(locker.refCount[appName], check.Equals, 1)
	appDB, err = app.GetByName(appName)
	c.Assert(err, check.IsNil)
	c.Assert(appDB.Lock.Locked, check.Equals, true)
	locker.unlock(appName)
	c.Assert(locker.refCount[appName], check.Equals, 0)
	appDB, err = app.GetByName(appName)
	c.Assert(err, check.IsNil)
	c.Assert(appDB.Lock.Locked, check.Equals, false)
}

func (s *S) TestAppLockerBlockOtherLockers(c *check.C) {
	appName := "myapp"
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	appDB := &app.App{Name: appName}
	defer conn.Apps().Remove(bson.M{"name": appName})
	err = conn.Apps().Insert(appDB)
	c.Assert(err, check.IsNil)
	locker := &appLocker{}
	hasLock := locker.lock(appName)
	c.Assert(hasLock, check.Equals, true)
	c.Assert(locker.refCount[appName], check.Equals, 1)
	appDB, err = app.GetByName(appName)
	c.Assert(err, check.IsNil)
	c.Assert(appDB.Lock.Locked, check.Equals, true)
	otherLocker := &appLocker{}
	hasLock = otherLocker.lock(appName)
	c.Assert(hasLock, check.Equals, false)
}

func (s *S) TestRebalanceContainersManyApps(c *check.C) {
	p, err := s.startMultipleServersCluster()
	c.Assert(err, check.IsNil)
	defer s.stopMultipleServersCluster(p)
	err = s.newFakeImage(p, "tsuru/app-myapp", nil)
	c.Assert(err, check.IsNil)
	err = s.newFakeImage(p, "tsuru/app-otherapp", nil)
	c.Assert(err, check.IsNil)
	appInstance := provisiontest.NewFakeApp("myapp", "python", 0)
	defer p.Destroy(appInstance)
	p.Provision(appInstance)
	appInstance2 := provisiontest.NewFakeApp("otherapp", "python", 0)
	defer p.Destroy(appInstance2)
	p.Provision(appInstance2)
	imageId, err := appCurrentImageName(appInstance.GetName())
	c.Assert(err, check.IsNil)
	_, err = addContainersWithHost(&changeUnitsPipelineArgs{
		toHost:      "localhost",
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 1}},
		app:         appInstance,
		imageId:     imageId,
		provisioner: p,
	})
	c.Assert(err, check.IsNil)
	imageId2, err := appCurrentImageName(appInstance2.GetName())
	c.Assert(err, check.IsNil)
	_, err = addContainersWithHost(&changeUnitsPipelineArgs{
		toHost:      "localhost",
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 1}},
		app:         appInstance2,
		imageId:     imageId2,
		provisioner: p,
	})
	c.Assert(err, check.IsNil)
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	appStruct := &app.App{
		Name: appInstance.GetName(),
	}
	err = conn.Apps().Insert(appStruct)
	c.Assert(err, check.IsNil)
	defer conn.Apps().Remove(bson.M{"name": appStruct.Name})
	appStruct2 := &app.App{
		Name: appInstance2.GetName(),
	}
	err = conn.Apps().Insert(appStruct2)
	c.Assert(err, check.IsNil)
	defer conn.Apps().Remove(bson.M{"name": appStruct2.Name})
	buf := safe.NewBuffer(nil)
	c1, err := p.listContainersByHost("localhost")
	c.Assert(len(c1), check.Equals, 2)
	err = p.rebalanceContainers(buf, false)
	c.Assert(err, check.IsNil)
	c1, err = p.listContainersByHost("localhost")
	c.Assert(len(c1), check.Equals, 1)
	c2, err := p.listContainersByHost("127.0.0.1")
	c.Assert(len(c2), check.Equals, 1)
}

func (s *S) TestRebalanceContainersDry(c *check.C) {
	p, err := s.startMultipleServersCluster()
	c.Assert(err, check.IsNil)
	defer s.stopMultipleServersCluster(p)
	err = s.newFakeImage(p, "tsuru/app-myapp", nil)
	c.Assert(err, check.IsNil)
	appInstance := provisiontest.NewFakeApp("myapp", "python", 0)
	defer p.Destroy(appInstance)
	p.Provision(appInstance)
	imageId, err := appCurrentImageName(appInstance.GetName())
	c.Assert(err, check.IsNil)
	args := changeUnitsPipelineArgs{
		app:         appInstance,
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 5}},
		imageId:     imageId,
		provisioner: p,
		toHost:      "localhost",
	}
	pipeline := action.NewPipeline(
		&provisionAddUnitsToHost,
		&bindAndHealthcheck,
		&addNewRoutes,
		&updateAppImage,
	)
	err = pipeline.Execute(args)
	c.Assert(err, check.IsNil)
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	appStruct := &app.App{
		Name: appInstance.GetName(),
	}
	err = conn.Apps().Insert(appStruct)
	c.Assert(err, check.IsNil)
	defer conn.Apps().Remove(bson.M{"name": appStruct.Name})
	router, err := getRouterForApp(appInstance)
	c.Assert(err, check.IsNil)
	beforeRoutes, err := router.Routes(appStruct.Name)
	c.Assert(err, check.IsNil)
	c.Assert(beforeRoutes, check.HasLen, 5)
	var serviceCalled bool
	rollback := s.addServiceInstance(c, appInstance.GetName(), nil, func(w http.ResponseWriter, r *http.Request) {
		serviceCalled = true
		w.WriteHeader(http.StatusOK)
	})
	defer rollback()
	buf := safe.NewBuffer(nil)
	err = p.rebalanceContainers(buf, true)
	c.Assert(err, check.IsNil)
	c1, err := p.listContainersByHost("localhost")
	c.Assert(err, check.IsNil)
	c2, err := p.listContainersByHost("127.0.0.1")
	c.Assert(err, check.IsNil)
	c.Assert(len(c1), check.Equals, 5)
	c.Assert(len(c2), check.Equals, 0)
	routes, err := router.Routes(appStruct.Name)
	c.Assert(err, check.IsNil)
	c.Assert(routes, check.DeepEquals, beforeRoutes)
	c.Assert(serviceCalled, check.Equals, false)
}
