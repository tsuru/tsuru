// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"sort"

	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/docker/container"
	"github.com/tsuru/tsuru/provision/docker/types"
	check "gopkg.in/check.v1"
)

func (s *S) getContainerCollection(appName string, containerIds ...string) func() {
	coll := s.p.Collection()
	for _, containerId := range containerIds {
		container := container.Container{Container: types.Container{AppName: appName, ID: containerId}}
		coll.Insert(container)
	}
	return func() {
		for _, containerId := range containerIds {
			coll.Remove(bson.M{"id": containerId})
		}
		coll.Close()
	}
}

func (s *S) TestListContainersByApp(c *check.C) {
	var result []container.Container
	coll := s.p.Collection()
	defer coll.Close()
	coll.Insert(
		container.Container{Container: types.Container{ID: "Hey", Type: "python", AppName: "myapp", HostAddr: "http://cittavld1180.globoi.com"}},
		container.Container{Container: types.Container{ID: "Ho", Type: "python", AppName: "myapp", HostAddr: "http://cittavld1182.globoi.com"}},
		container.Container{Container: types.Container{ID: "Let's Go", Type: "java", AppName: "other", HostAddr: "http://cittavld597.globoi.com"}},
	)
	defer coll.RemoveAll(bson.M{"appname": "myapp"})
	result, err := s.p.listContainersByApp("myapp")
	c.Assert(err, check.IsNil)
	c.Assert(result, check.HasLen, 2)
	cond := (result[0].ID == "Hey" && result[1].ID == "Ho") || (result[0].ID == "Ho" && result[1].ID == "Hey")
	c.Assert(cond, check.Equals, true)
}

func (s *S) TestListContainersByProcess(c *check.C) {
	var result []container.Container
	coll := s.p.Collection()
	defer coll.Close()
	coll.Insert(
		container.Container{Container: types.Container{ID: "Hey", Type: "python", AppName: "myapp", HostAddr: "http://cittavld1180.globoi.com", ProcessName: "web"}},
		container.Container{Container: types.Container{ID: "Ho", Type: "python", AppName: "myapp", HostAddr: "http://cittavld1182.globoi.com", ProcessName: "worker"}},
		container.Container{Container: types.Container{ID: "Let's Go", Type: "java", AppName: "other", HostAddr: "http://cittavld597.globoi.com"}},
	)
	defer coll.RemoveAll(bson.M{"appname": "myapp"})
	result, err := s.p.listContainersByProcess("myapp", "web")
	c.Assert(err, check.IsNil)
	c.Assert(result, check.HasLen, 1)
	c.Assert(result[0].ID, check.Equals, "Hey")
}

func (s *S) TestListContainersByEmptyProcess(c *check.C) {
	var result []container.Container
	coll := s.p.Collection()
	defer coll.Close()
	coll.Insert(
		container.Container{Container: types.Container{ID: "Hey", Type: "python", AppName: "myapp", HostAddr: "http://cittavld1180.globoi.com", ProcessName: "web"}},
		container.Container{Container: types.Container{ID: "Ho", Type: "python", AppName: "myapp", HostAddr: "http://cittavld1182.globoi.com", ProcessName: "worker"}},
		container.Container{Container: types.Container{ID: "Let's Go", Type: "java", AppName: "other", HostAddr: "http://cittavld597.globoi.com"}},
	)
	defer coll.RemoveAll(bson.M{"appname": "myapp"})
	result, err := s.p.listContainersByProcess("myapp", "")
	c.Assert(err, check.IsNil)
	c.Assert(result, check.HasLen, 2)
	cond := (result[0].ID == "Hey" && result[1].ID == "Ho") || (result[0].ID == "Ho" && result[1].ID == "Hey")
	c.Assert(cond, check.Equals, true)
}

type containerByIdList []container.Container

func (l containerByIdList) Len() int           { return len(l) }
func (l containerByIdList) Swap(i, j int)      { l[i], l[j] = l[j], l[i] }
func (l containerByIdList) Less(i, j int) bool { return l[i].ID < l[j].ID }

func stripMongoID(conts []container.Container) []container.Container {
	for i := range conts {
		conts[i].MongoID = bson.ObjectId("")
	}
	return conts
}

