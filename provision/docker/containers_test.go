// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"io/ioutil"
	"net/http"
	"regexp"
	"strings"

	dtesting "github.com/fsouza/go-dockerclient/testing"
	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/app/image"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/docker/container"
	"github.com/tsuru/tsuru/provision/docker/types"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/router"
	"github.com/tsuru/tsuru/safe"
	"gopkg.in/check.v1"
)

func (s *S) TestMoveContainers(c *check.C) {
	p, err := s.startMultipleServersCluster()
	c.Assert(err, check.IsNil)
	err = newFakeImage(p, "tsuru/app-myapp", nil)
	c.Assert(err, check.IsNil)
	appInstance := provisiontest.NewFakeApp("myapp", "python", "test-default", 0)
	defer p.Destroy(appInstance)
	p.Provision(appInstance)
	coll := p.Collection()
	defer coll.Close()
	coll.Insert(container.Container{
		Container: types.Container{
			ID:      "container-id",
			AppName: appInstance.GetName(),
			Version: "container-version",
			Image:   "tsuru/python",
		},
	})
	defer coll.RemoveAll(bson.M{"appname": appInstance.GetName()})
	imageID, err := image.AppCurrentImageName(appInstance.GetName())
	c.Assert(err, check.IsNil)
	_, err = addContainersWithHost(&changeUnitsPipelineArgs{
		toHost:      "localhost",
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 2}},
		app:         appInstance,
		imageID:     imageID,
		provisioner: p,
	})
	c.Assert(err, check.IsNil)
	appStruct := s.newAppFromFake(appInstance)
	err = s.conn.Apps().Insert(appStruct)
	c.Assert(err, check.IsNil)
	buf := safe.NewBuffer(nil)
	err = p.MoveContainers("localhost", "127.0.0.1", buf)
	c.Assert(err, check.IsNil)
	containers, err := p.listContainersByHost("localhost")
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 0)
	containers, err = p.listContainersByHost("127.0.0.1")
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 2)
	parts := strings.Split(buf.String(), "\n")
	c.Assert(parts[0], check.Matches, ".*Moving 2 units.*")
	var matches int
	movingRegexp := regexp.MustCompile(`.*Moving unit.*for.*myapp.*localhost.*127.0.0.1.*`)
	for _, line := range parts[1:] {
		if movingRegexp.MatchString(line) {
			matches++
		}
	}
	c.Assert(matches, check.Equals, 2)
}

func (s *S) TestMoveContainersUnknownDest(c *check.C) {
	p, err := s.startMultipleServersCluster()
	c.Assert(err, check.IsNil)
	err = newFakeImage(p, "tsuru/app-myapp", nil)
	c.Assert(err, check.IsNil)
	appInstance := provisiontest.NewFakeApp("myapp", "python", "test-default", 0)
	defer p.Destroy(appInstance)
	p.Provision(appInstance)
	coll := p.Collection()
	defer coll.Close()
	coll.Insert(container.Container{Container: types.Container{ID: "container-id", AppName: appInstance.GetName(), Version: "container-version", Image: "tsuru/python"}})
	defer coll.RemoveAll(bson.M{"appname": appInstance.GetName()})
	imageID, err := image.AppCurrentImageName(appInstance.GetName())
	c.Assert(err, check.IsNil)
	_, err = addContainersWithHost(&changeUnitsPipelineArgs{
		toHost:      "localhost",
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 2}},
		app:         appInstance,
		imageID:     imageID,
		provisioner: p,
	})
	c.Assert(err, check.IsNil)
	appStruct := s.newAppFromFake(appInstance)
	err = s.conn.Apps().Insert(appStruct)
	c.Assert(err, check.IsNil)
	buf := safe.NewBuffer(nil)
	err = p.MoveContainers("localhost", "unknown", buf)
	multiErr := err.(*tsuruErrors.MultiError)
	c.Assert(multiErr.Len(), check.Equals, 2)
	parts := strings.Split(buf.String(), "\n")
	c.Assert(parts, check.HasLen, 6)
	c.Assert(parts[0], check.Matches, ".*Moving 2 units.*")
	var matches int
	errorRegexp := regexp.MustCompile(`(?s).*Error moving unit.*Caused by:.*unknown.*not found`)
	for _, line := range parts[2:] {
		if errorRegexp.MatchString(line) {
			matches++
		}
	}
	c.Assert(matches, check.Equals, 2)
}

