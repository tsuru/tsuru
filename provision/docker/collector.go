// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/provision"
	_ "github.com/tsuru/tsuru/router/hipache"
	_ "github.com/tsuru/tsuru/router/testing"
	"labix.org/v2/mgo/bson"
	"net"
	"strings"
	"sync"
)

func (p *dockerProvisioner) CollectStatus() error {
	var containersGroup sync.WaitGroup
	containers, err := listAllContainers()
	if err != nil {
		return err
	}
	if len(containers) == 0 {
		return nil
	}
	for _, container := range containers {
		containersGroup.Add(1)
		go collectUnit(container, units, &containersGroup)
	}
	containersGroup.Wait()
	close(units)
	return nil
}

func collectUnit(container container, wg *sync.WaitGroup) {
	defer wg.Done()
	if container.available() {
		ip, hostPort, err := container.networkInfo()
		if err == nil &&
			(hostPort != container.HostPort || ip != container.IP) {
			err = fixContainer(&container, ip, hostPort)
			if err != nil {
				log.Errorf("error on fix container hostport for [container %s]", container.ID)
				return
			}
		}
		addr := strings.Replace(container.getAddress(), "http://", "", 1)
		conn, err := net.Dial("tcp", addr)
		if err != nil {
			unit.Status = provision.StatusUnreachable
		} else {
			conn.Close()
			unit.Status = provision.StatusStarted
		}
	}
}

func fixContainer(container *container, ip, port string) error {
	router, err := getRouter()
	if err != nil {
		return err
	}
	router.RemoveRoute(container.AppName, container.getAddress())
	container.removeHost()
	container.IP = ip
	container.HostPort = port
	router.AddRoute(container.AppName, container.getAddress())
	coll := collection()
	defer coll.Close()
	return coll.Update(bson.M{"id": container.ID}, container)
}
