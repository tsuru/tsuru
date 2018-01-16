// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"fmt"

	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/docker/container"
)

func (p *dockerProvisioner) GetContainer(id string) (*container.Container, error) {
	var containers []container.Container
	coll := p.Collection()
	defer coll.Close()
	pattern := fmt.Sprintf("^%s.*", id)
	err := coll.Find(bson.M{"id": bson.RegEx{Pattern: pattern}}).All(&containers)
	if err != nil {
		return nil, err
	}
	lenContainers := len(containers)
	if lenContainers == 0 {
		return nil, &provision.UnitNotFoundError{ID: id}
	}
	if lenContainers > 1 {
		log.Debugf("ambiguous container id. found %d containers (%v) when looking for %q", lenContainers, containers, id)
		return nil, &AmbiguousContainerError{ID: id}
	}
	return &containers[0], nil
}

func (p *dockerProvisioner) GetContainerByName(name string) (*container.Container, error) {
	var containers []container.Container
	coll := p.Collection()
	defer coll.Close()
	err := coll.Find(bson.M{"name": name}).All(&containers)
	if err != nil {
		return nil, err
	}
	lenContainers := len(containers)
	if lenContainers == 0 {
		return nil, &provision.UnitNotFoundError{ID: name}
	}
	if lenContainers > 1 {
		return nil, &AmbiguousContainerError{ID: name}
	}
	return &containers[0], nil
}

func (p *dockerProvisioner) listContainersByHost(address string) ([]container.Container, error) {
	return p.ListContainers(bson.M{"hostaddr": address})
}

func (p *dockerProvisioner) listRunningContainersByHost(address string) ([]container.Container, error) {
	return p.ListContainers(bson.M{
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

func (p *dockerProvisioner) listContainersByProcess(appName, processName string) ([]container.Container, error) {
	query := bson.M{"appname": appName}
	if processName != "" {
		query["processname"] = processName
	}
	return p.ListContainers(query)
}

func (p *dockerProvisioner) listContainersByApp(appName string) ([]container.Container, error) {
	return p.ListContainers(bson.M{"appname": appName})
}

func (p *dockerProvisioner) listContainersByAppAndHost(appNames, addresses []string) ([]container.Container, error) {
	query := bson.M{}
	if len(appNames) > 0 {
		query["appname"] = bson.M{"$in": appNames}
	}
	if len(addresses) > 0 {
		query["hostaddr"] = bson.M{"$in": addresses}
	}
	return p.ListContainers(query)
}

func (p *dockerProvisioner) listRunnableContainersByApp(appName string) ([]container.Container, error) {
	return p.ListContainers(bson.M{
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

func (p *dockerProvisioner) listContainersByAppAndStatus(appNames []string, status []string) ([]container.Container, error) {
	query := bson.M{}
	if len(appNames) > 0 {
		query["appname"] = bson.M{"$in": appNames}
	}
	if len(status) > 0 {
		query["status"] = bson.M{"$in": status}
	}
	return p.ListContainers(query)
}

func (p *dockerProvisioner) listAllContainers() ([]container.Container, error) {
	return p.ListContainers(nil)
}

func (p *dockerProvisioner) listContainersWithIDOrName(ids []string, names []string) ([]container.Container, error) {
	return p.ListContainers(bson.M{
		"$or": []bson.M{
			{"name": bson.M{"$in": names}},
			{"id": bson.M{"$in": ids}},
		},
	})
}

func (p *dockerProvisioner) listAppsForNodes(nodes []*cluster.Node) ([]string, error) {
	coll := p.Collection()
	defer coll.Close()
	nodeNames := make([]string, len(nodes))
	for i, n := range nodes {
		nodeNames[i] = net.URLToHost(n.Address)
	}
	var appNames []string
	err := coll.Find(bson.M{"hostaddr": bson.M{"$in": nodeNames}}).Distinct("appname", &appNames)
	return appNames, err
}

func (p *dockerProvisioner) ListContainers(query bson.M) ([]container.Container, error) {
	var list []container.Container
	coll := p.Collection()
	defer coll.Close()
	err := coll.Find(query).All(&list)
	return list, err
}

func (p *dockerProvisioner) updateContainers(query bson.M, update bson.M) error {
	coll := p.Collection()
	defer coll.Close()
	_, err := coll.UpdateAll(query, update)
	return err
}

func (p *dockerProvisioner) getOneContainerByAppName(appName string) (*container.Container, error) {
	var c container.Container
	coll := p.Collection()
	defer coll.Close()
	err := coll.Find(bson.M{"appname": appName}).One(&c)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (p *dockerProvisioner) getContainerCountForAppName(appName string) (int, error) {
	coll := p.Collection()
	defer coll.Close()
	return coll.Find(bson.M{"appname": appName}).Count()
}

type AmbiguousContainerError struct {
	ID string
}

func (e *AmbiguousContainerError) Error() string {
	return fmt.Sprintf("ambiguous container name/id: %q", e.ID)
}