func (s *S) TestMoveContainer(c *check.C) {
	p, err := s.startMultipleServersCluster()
	c.Assert(err, check.IsNil)
	err = newFakeImage(p, "tsuru/app-myapp", nil)
	c.Assert(err, check.IsNil)
	appInstance := provisiontest.NewFakeApp("myapp", "python", "test-default", 0)
	defer p.Destroy(appInstance)
	p.Provision(appInstance)
	coll := p.Collection()
	defer coll.Close()
	coll.Insert(container.Container{
		Container: types.Container{
			ID:      "container-id",
			AppName: appInstance.GetName(),
			Version: "container-version",
			Image:   "tsuru/python",
		},
	})
	defer coll.RemoveAll(bson.M{"appname": appInstance.GetName()})
	imageID, err := image.AppCurrentImageName(appInstance.GetName())
	c.Assert(err, check.IsNil)
	addedConts, err := addContainersWithHost(&changeUnitsPipelineArgs{
		toHost:      "localhost",
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 2}},
		app:         appInstance,
		imageID:     imageID,
		provisioner: p,
	})
	c.Assert(err, check.IsNil)
	appStruct := s.newAppFromFake(appInstance)
	err = s.conn.Apps().Insert(appStruct)
	c.Assert(err, check.IsNil)
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
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 1)
	containers, err = p.listContainersByHost("127.0.0.1")
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 1)
	c.Assert(serviceBodies, check.HasLen, 2)
	c.Assert(serviceMethods, check.HasLen, 2)
	c.Assert(serviceMethods[0], check.Equals, "POST")
	c.Assert(serviceBodies[0], check.Matches, ".*unit-host=127.0.0.1")
	c.Assert(serviceMethods[1], check.Equals, "DELETE")
	c.Assert(serviceBodies[1], check.Matches, ".*unit-host=localhost")
}

func (s *S) TestMoveContainerStopped(c *check.C) {
	p, err := s.startMultipleServersCluster()
	c.Assert(err, check.IsNil)
	err = newFakeImage(p, "tsuru/app-myapp", nil)
	c.Assert(err, check.IsNil)
	appInstance := provisiontest.NewFakeApp("myapp", "python", "test-default", 0)
	p.Provision(appInstance)
	imageID, err := image.AppCurrentImageName(appInstance.GetName())
	c.Assert(err, check.IsNil)
	addedConts, err := addContainersWithHost(&changeUnitsPipelineArgs{
		toHost:      "localhost",
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 2, Status: provision.StatusStopped}},
		app:         appInstance,
		imageID:     imageID,
		provisioner: p,
	})
	c.Assert(err, check.IsNil)
	appStruct := s.newAppFromFake(appInstance)
	err = s.conn.Apps().Insert(appStruct)
	c.Assert(err, check.IsNil)
	buf := safe.NewBuffer(nil)
	_, err = p.moveContainer(addedConts[0].ID[:6], "127.0.0.1", buf)
	c.Assert(err, check.IsNil)
	containers, err := p.listContainersByHost("localhost")
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 1)
	containers, err = p.listContainersByHost("127.0.0.1")
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 1)
	c.Assert(containers[0].Status, check.Equals, provision.StatusStopped.String())
}