func (s *S) TestListContainersByAppAndHost(c *check.C) {
	var result []container.Container
	coll := s.p.Collection()
	defer coll.Close()
	coll.Insert(
		container.Container{Container: types.Container{ID: "1", AppName: "myapp1", HostAddr: "host1"}},
		container.Container{Container: types.Container{ID: "2", AppName: "myapp2", HostAddr: "host2"}},
		container.Container{Container: types.Container{ID: "3", AppName: "other", HostAddr: "host3"}},
	)
	result, err := s.p.listContainersByAppAndHost([]string{"myapp1", "myapp2"}, nil)
	c.Assert(err, check.IsNil)
	sort.Sort(containerByIdList(result))
	c.Assert(stripMongoID(result), check.DeepEquals, []container.Container{
		{Container: types.Container{ID: "1", AppName: "myapp1", HostAddr: "host1"}},
		{Container: types.Container{ID: "2", AppName: "myapp2", HostAddr: "host2"}},
	})
	result, err = s.p.listContainersByAppAndHost(nil, nil)
	c.Assert(err, check.IsNil)
	sort.Sort(containerByIdList(result))
	c.Assert(stripMongoID(result), check.DeepEquals, []container.Container{
		{Container: types.Container{ID: "1", AppName: "myapp1", HostAddr: "host1"}},
		{Container: types.Container{ID: "2", AppName: "myapp2", HostAddr: "host2"}},
		{Container: types.Container{ID: "3", AppName: "other", HostAddr: "host3"}},
	})
	result, err = s.p.listContainersByAppAndHost(nil, []string{"host2", "host3"})
	c.Assert(err, check.IsNil)
	sort.Sort(containerByIdList(result))
	c.Assert(stripMongoID(result), check.DeepEquals, []container.Container{
		{Container: types.Container{ID: "2", AppName: "myapp2", HostAddr: "host2"}},
		{Container: types.Container{ID: "3", AppName: "other", HostAddr: "host3"}},
	})
	result, err = s.p.listContainersByAppAndHost([]string{"myapp1"}, []string{"host2"})
	c.Assert(err, check.IsNil)
	sort.Sort(containerByIdList(result))
	c.Assert(stripMongoID(result), check.IsNil)
	result, err = s.p.listContainersByAppAndHost([]string{"myapp1", "myapp2"}, []string{"host2", "host3"})
	c.Assert(err, check.IsNil)
	sort.Sort(containerByIdList(result))
	c.Assert(stripMongoID(result), check.DeepEquals, []container.Container{
		{Container: types.Container{ID: "2", AppName: "myapp2", HostAddr: "host2"}},
	})
}

func (s *S) TestListContainersByHost(c *check.C) {
	var result []container.Container
	coll := s.p.Collection()
	defer coll.Close()
	coll.Insert(
		container.Container{Container: types.Container{ID: "1", Type: "python", AppName: "myapp", HostAddr: "http://cittavld1182.globoi.com"}},
		container.Container{Container: types.Container{ID: "2", Type: "python", AppName: "wat", HostAddr: "http://cittavld1182.globoi.com"}},
		container.Container{Container: types.Container{ID: "3", Type: "java", AppName: "masoq", HostAddr: "http://cittavld9999.globoi.com"}},
	)
	defer coll.RemoveAll(bson.M{"hostaddr": "http://cittavld1182.globoi.com"})
	result, err := s.p.listContainersByHost("http://cittavld1182.globoi.com")
	c.Assert(err, check.IsNil)
	c.Assert(result, check.HasLen, 2)
	cond := (result[0].ID == "1" && result[1].ID == "2") || (result[0].ID == "2" && result[1].ID == "1")
	c.Assert(cond, check.Equals, true)
}

func (s *S) TestListAllContainers(c *check.C) {
	appName := "some-app"
	containerIds := []string{"some-container-1", "some-container-2"}
	cleanupFunc := s.getContainerCollection(appName, containerIds...)
	defer cleanupFunc()
	containers, err := s.p.listAllContainers()
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 2)
	cond := (containers[0].ID == containerIds[0] && containers[1].ID == containerIds[1]) ||
		(containers[0].ID == containerIds[1] && containers[1].ID == containerIds[0])
	c.Assert(cond, check.Equals, true)
}

