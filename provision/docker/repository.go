// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"errors"
	"fmt"
	"time"

	"github.com/tsuru/docker-cluster/cluster"
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

func (p *dockerProvisioner) listRunningContainersByHost(address string) ([]container, error) {
	return p.listContainersBy(bson.M{
		"hostaddr": address,
		"status": bson.M{
			"$nin": []string{
				provision.StatusCreated.String(),
				provision.StatusBuilding.String(),
				provision.StatusStopped.String(),
			},
		},
	})
}

func (p *dockerProvisioner) listContainersByProcess(appName, processName string) ([]container, error) {
	query := bson.M{"appname": appName}
	if processName != "" {
		query["processname"] = processName
	}
	return p.listContainersBy(query)
}

func (p *dockerProvisioner) listContainersByApp(appName string) ([]container, error) {
	return p.listContainersBy(bson.M{"appname": appName})
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

func (p *dockerProvisioner) listAllContainers() ([]container, error) {
	return p.listContainersBy(nil)
}

func (p *dockerProvisioner) listAppsForNodes(nodes []*cluster.Node) ([]string, error) {
	coll := p.collection()
	defer coll.Close()
	nodeNames := make([]string, len(nodes))
	for i, n := range nodes {
		nodeNames[i] = urlToHost(n.Address)
	}
	var appNames []string
	err := coll.Find(bson.M{"hostaddr": bson.M{"$in": nodeNames}}).Distinct("appname", &appNames)
	return appNames, err
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