func (s *S) TestMoveContainerErrorStopped(c *check.C) {
	p, err := s.startMultipleServersCluster()
	c.Assert(err, check.IsNil)
	err = newFakeImage(p, "tsuru/app-myapp", nil)
	c.Assert(err, check.IsNil)
	appInstance := provisiontest.NewFakeApp("myapp", "python", "test-default", 0)
	p.Provision(appInstance)
	imageID, err := image.AppCurrentImageName(appInstance.GetName())
	c.Assert(err, check.IsNil)
	addedConts, err := addContainersWithHost(&changeUnitsPipelineArgs{
		toHost:      "localhost",
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 2, Status: provision.StatusStopped}},
		app:         appInstance,
		imageID:     imageID,
		provisioner: p,
	})
	c.Assert(err, check.IsNil)
	err = addedConts[0].SetStatus(p.ClusterClient(), provision.StatusError, true)
	c.Assert(err, check.IsNil)
	appStruct := s.newAppFromFake(appInstance)
	err = s.conn.Apps().Insert(appStruct)
	c.Assert(err, check.IsNil)
	buf := safe.NewBuffer(nil)
	_, err = p.moveContainer(addedConts[0].ID[:6], "127.0.0.1", buf)
	c.Assert(err, check.IsNil)
	containers, err := p.listContainersByHost("localhost")
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 1)
	containers, err = p.listContainersByHost("127.0.0.1")
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 1)
	c.Assert(containers[0].Status, check.Equals, provision.StatusStopped.String())
}

func (s *S) TestMoveContainerErrorStarted(c *check.C) {
	p, err := s.startMultipleServersCluster()
	c.Assert(err, check.IsNil)
	err = newFakeImage(p, "tsuru/app-myapp", nil)
	c.Assert(err, check.IsNil)
	appInstance := provisiontest.NewFakeApp("myapp", "python", "test-default", 0)
	p.Provision(appInstance)
	imageID, err := image.AppCurrentImageName(appInstance.GetName())
	c.Assert(err, check.IsNil)
	addedConts, err := addContainersWithHost(&changeUnitsPipelineArgs{
		toHost:      "localhost",
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 2}},
		app:         appInstance,
		imageID:     imageID,
		provisioner: p,
	})
	c.Assert(err, check.IsNil)
	err = addedConts[0].SetStatus(p.ClusterClient(), provision.StatusError, true)
	c.Assert(err, check.IsNil)
	appStruct := s.newAppFromFake(appInstance)
	err = s.conn.Apps().Insert(appStruct)
	c.Assert(err, check.IsNil)
	buf := safe.NewBuffer(nil)
	_, err = p.moveContainer(addedConts[0].ID[:6], "127.0.0.1", buf)
	c.Assert(err, check.IsNil)
	containers, err := p.listContainersByHost("localhost")
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 1)
	containers, err = p.listContainersByHost("127.0.0.1")
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 1)
	c.Assert(containers[0].Status, check.Equals, provision.StatusStarting.String())
}

