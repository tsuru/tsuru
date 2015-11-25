// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/provision/docker/container"
	"github.com/tsuru/tsuru/router"
	"gopkg.in/mgo.v2/bson"
)

func (p *dockerProvisioner) checkContainer(container *container.Container) error {
	if container.Available() {
		info, err := container.NetworkInfo(p)
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

func (p *dockerProvisioner) fixContainer(container *container.Container, info container.NetworkInfo) error {
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
	err = r.RemoveRoute(container.AppName, container.Address())
	if err != nil && err != router.ErrRouteNotFound {
		return err
	}
	container.IP = info.IP
	container.HostPort = info.HTTPHostPort
	err = r.AddRoute(container.AppName, container.Address())
	if err != nil && err != router.ErrRouteExists {
		return err
	}
	coll := p.Collection()
	defer coll.Close()
	return coll.Update(bson.M{"id": container.ID}, container)
}
