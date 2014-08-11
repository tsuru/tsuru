// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"github.com/tsuru/tsuru/log"
	_ "github.com/tsuru/tsuru/router/hipache"
	_ "github.com/tsuru/tsuru/router/testing"
	"gopkg.in/mgo.v2/bson"
	"sync"
)

func fixContainers() error {
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
		go checkContainer(container, &containersGroup)
	}
	containersGroup.Wait()
	return nil
}

func checkContainer(container container, wg *sync.WaitGroup) {
	defer wg.Done()
	if container.available() {
		info, err := container.networkInfo()
		if err == nil &&
			(info.HTTPHostPort != container.HostPort || info.IP != container.IP || info.SSHHostPort != container.SSHHostPort) {
			err = fixContainer(&container, info)
			if err != nil {
				log.Errorf("error on fix container hostport for [container %s]", container.ID)
				return
			}
		}
	}
}

func fixContainer(container *container, info containerNetworkInfo) error {
	router, err := getRouter()
	if err != nil {
		return err
	}
	router.RemoveRoute(container.AppName, container.getAddress())
	container.IP = info.IP
	container.HostPort = info.HTTPHostPort
	container.SSHHostPort = info.SSHHostPort
	router.AddRoute(container.AppName, container.getAddress())
	coll := collection()
	defer coll.Close()
	return coll.Update(bson.M{"id": container.ID}, container)
}
