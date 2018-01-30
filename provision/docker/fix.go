// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/provision/docker/container"
	"github.com/tsuru/tsuru/router/rebuild"
)

func (p *dockerProvisioner) checkContainer(container *container.Container) error {
	if container.Available() {
		info, err := container.NetworkInfo(p.ClusterClient())
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
	container.IP = info.IP
	container.HostPort = info.HTTPHostPort
	coll := p.Collection()
	defer coll.Close()
	err := coll.Update(bson.M{"id": container.ID}, bson.M{
		"$set": bson.M{"hostport": container.HostPort, "ip": container.IP},
	})
	rebuild.LockedRoutesRebuildOrEnqueue(container.AppName)
	return err
}
