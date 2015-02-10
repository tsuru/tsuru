// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"sort"
	"time"

	"github.com/tsuru/tsuru/provision"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

func (s *S) getContainerCollection(appName string, containerIds ...string) func() {
	coll := s.p.collection()
	for _, containerId := range containerIds {
		container := container{AppName: appName, ID: containerId}
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
	var result []container
	coll := s.p.collection()
	defer coll.Close()
	coll.Insert(
		container{ID: "Hey", Type: "python", AppName: "myapp", HostAddr: "http://cittavld1180.globoi.com"},
		container{ID: "Ho", Type: "python", AppName: "myapp", HostAddr: "http://cittavld1182.globoi.com"},
		container{ID: "Let's Go", Type: "java", AppName: "other", HostAddr: "http://cittavld597.globoi.com"},
	)
	defer coll.RemoveAll(bson.M{"appname": "myapp"})
	result, err := s.p.listContainersByApp("myapp")
	c.Assert(err, check.IsNil)
	c.Assert(len(result), check.Equals, 2)
	cond := (result[0].ID == "Hey" && result[1].ID == "Ho") || (result[0].ID == "Ho" && result[1].ID == "Hey")
	c.Assert(cond, check.Equals, true)
}

func (s *S) TestListContainersByHost(c *check.C) {
	var result []container
	coll := s.p.collection()
	defer coll.Close()
	coll.Insert(
		container{ID: "1", Type: "python", AppName: "myapp", HostAddr: "http://cittavld1182.globoi.com"},
		container{ID: "2", Type: "python", AppName: "wat", HostAddr: "http://cittavld1182.globoi.com"},
		container{ID: "3", Type: "java", AppName: "masoq", HostAddr: "http://cittavld9999.globoi.com"},
	)
	defer coll.RemoveAll(bson.M{"hostaddr": "http://cittavld1182.globoi.com"})
	result, err := s.p.listContainersByHost("http://cittavld1182.globoi.com")
	c.Assert(err, check.IsNil)
	c.Assert(len(result), check.Equals, 2)
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
	c.Assert(len(containers), check.Equals, 2)
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
	c.Assert(len(containers), check.Equals, 3)
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
	coll := s.p.collection()
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
	_, err := s.p.getContainer("container")
	c.Assert(err, check.Equals, errAmbiguousContainer)
}

func (s *S) TestGetContainerPartialIdNotFound(c *check.C) {
	containerIds := []string{"container-1", "container-2"}
	cleanupFunc := s.getContainerCollection("some-app", containerIds...)
	defer cleanupFunc()
	_, err := s.p.getContainer("container-9")
	c.Assert(err, check.Equals, mgo.ErrNotFound)
}

func (s *S) TestGetContainerPartialId(c *check.C) {
	containerIds := []string{"container-a1", "container-b2"}
	cleanupFunc := s.getContainerCollection("some-app", containerIds...)
	defer cleanupFunc()
	cont, err := s.p.getContainer("container-a")
	c.Assert(err, check.IsNil)
	c.Assert(cont.ID, check.Equals, "container-a1")
}

func (s *S) TestListContainersByAppOrderedByStatus(c *check.C) {
	coll := s.p.collection()
	defer coll.Close()
	coll.Insert(
		container{AppName: "myapp", ID: "0", Status: provision.StatusStarted.String()},
		container{AppName: "myapp", ID: "1", Status: provision.StatusBuilding.String()},
		container{AppName: "myapp", ID: "2", Status: provision.StatusError.String()},
		container{AppName: "myapp", ID: "3", Status: provision.StatusCreated.String()},
		container{AppName: "myapp", ID: "4", Status: provision.StatusStopped.String()},
		container{AppName: "myapp", ID: "5", Status: provision.StatusStarting.String()},
	)
	defer coll.RemoveAll(bson.M{"appname": "myapp"})
	containers, err := s.p.listContainersByAppOrderedByStatus("myapp")
	c.Assert(err, check.IsNil)
	c.Assert(len(containers), check.Equals, 6)
	c.Assert(containers[0].Status, check.Equals, provision.StatusCreated.String())
	c.Assert(containers[1].Status, check.Equals, provision.StatusBuilding.String())
	c.Assert(containers[2].Status, check.Equals, provision.StatusError.String())
	c.Assert(containers[3].Status, check.Equals, provision.StatusStopped.String())
	c.Assert(containers[4].Status, check.Equals, provision.StatusStarting.String())
	c.Assert(containers[5].Status, check.Equals, provision.StatusStarted.String())
}

func (s *S) TestcontainerSliceLen(c *check.C) {
	containers := containerSlice{container{}, container{}}
	c.Assert(containers.Len(), check.Equals, 2)
}

func (s *S) TestcontainerSliceLess(c *check.C) {
	containers := containerSlice{
		container{Name: "b", Status: provision.StatusError.String()},
		container{Name: "d", Status: provision.StatusBuilding.String()},
		container{Name: "e", Status: provision.StatusStarted.String()},
		container{Name: "s", Status: provision.StatusStarting.String()},
		container{Name: "z", Status: provision.StatusStopped.String()},
		container{Name: "p", Status: provision.StatusCreated.String()},
	}
	c.Assert(containers.Less(0, 1), check.Equals, false)
	c.Assert(containers.Less(1, 2), check.Equals, true)
	c.Assert(containers.Less(2, 0), check.Equals, false)
	c.Assert(containers.Less(3, 2), check.Equals, true)
	c.Assert(containers.Less(3, 1), check.Equals, false)
	c.Assert(containers.Less(4, 3), check.Equals, true)
	c.Assert(containers.Less(4, 1), check.Equals, false)
	c.Assert(containers.Less(5, 1), check.Equals, true)
	c.Assert(containers.Less(5, 0), check.Equals, true)
}

func (s *S) TestcontainerSliceSwap(c *check.C) {
	containers := containerSlice{
		container{Name: "b", Status: provision.StatusError.String()},
		container{Name: "f", Status: provision.StatusBuilding.String()},
		container{Name: "g", Status: provision.StatusStarted.String()},
	}
	containers.Swap(0, 1)
	c.Assert(containers[0].Status, check.Equals, provision.StatusBuilding.String())
	c.Assert(containers[1].Status, check.Equals, provision.StatusError.String())
}

func (s *S) TestcontainerSliceSort(c *check.C) {
	containers := containerSlice{
		container{Name: "f", Status: provision.StatusBuilding.String()},
		container{Name: "g", Status: provision.StatusStarted.String()},
		container{Name: "b", Status: provision.StatusError.String()},
	}
	c.Assert(sort.IsSorted(containers), check.Equals, false)
	sort.Sort(containers)
	c.Assert(sort.IsSorted(containers), check.Equals, true)
}

func (s *S) TestListUnresponsiveContainers(c *check.C) {
	var result []container
	coll := s.p.collection()
	defer coll.Close()
	now := time.Now().UTC()
	coll.Insert(
		container{ID: "c1", AppName: "app_time_test", LastSuccessStatusUpdate: now, HostPort: "80"},
		container{ID: "c2", AppName: "app_time_test", LastSuccessStatusUpdate: now.Add(-1 * time.Minute), HostPort: "80"},
		container{ID: "c3", AppName: "app_time_test", LastSuccessStatusUpdate: now.Add(-5 * time.Minute), HostPort: "80"},
	)
	defer coll.RemoveAll(bson.M{"appname": "app_time_test"})
	result, err := s.p.listUnresponsiveContainers(3 * time.Minute)
	c.Assert(err, check.IsNil)
	c.Assert(len(result), check.Equals, 1)
	c.Assert(result[0].ID, check.Equals, "c3")
}

func (s *S) TestListUnresponsiveContainersNoHostPort(c *check.C) {
	var result []container
	coll := s.p.collection()
	defer coll.Close()
	now := time.Now().UTC()
	coll.Insert(
		container{ID: "c1", AppName: "app_time_test", LastSuccessStatusUpdate: now.Add(-10 * time.Minute)},
	)
	defer coll.RemoveAll(bson.M{"appname": "app_time_test"})
	result, err := s.p.listUnresponsiveContainers(3 * time.Minute)
	c.Assert(err, check.IsNil)
	c.Assert(len(result), check.Equals, 0)
}

func (s *S) TestListUnresponsiveContainersStopped(c *check.C) {
	var result []container
	coll := s.p.collection()
	defer coll.Close()
	now := time.Now().UTC()
	coll.Insert(
		container{ID: "c1", AppName: "app_time_test",
			LastSuccessStatusUpdate: now.Add(-5 * time.Minute), HostPort: "80", Status: provision.StatusStopped.String()},
		container{ID: "c2", AppName: "app_time_test",
			LastSuccessStatusUpdate: now.Add(-5 * time.Minute), HostPort: "80", Status: provision.StatusStarted.String()},
	)
	defer coll.RemoveAll(bson.M{"appname": "app_time_test"})
	result, err := s.p.listUnresponsiveContainers(3 * time.Minute)
	c.Assert(err, check.IsNil)
	c.Assert(len(result), check.Equals, 1)
	c.Assert(result[0].ID, check.Equals, "c2")
}

func (s *S) TestListRunnableContainersByApp(c *check.C) {
	var result []container
	coll := s.p.collection()
	defer coll.Close()
	coll.Insert(
		container{Name: "a", AppName: "myapp", Status: provision.StatusCreated.String()},
		container{Name: "b", AppName: "myapp", Status: provision.StatusBuilding.String()},
		container{Name: "c", AppName: "myapp", Status: provision.StatusStarting.String()},
		container{Name: "d", AppName: "myapp", Status: provision.StatusError.String()},
		container{Name: "e", AppName: "myapp", Status: provision.StatusStarted.String()},
		container{Name: "f", AppName: "myapp", Status: provision.StatusStopped.String()},
	)
	defer coll.RemoveAll(bson.M{"appname": "myapp"})
	result, err := s.p.listRunnableContainersByApp("myapp")
	c.Assert(err, check.IsNil)
	c.Assert(len(result), check.Equals, 3)
	var names []string
	for _, c := range result {
		names = append(names, c.Name)
	}
	sort.Strings(names)
	c.Assert(names, check.DeepEquals, []string{"c", "d", "e"})
}