func (s *S) TestUpdateContainers(c *check.C) {
	appName := "myapp"
	containerIds := []string{"some-container-1", "some-container-2", "some-container-3"}
	cleanupFunc := s.getContainerCollection(appName, containerIds...)
	defer cleanupFunc()
	err := s.p.updateContainers(bson.M{"appname": "myapp"}, bson.M{"$set": bson.M{"appname": "yourapp"}})
	c.Assert(err, check.IsNil)
	containers, err := s.p.listAllContainers()
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 3)
	ids := make([]string, len(containers))
	names := make([]string, len(containers))
	for i, container := range containers {
		ids[i] = container.ID
		names[i] = container.AppName
	}
	sort.Strings(ids)
	c.Assert(ids, check.DeepEquals, containerIds)
	c.Assert(names, check.DeepEquals, []string{"yourapp", "yourapp", "yourapp"})
}

func (s *S) TestGetOneContainerByAppName(c *check.C) {
	appName := "some-app"
	containerIds := []string{"some-container-1", "some-container-2"}
	cleanupFunc := s.getContainerCollection(appName, containerIds...)
	defer cleanupFunc()
	container, err := s.p.getOneContainerByAppName(appName)
	c.Assert(err, check.IsNil)
	c.Assert(container.AppName, check.Equals, appName)
	checkId := container.ID == containerIds[0] || container.ID == containerIds[1]
	c.Assert(checkId, check.Equals, true)
}

func (s *S) TestShouldNotGetOneContainerByAppName(c *check.C) {
	coll := s.p.Collection()
	defer coll.Close()
	container, err := s.p.getOneContainerByAppName("unexisting-app-name")
	c.Assert(err, check.NotNil)
	c.Assert(container, check.IsNil)
}

func (s *S) TestGetContainerCountForAppName(c *check.C) {
	appName := "some-app"
	containerIds := []string{"some-container-1", "some-container-2"}
	cleanupFunc := s.getContainerCollection(appName, containerIds...)
	defer cleanupFunc()
	count, err := s.p.getContainerCountForAppName(appName)
	c.Assert(err, check.IsNil)
	c.Assert(count, check.Equals, len(containerIds))
}

func (s *S) TestGetContainerPartialIdAmbiguous(c *check.C) {
	containerIds := []string{"container-1", "container-2"}
	cleanupFunc := s.getContainerCollection("some-app", containerIds...)
	defer cleanupFunc()
	_, err := s.p.GetContainer("container")
	c.Assert(err, check.NotNil)
	e, ok := err.(*AmbiguousContainerError)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.ID, check.Equals, "container")
	c.Assert(err.Error(), check.Equals, `ambiguous container name/id: "container"`)
}

func (s *S) TestGetContainerPartialIdNotFound(c *check.C) {
	containerIds := []string{"container-1", "container-2"}
	cleanupFunc := s.getContainerCollection("some-app", containerIds...)
	defer cleanupFunc()
	_, err := s.p.GetContainer("container-9")
	c.Assert(err, check.NotNil)
	e, ok := err.(*provision.UnitNotFoundError)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.ID, check.Equals, "container-9")
}

func (s *S) TestGetContainerPartialId(c *check.C) {
	containerIds := []string{"container-a1", "container-b2"}
	cleanupFunc := s.getContainerCollection("some-app", containerIds...)
	defer cleanupFunc()
	cont, err := s.p.GetContainer("container-a")
	c.Assert(err, check.IsNil)
	c.Assert(cont.ID, check.Equals, "container-a1")
}