func (s *S) TestRebalanceContainers(c *check.C) {
	p, err := s.startMultipleServersCluster()
	c.Assert(err, check.IsNil)
	err = newFakeImage(p, "tsuru/app-myapp", nil)
	c.Assert(err, check.IsNil)
	appInstance := provisiontest.NewFakeApp("myapp", "python", "test-default", 0)
	defer p.Destroy(appInstance)
	p.Provision(appInstance)
	imageID, err := image.AppCurrentImageName(appInstance.GetName())
	c.Assert(err, check.IsNil)
	_, err = addContainersWithHost(&changeUnitsPipelineArgs{
		toHost:      "localhost",
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 5}},
		app:         appInstance,
		imageID:     imageID,
		provisioner: p,
	})
	c.Assert(err, check.IsNil)
	appStruct := s.newAppFromFake(appInstance)
	appStruct.Pool = "test-default"
	err = s.conn.Apps().Insert(appStruct)
	c.Assert(err, check.IsNil)
	buf := safe.NewBuffer(nil)
	_, err = p.rebalanceContainersByFilter(buf, nil, nil, false)
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
	defer otherServer.Stop()
	otherURL := strings.Replace(otherServer.URL(), "127.0.0.1", "localhost", 1)
	p := &dockerProvisioner{}
	err = p.Initialize()
	c.Assert(err, check.IsNil)
	p.storage = &cluster.MapStorage{}
	p.scheduler = &segregatedScheduler{provisioner: p}
	p.cluster, err = cluster.New(p.scheduler, p.storage, "",
		cluster.Node{Address: s.server.URL(), Metadata: map[string]string{"pool": "pool1"}},
		cluster.Node{Address: otherURL, Metadata: map[string]string{"pool": "pool1"}},
	)
	c.Assert(err, check.IsNil)
	opts := pool.AddPoolOptions{Name: "pool1"}
	err = pool.AddPool(opts)
	c.Assert(err, check.IsNil)
	err = pool.AddTeamsToPool("pool1", []string{"team1"})
	c.Assert(err, check.IsNil)
	err = newFakeImage(p, "tsuru/app-myapp", nil)
	c.Assert(err, check.IsNil)
	appInstance := provisiontest.NewFakeApp("myapp", "python", "test-default", 0)
	defer p.Destroy(appInstance)
	p.Provision(appInstance)
	imageID, err := image.AppCurrentImageName(appInstance.GetName())
	c.Assert(err, check.IsNil)
	_, err = addContainersWithHost(&changeUnitsPipelineArgs{
		toHost:      "localhost",
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 5}},
		app:         appInstance,
		imageID:     imageID,
		provisioner: p,
	})
	c.Assert(err, check.IsNil)
	appStruct := s.newAppFromFake(appInstance)
	appStruct.TeamOwner = "team1"
	appStruct.Pool = "pool1"
	err = s.conn.Apps().Insert(appStruct)
	c.Assert(err, check.IsNil)
	c1, err := p.listContainersByHost("localhost")
	c.Assert(err, check.IsNil)
	c.Assert(c1, check.HasLen, 5)
	buf := safe.NewBuffer(nil)
	_, err = p.rebalanceContainersByFilter(buf, nil, nil, false)
	c.Assert(err, check.IsNil)
	c.Assert(p.scheduler.ignoredContainers, check.IsNil)
	c1, err = p.listContainersByHost("localhost")
	c.Assert(err, check.IsNil)
	c2, err := p.listContainersByHost("127.0.0.1")
	c.Assert(err, check.IsNil)
	c.Assert((len(c1) == 2 && len(c2) == 3) || (len(c1) == 3 && len(c2) == 2), check.Equals, true)
}

func (s *S) TestRebalanceContainersByHost(c *check.C) {
	otherServer, err := dtesting.NewServer("localhost:0", nil, nil)
	c.Assert(err, check.IsNil)
	defer otherServer.Stop()
	otherURL := strings.Replace(otherServer.URL(), "127.0.0.1", "localhost", 1)
	p := &dockerProvisioner{}
	err = p.Initialize()
	c.Assert(err, check.IsNil)
	p.storage = &cluster.MapStorage{}
	p.scheduler = &segregatedScheduler{provisioner: p}
	p.cluster, err = cluster.New(p.scheduler, p.storage, "",
		cluster.Node{Address: s.server.URL(), Metadata: map[string]string{"pool": "pool1"}},
		cluster.Node{Address: otherURL, Metadata: map[string]string{"pool": "pool1"}},
	)
	c.Assert(err, check.IsNil)
	opts := pool.AddPoolOptions{Name: "pool1"}
	err = pool.AddPool(opts)
	c.Assert(err, check.IsNil)
	err = pool.AddTeamsToPool("pool1", []string{"team1"})
	c.Assert(err, check.IsNil)
	err = newFakeImage(p, "tsuru/app-myapp", nil)
	c.Assert(err, check.IsNil)
	appInstance := provisiontest.NewFakeApp("myapp", "python", "test-default", 0)
	defer p.Destroy(appInstance)
	p.Provision(appInstance)
	imageID, err := image.AppCurrentImageName(appInstance.GetName())
	c.Assert(err, check.IsNil)
	_, err = addContainersWithHost(&changeUnitsPipelineArgs{
		toHost:      "localhost",
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 5}},
		app:         appInstance,
		imageID:     imageID,
		provisioner: p,
	})
	c.Assert(err, check.IsNil)
	appStruct := s.newAppFromFake(appInstance)
	appStruct.TeamOwner = "team1"
	appStruct.Pool = "pool1"
	err = s.conn.Apps().Insert(appStruct)
	c.Assert(err, check.IsNil)
	c1, err := p.listContainersByHost("localhost")
	c.Assert(err, check.IsNil)
	c.Assert(c1, check.HasLen, 5)
	c2, err := p.listContainersByHost("127.0.0.1")
	c.Assert(err, check.IsNil)
	c.Assert(c2, check.HasLen, 0)
	err = p.Cluster().Unregister(otherURL)
	c.Assert(err, check.IsNil)
	buf := safe.NewBuffer(nil)
	err = p.rebalanceContainersByHost(net.URLToHost(otherURL), buf)
	c.Assert(err, check.IsNil)
	c.Assert(p.scheduler.ignoredContainers, check.IsNil)
	c2, err = p.listContainersByHost("127.0.0.1")
	c.Assert(err, check.IsNil)
	c.Assert(c2, check.HasLen, 5)
}

