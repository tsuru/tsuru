// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/tsuru/tsuru/provision"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

var errAmbiguousContainer error = errors.New("ambiguous container name")

func (p *dockerProvisioner) getContainer(id string) (*container, error) {
	var containers []container
	coll := p.collection()
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

func (p *dockerProvisioner) listContainersByHost(address string) ([]container, error) {
	return p.listContainersBy(bson.M{"hostaddr": address})
}

func (p *dockerProvisioner) listContainersByApp(appName string) ([]container, error) {
	return p.listContainersBy(bson.M{"appname": appName})
}

func (p *dockerProvisioner) listContainersByImage(imageName string) ([]container, error) {
	return p.listContainersBy(bson.M{"image": imageName})
}

func (p *dockerProvisioner) listContainersByAppAndHost(appNames, addresses []string) ([]container, error) {
	query := bson.M{}
	if len(appNames) > 0 {
		query["appname"] = bson.M{"$in": appNames}
	}
	if len(addresses) > 0 {
		query["hostaddr"] = bson.M{"$in": addresses}
	}
	return p.listContainersBy(query)
}

func (p *dockerProvisioner) listRunnableContainersByApp(appName string) ([]container, error) {
	return p.listContainersBy(bson.M{
		"appname": appName,
		"status": bson.M{
			"$nin": []string{
				provision.StatusCreated.String(),
				provision.StatusBuilding.String(),
				provision.StatusStopped.String(),
			},
		},
	})
}

// ContainerSlice attaches the methods of sort.Interface to []container, sorting in increasing order.
type containerSlice []container

func (c containerSlice) Len() int {
	return len(c)
}

func (c containerSlice) Less(i, j int) bool {
	weight := map[string]int{
		provision.StatusCreated.String():  0,
		provision.StatusBuilding.String(): 1,
		provision.StatusError.String():    2,
		provision.StatusStopped.String():  3,
		provision.StatusStarting.String(): 4,
		provision.StatusStarted.String():  5,
	}
	return weight[c[i].Status] < weight[c[j].Status]
}

func (c containerSlice) Swap(i, j int) {
	c[i], c[j] = c[j], c[i]
}

func (p *dockerProvisioner) listContainersByAppOrderedByStatus(appName string) ([]container, error) {
	containers, err := p.listContainersBy(bson.M{"appname": appName})
	if err != nil {
		return nil, err
	}
	sort.Sort(containerSlice(containers))
	return containers, nil
}

func (p *dockerProvisioner) listAllContainers() ([]container, error) {
	return p.listContainersBy(nil)
}

func (p *dockerProvisioner) listContainersBy(query bson.M) ([]container, error) {
	var list []container
	coll := p.collection()
	defer coll.Close()
	err := coll.Find(query).All(&list)
	return list, err
}

func (p *dockerProvisioner) updateContainers(query bson.M, update bson.M) error {
	coll := p.collection()
	defer coll.Close()
	_, err := coll.UpdateAll(query, update)
	return err
}

func (p *dockerProvisioner) getOneContainerByAppName(appName string) (*container, error) {
	var c container
	coll := p.collection()
	defer coll.Close()
	err := coll.Find(bson.M{"appname": appName}).One(&c)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (p *dockerProvisioner) getContainerCountForAppName(appName string) (int, error) {
	coll := p.collection()
	defer coll.Close()
	return coll.Find(bson.M{"appname": appName}).Count()
}

func (p *dockerProvisioner) listUnresponsiveContainers(maxUnresponsiveTime time.Duration) ([]container, error) {
	now := time.Now().UTC()
	return p.listContainersBy(bson.M{
		"lastsuccessstatusupdate": bson.M{"$lt": now.Add(-maxUnresponsiveTime)},
		"hostport":                bson.M{"$ne": ""},
		"status":                  bson.M{"$ne": provision.StatusStopped.String()},
	})
}