func (s *S) TestListRunnableContainersByApp(c *check.C) {
	var result []container.Container
	coll := s.p.Collection()
	defer coll.Close()
	coll.Insert(
		container.Container{Container: types.Container{Name: "a", AppName: "myapp", Status: provision.StatusCreated.String()}},
		container.Container{Container: types.Container{Name: "b", AppName: "myapp", Status: provision.StatusBuilding.String()}},
		container.Container{Container: types.Container{Name: "c", AppName: "myapp", Status: provision.StatusStarting.String()}},
		container.Container{Container: types.Container{Name: "d", AppName: "myapp", Status: provision.StatusError.String()}},
		container.Container{Container: types.Container{Name: "e", AppName: "myapp", Status: provision.StatusStarted.String()}},
		container.Container{Container: types.Container{Name: "f", AppName: "myapp", Status: provision.StatusStopped.String()}},
	)
	defer coll.RemoveAll(bson.M{"appname": "myapp"})
	result, err := s.p.listRunnableContainersByApp("myapp")
	c.Assert(err, check.IsNil)
	c.Assert(result, check.HasLen, 3)
	var names []string
	for _, c := range result {
		names = append(names, c.Name)
	}
	sort.Strings(names)
	c.Assert(names, check.DeepEquals, []string{"c", "d", "e"})
}

func (s *S) TestListContainersByAppAndStatus(c *check.C) {
	var result []container.Container
	coll := s.p.Collection()
	defer coll.Close()
	coll.Insert(
		container.Container{Container: types.Container{ID: "1", AppName: "myapp1", Status: "started"}},
		container.Container{Container: types.Container{ID: "2", AppName: "myapp2", Status: "building"}},
		container.Container{Container: types.Container{ID: "3", AppName: "myapp3", Status: "stopped"}},
		container.Container{Container: types.Container{ID: "4", AppName: "myapp2", Status: "started"}},
	)
	defer coll.RemoveAll(bson.M{
		"appname": bson.M{
			"$in": []string{"myapp1", "myapp2", "myapp3"},
		},
	})
	result, err := s.p.listContainersByAppAndStatus([]string{"myapp1", "myapp2"}, []string{"started"})
	c.Assert(err, check.IsNil)
	sort.Sort(containerByIdList(result))
	c.Assert(stripMongoID(result), check.DeepEquals, []container.Container{
		{Container: types.Container{ID: "1", AppName: "myapp1", Status: "started"}},
		{Container: types.Container{ID: "4", AppName: "myapp2", Status: "started"}},
	})
	result, err = s.p.listContainersByAppAndStatus(nil, nil)
	c.Assert(err, check.IsNil)
	sort.Sort(containerByIdList(result))
	c.Assert(stripMongoID(result), check.DeepEquals, []container.Container{
		{Container: types.Container{ID: "1", AppName: "myapp1", Status: "started"}},
		{Container: types.Container{ID: "2", AppName: "myapp2", Status: "building"}},
		{Container: types.Container{ID: "3", AppName: "myapp3", Status: "stopped"}},
		{Container: types.Container{ID: "4", AppName: "myapp2", Status: "started"}},
	})
	result, err = s.p.listContainersByAppAndStatus(nil, []string{"started", "stopped"})
	c.Assert(err, check.IsNil)
	sort.Sort(containerByIdList(result))
	c.Assert(stripMongoID(result), check.DeepEquals, []container.Container{
		{Container: types.Container{ID: "1", AppName: "myapp1", Status: "started"}},
		{Container: types.Container{ID: "3", AppName: "myapp3", Status: "stopped"}},
		{Container: types.Container{ID: "4", AppName: "myapp2", Status: "started"}},
	})
}

func (s *S) TestListAppsForNodes(c *check.C) {
	coll := s.p.Collection()
	defer coll.Close()
	coll.Insert(
		container.Container{Container: types.Container{Name: "a", AppName: "app1", HostAddr: "host1.com"}},
		container.Container{Container: types.Container{Name: "b", AppName: "app2", HostAddr: "host1.com"}},
		container.Container{Container: types.Container{Name: "c", AppName: "app2", HostAddr: "host1.com"}},
		container.Container{Container: types.Container{Name: "d", AppName: "app3", HostAddr: "host2.com"}},
		container.Container{Container: types.Container{Name: "e", AppName: "app4", HostAddr: "host2.com"}},
		container.Container{Container: types.Container{Name: "f", AppName: "app5", HostAddr: "host3.com"}},
	)
	nodes := []*cluster.Node{{Address: "http://host1.com"}, {Address: "http://host3.com"}}
	apps, err := s.p.listAppsForNodes(nodes)
	c.Assert(err, check.IsNil)
	sort.Strings(apps)
	c.Assert(apps, check.DeepEquals, []string{"app1", "app2", "app5"})
}
