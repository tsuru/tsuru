// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
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
	_, err := getContainerPartialId("container")
	c.Assert(err, gocheck.Equals, ambiguousContainerError)
}

func (s *S) TestGetContainerPartialIdNotFound(c *gocheck.C) {
	containerIds := []string{"container-1", "container-2"}
	cleanupFunc := s.getContainerCollection("some-app", containerIds...)
	defer cleanupFunc()
	_, err := getContainerPartialId("container-9")
	c.Assert(err, gocheck.Equals, mgo.ErrNotFound)
}

func (s *S) TestGetContainerPartialId(c *gocheck.C) {
	containerIds := []string{"container-a1", "container-b2"}
	cleanupFunc := s.getContainerCollection("some-app", containerIds...)
	defer cleanupFunc()
	cont, err := getContainerPartialId("container-a")
	c.Assert(err, gocheck.IsNil)
	c.Assert(cont.ID, gocheck.Equals, "container-a1")
}
