// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"sort"
	"time"

	"github.com/tsuru/tsuru/provision"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"launchpad.net/gocheck"
)

func (s *S) getContainerCollection(appName string, containerIds ...string) func() {
	coll := collection()
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

func (s *S) TestListContainersByApp(c *gocheck.C) {
	var result []container
	coll := collection()
	defer coll.Close()
	coll.Insert(
		container{ID: "Hey", Type: "python", AppName: "myapp", HostAddr: "http://cittavld1180.globoi.com"},
		container{ID: "Ho", Type: "python", AppName: "myapp", HostAddr: "http://cittavld1182.globoi.com"},
		container{ID: "Let's Go", Type: "java", AppName: "other", HostAddr: "http://cittavld597.globoi.com"},
	)
	defer coll.RemoveAll(bson.M{"appname": "myapp"})
	result, err := listContainersByApp("myapp")
	c.Assert(err, gocheck.IsNil)
	c.Assert(len(result), gocheck.Equals, 2)
	check := (result[0].ID == "Hey" && result[1].ID == "Ho") || (result[0].ID == "Ho" && result[1].ID == "Hey")
	c.Assert(check, gocheck.Equals, true)
}

func (s *S) TestListContainersByHost(c *gocheck.C) {
	var result []container
	coll := collection()
	defer coll.Close()
	coll.Insert(
		container{ID: "1", Type: "python", AppName: "myapp", HostAddr: "http://cittavld1182.globoi.com"},
		container{ID: "2", Type: "python", AppName: "wat", HostAddr: "http://cittavld1182.globoi.com"},
		container{ID: "3", Type: "java", AppName: "masoq", HostAddr: "http://cittavld9999.globoi.com"},
	)
	defer coll.RemoveAll(bson.M{"hostaddr": "http://cittavld1182.globoi.com"})
	result, err := listContainersByHost("http://cittavld1182.globoi.com")
	c.Assert(err, gocheck.IsNil)
	c.Assert(len(result), gocheck.Equals, 2)
	check := (result[0].ID == "1" && result[1].ID == "2") || (result[0].ID == "2" && result[1].ID == "1")
	c.Assert(check, gocheck.Equals, true)
}

func (s *S) TestListAllContainers(c *gocheck.C) {
	appName := "some-app"
	containerIds := []string{"some-container-1", "some-container-2"}
	cleanupFunc := s.getContainerCollection(appName, containerIds...)
	defer cleanupFunc()
	containers, err := listAllContainers()
	c.Assert(err, gocheck.IsNil)
	c.Assert(len(containers), gocheck.Equals, 2)
	check := (containers[0].ID == containerIds[0] && containers[1].ID == containerIds[1]) ||
		(containers[0].ID == containerIds[1] && containers[1].ID == containerIds[0])
	c.Assert(check, gocheck.Equals, true)
}

func (s *S) TestGetOneContainerByAppName(c *gocheck.C) {
	appName := "some-app"
	containerIds := []string{"some-container-1", "some-container-2"}
	cleanupFunc := s.getContainerCollection(appName, containerIds...)
	defer cleanupFunc()
	container, err := getOneContainerByAppName(appName)
	c.Assert(err, gocheck.IsNil)
	c.Assert(container.AppName, gocheck.Equals, appName)
	checkId := container.ID == containerIds[0] || container.ID == containerIds[1]
	c.Assert(checkId, gocheck.Equals, true)
}

func (s *S) TestShouldNotGetOneContainerByAppName(c *gocheck.C) {
	coll := collection()
	defer coll.Close()
	container, err := getOneContainerByAppName("unexisting-app-name")
	c.Assert(err, gocheck.NotNil)
	c.Assert(container, gocheck.IsNil)
}

func (s *S) TestGetContainerCountForAppName(c *gocheck.C) {
	appName := "some-app"
	containerIds := []string{"some-container-1", "some-container-2"}
	cleanupFunc := s.getContainerCollection(appName, containerIds...)
	defer cleanupFunc()
	count, err := getContainerCountForAppName(appName)
	c.Assert(err, gocheck.IsNil)
	c.Assert(count, gocheck.Equals, len(containerIds))
}

func (s *S) TestGetContainerPartialIdAmbiguous(c *gocheck.C) {
	containerIds := []string{"container-1", "container-2"}
	cleanupFunc := s.getContainerCollection("some-app", containerIds...)
	defer cleanupFunc()
	_, err := getContainer("container")
	c.Assert(err, gocheck.Equals, errAmbiguousContainer)
}

func (s *S) TestGetContainerPartialIdNotFound(c *gocheck.C) {
	containerIds := []string{"container-1", "container-2"}
	cleanupFunc := s.getContainerCollection("some-app", containerIds...)
	defer cleanupFunc()
	_, err := getContainer("container-9")
	c.Assert(err, gocheck.Equals, mgo.ErrNotFound)
}

func (s *S) TestGetContainerPartialId(c *gocheck.C) {
	containerIds := []string{"container-a1", "container-b2"}
	cleanupFunc := s.getContainerCollection("some-app", containerIds...)
	defer cleanupFunc()
	cont, err := getContainer("container-a")
	c.Assert(err, gocheck.IsNil)
	c.Assert(cont.ID, gocheck.Equals, "container-a1")
}

func (s *S) TestListContainersByAppOrderedByStatus(c *gocheck.C) {
	coll := collection()
	defer coll.Close()
	coll.Insert(
		container{AppName: "myapp", ID: "0", Status: provision.StatusStarted.String()},
		container{AppName: "myapp", ID: "1", Status: provision.StatusBuilding.String()},
		container{AppName: "myapp", ID: "2", Status: provision.StatusUnreachable.String()},
		container{AppName: "myapp", ID: "3", Status: provision.StatusDown.String()},
	)
	defer coll.RemoveAll(bson.M{"appname": "myapp"})
	containers, err := listContainersByAppOrderedByStatus("myapp")
	c.Assert(err, gocheck.IsNil)
	c.Assert(len(containers), gocheck.Equals, 4)
	c.Assert(containers[0].Status, gocheck.Equals, provision.StatusDown.String())
	c.Assert(containers[1].Status, gocheck.Equals, provision.StatusBuilding.String())
	c.Assert(containers[2].Status, gocheck.Equals, provision.StatusUnreachable.String())
	c.Assert(containers[3].Status, gocheck.Equals, provision.StatusStarted.String())
}

func (S) TestcontainerSliceLen(c *gocheck.C) {
	containers := containerSlice{container{}, container{}}
	c.Assert(containers.Len(), gocheck.Equals, 2)
}

func (S) TestcontainerSliceLess(c *gocheck.C) {
	containers := containerSlice{
		container{Name: "b", Status: provision.StatusDown.String()},
		container{Name: "d", Status: provision.StatusBuilding.String()},
		container{Name: "e", Status: provision.StatusStarted.String()},
		container{Name: "s", Status: provision.StatusUnreachable.String()},
		container{Name: "z", Status: provision.StatusStopped.String()},
	}
	c.Assert(containers.Less(0, 1), gocheck.Equals, true)
	c.Assert(containers.Less(1, 2), gocheck.Equals, true)
	c.Assert(containers.Less(2, 0), gocheck.Equals, false)
	c.Assert(containers.Less(3, 2), gocheck.Equals, true)
	c.Assert(containers.Less(3, 1), gocheck.Equals, false)
	c.Assert(containers.Less(4, 3), gocheck.Equals, true)
	c.Assert(containers.Less(4, 1), gocheck.Equals, false)
}

func (S) TestcontainerSliceSwap(c *gocheck.C) {
	containers := containerSlice{
		container{Name: "b", Status: provision.StatusDown.String()},
		container{Name: "f", Status: provision.StatusBuilding.String()},
		container{Name: "g", Status: provision.StatusStarted.String()},
	}
	containers.Swap(0, 1)
	c.Assert(containers[0].Status, gocheck.Equals, provision.StatusBuilding.String())
	c.Assert(containers[1].Status, gocheck.Equals, provision.StatusDown.String())
}

func (S) TestcontainerSliceSort(c *gocheck.C) {
	containers := containerSlice{
		container{Name: "f", Status: provision.StatusBuilding.String()},
		container{Name: "g", Status: provision.StatusStarted.String()},
		container{Name: "b", Status: provision.StatusDown.String()},
	}
	c.Assert(sort.IsSorted(containers), gocheck.Equals, false)
	sort.Sort(containers)
	c.Assert(sort.IsSorted(containers), gocheck.Equals, true)
}

func (S) TestListUnresponsiveContainers(c *gocheck.C) {
	var result []container
	coll := collection()
	defer coll.Close()
	now := time.Now().UTC()
	coll.Insert(
		container{ID: "c1", AppName: "app_time_test", LastSuccessStatusUpdate: now},
		container{ID: "c2", AppName: "app_time_test", LastSuccessStatusUpdate: now.Add(-1 * time.Minute)},
		container{ID: "c3", AppName: "app_time_test", LastSuccessStatusUpdate: now.Add(-5 * time.Minute)},
	)
	defer coll.RemoveAll(bson.M{"appname": "app_time_test"})
	result, err := listUnresponsiveContainers(3 * time.Minute)
	c.Assert(err, gocheck.IsNil)
	c.Assert(len(result), gocheck.Equals, 1)
	c.Assert(result[0].ID, gocheck.Equals, "c3")
}