func (s *S) TestAppLocker(c *check.C) {
	appName := "myapp"
	appDB := &app.App{Name: appName}
	err := s.conn.Apps().Insert(appDB)
	c.Assert(err, check.IsNil)
	locker := &appLocker{}
	hasLock := locker.Lock(appName)
	c.Assert(hasLock, check.Equals, true)
	c.Assert(locker.refCount[appName], check.Equals, 1)
	appDB, err = app.GetByName(appName)
	c.Assert(err, check.IsNil)
	c.Assert(appDB.Lock.Locked, check.Equals, true)
	c.Assert(appDB.Lock.Owner, check.Equals, app.InternalAppName)
	c.Assert(appDB.Lock.Reason, check.Equals, "container-move")
	hasLock = locker.Lock(appName)
	c.Assert(hasLock, check.Equals, true)
	c.Assert(locker.refCount[appName], check.Equals, 2)
	locker.Unlock(appName)
	c.Assert(locker.refCount[appName], check.Equals, 1)
	appDB, err = app.GetByName(appName)
	c.Assert(err, check.IsNil)
	c.Assert(appDB.Lock.Locked, check.Equals, true)
	locker.Unlock(appName)
	c.Assert(locker.refCount[appName], check.Equals, 0)
	appDB, err = app.GetByName(appName)
	c.Assert(err, check.IsNil)
	c.Assert(appDB.Lock.Locked, check.Equals, false)
}

func (s *S) TestAppLockerBlockOtherLockers(c *check.C) {
	appName := "myapp"
	appDB := &app.App{Name: appName}
	err := s.conn.Apps().Insert(appDB)
	c.Assert(err, check.IsNil)
	locker := &appLocker{}
	hasLock := locker.Lock(appName)
	c.Assert(hasLock, check.Equals, true)
	c.Assert(locker.refCount[appName], check.Equals, 1)
	appDB, err = app.GetByName(appName)
	c.Assert(err, check.IsNil)
	c.Assert(appDB.Lock.Locked, check.Equals, true)
	otherLocker := &appLocker{}
	hasLock = otherLocker.Lock(appName)
	c.Assert(hasLock, check.Equals, false)
}

