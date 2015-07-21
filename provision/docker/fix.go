// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/router"
	"gopkg.in/mgo.v2/bson"
)

func (p *dockerProvisioner) fixContainers() error {
	containers, err := p.listAllContainers()
	if err != nil {
		return err
	}
	err = runInContainers(containers, func(c *container, _ chan *container) error {
		return p.checkContainer(c)
	}, nil, true)
	if err != nil {
		log.Errorf("error checking containers for fixing: %s", err.Error())
	}
	return err
}

func (p *dockerProvisioner) checkContainer(container *container) error {
	if container.available() {
		info, err := container.networkInfo(p)
		if err != nil {
			return err
		}
		if info.HTTPHostPort != container.HostPort || info.IP != container.IP {
			err = p.fixContainer(container, info)
			if err != nil {
				log.Errorf("error on fix container hostport for [container %s]", container.ID)
				return err
			}
		}
	}
	return nil
}

func (p *dockerProvisioner) fixContainer(container *container, info containerNetworkInfo) error {
	if info.HTTPHostPort == "" {
		return nil
	}
	appInstance, err := app.GetByName(container.AppName)
	if err != nil {
		return err
	}
	r, err := getRouterForApp(appInstance)
	if err != nil {
		return err
	}
	err = r.RemoveRoute(container.AppName, container.getAddress())
	if err != nil && err != router.ErrRouteNotFound {
		return err
	}
	container.IP = info.IP
	container.HostPort = info.HTTPHostPort
	err = r.AddRoute(container.AppName, container.getAddress())
	if err != nil && err != router.ErrRouteExists {
		return err
	}
	coll := p.collection()
	defer coll.Close()
	return coll.Update(bson.M{"id": container.ID}, container)
}
