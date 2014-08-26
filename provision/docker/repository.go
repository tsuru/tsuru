// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"errors"
	"fmt"
	"sort"

	"github.com/tsuru/tsuru/provision"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

var errAmbiguousContainer error = errors.New("ambiguous container name")

func getContainer(id string) (*container, error) {
	var containers []container
	coll := collection()
	defer coll.Close()
	id = fmt.Sprintf("^%s.*", id)
	err := coll.Find(bson.M{"id": bson.RegEx{Pattern: id}}).All(&containers)
	if err != nil {
		return nil, err
	}
	lenContainers := len(containers)
	if lenContainers == 0 {
		return nil, mgo.ErrNotFound
	}
	if lenContainers > 1 {
		return nil, errAmbiguousContainer
	}
	return &containers[0], nil
}

func listContainersByHost(address string) ([]container, error) {
	return listContainersBy(bson.M{"hostaddr": address})
}

func listContainersByApp(appName string) ([]container, error) {
	return listContainersBy(bson.M{"appname": appName})
}

// ContainerSlice attaches the methods of sort.Interface to []container, sorting in increasing order.
type containerSlice []container

func (c containerSlice) Len() int {
	return len(c)
}

func (c containerSlice) Less(i, j int) bool {
	weight := map[string]int{
		provision.StatusDown.String():        0,
		provision.StatusBuilding.String():    1,
		provision.StatusStopped.String():     2,
		provision.StatusUnreachable.String(): 3,
		provision.StatusStarting.String():    4,
		provision.StatusStarted.String():     5,
	}
	return weight[c[i].Status] < weight[c[j].Status]
}

func (c containerSlice) Swap(i, j int) {
	c[i], c[j] = c[j], c[i]
}

func listContainersByAppOrderedByStatus(appName string) ([]container, error) {
	containers, err := listContainersBy(bson.M{"appname": appName})
	if err != nil {
		return nil, err
	}
	sort.Sort(containerSlice(containers))
	return containers, nil
}

func listAllContainers() ([]container, error) {
	return listContainersBy(nil)
}

func listContainersBy(query bson.M) ([]container, error) {
	var list []container
	coll := collection()
	defer coll.Close()
	err := coll.Find(query).All(&list)
	return list, err
}

func getOneContainerByAppName(appName string) (*container, error) {
	var c container
	coll := collection()
	defer coll.Close()
	err := coll.Find(bson.M{"appname": appName}).One(&c)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func getContainerCountForAppName(appName string) (int, error) {
	coll := collection()
	defer coll.Close()
	return coll.Find(bson.M{"appname": appName}).Count()
}