func (s *S) TestRebalanceContainersManyApps(c *check.C) {
	p, err := s.startMultipleServersCluster()
	c.Assert(err, check.IsNil)
	err = newFakeImage(p, "tsuru/app-myapp", nil)
	c.Assert(err, check.IsNil)
	err = newFakeImage(p, "tsuru/app-otherapp", nil)
	c.Assert(err, check.IsNil)
	appInstance := provisiontest.NewFakeApp("myapp", "python", "test-default", 0)
	defer p.Destroy(appInstance)
	p.Provision(appInstance)
	appInstance2 := provisiontest.NewFakeApp("otherapp", "python", "test-default", 0)
	defer p.Destroy(appInstance2)
	p.Provision(appInstance2)
	imageID, err := image.AppCurrentImageName(appInstance.GetName())
	c.Assert(err, check.IsNil)
	_, err = addContainersWithHost(&changeUnitsPipelineArgs{
		toHost:      "localhost",
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 1}},
		app:         appInstance,
		imageID:     imageID,
		provisioner: p,
	})
	c.Assert(err, check.IsNil)
	imageID2, err := image.AppCurrentImageName(appInstance2.GetName())
	c.Assert(err, check.IsNil)
	_, err = addContainersWithHost(&changeUnitsPipelineArgs{
		toHost:      "localhost",
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 1}},
		app:         appInstance2,
		imageID:     imageID2,
		provisioner: p,
	})
	c.Assert(err, check.IsNil)
	appStruct := s.newAppFromFake(appInstance)
	appStruct.Pool = "test-default"
	err = s.conn.Apps().Insert(appStruct)
	c.Assert(err, check.IsNil)
	appStruct2 := s.newAppFromFake(appInstance2)
	appStruct2.Pool = "test-default"
	err = s.conn.Apps().Insert(appStruct2)
	c.Assert(err, check.IsNil)
	buf := safe.NewBuffer(nil)
	c1, err := p.listContainersByHost("localhost")
	c.Assert(err, check.IsNil)
	c.Assert(c1, check.HasLen, 2)
	_, err = p.rebalanceContainersByFilter(buf, nil, nil, false)
	c.Assert(err, check.IsNil)
	c1, err = p.listContainersByHost("localhost")
	c.Assert(err, check.IsNil)
	c.Assert(c1, check.HasLen, 1)
	c2, err := p.listContainersByHost("127.0.0.1")
	c.Assert(err, check.IsNil)
	c.Assert(c2, check.HasLen, 1)
}

func (s *S) TestRebalanceContainersDry(c *check.C) {
	p, err := s.startMultipleServersCluster()
	c.Assert(err, check.IsNil)
	err = newFakeImage(p, "tsuru/app-myapp", nil)
	c.Assert(err, check.IsNil)
	appInstance := provisiontest.NewFakeApp("myapp", "python", "test-default", 0)
	defer p.Destroy(appInstance)
	p.Provision(appInstance)
	imageID, err := image.AppCurrentImageName(appInstance.GetName())
	c.Assert(err, check.IsNil)
	args := changeUnitsPipelineArgs{
		app:         appInstance,
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 5}},
		imageID:     imageID,
		provisioner: p,
		toHost:      "localhost",
	}
	pipeline := action.NewPipeline(
		&provisionAddUnitsToHost,
		&bindAndHealthcheck,
		&addNewRoutes,
		&setRouterHealthcheck,
		&updateAppImage,
	)
	err = pipeline.Execute(args)
	c.Assert(err, check.IsNil)
	appStruct := s.newAppFromFake(appInstance)
	appStruct.Pool = "test-default"
	err = s.conn.Apps().Insert(appStruct)
	c.Assert(err, check.IsNil)
	routers := appInstance.GetRouters()
	r, err := router.Get(routers[0].Name)
	c.Assert(err, check.IsNil)
	beforeRoutes, err := r.Routes(appStruct.Name)
	c.Assert(err, check.IsNil)
	c.Assert(beforeRoutes, check.HasLen, 5)
	var serviceCalled bool
	rollback := s.addServiceInstance(c, appInstance.GetName(), nil, func(w http.ResponseWriter, r *http.Request) {
		serviceCalled = true
		w.WriteHeader(http.StatusOK)
	})
	defer rollback()
	buf := safe.NewBuffer(nil)
	_, err = p.rebalanceContainersByFilter(buf, nil, nil, true)
	c.Assert(err, check.IsNil)
	c1, err := p.listContainersByHost("localhost")
	c.Assert(err, check.IsNil)
	c2, err := p.listContainersByHost("127.0.0.1")
	c.Assert(err, check.IsNil)
	c.Assert(c1, check.HasLen, 5)
	c.Assert(c2, check.HasLen, 0)
	routes, err := r.Routes(appStruct.Name)
	c.Assert(err, check.IsNil)
	c.Assert(routes, check.DeepEquals, beforeRoutes)
	c.Assert(serviceCalled, check.Equals, false)
}
