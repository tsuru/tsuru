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

func (p *dockerProvisioner) CollectStatus() ([]provision.Unit, error) {
	var containersGroup sync.WaitGroup
	containers, err := listAllContainers()
	if err != nil {
		return nil, err
	}
	if len(containers) == 0 {
		return nil, nil
	}
	units := make(chan provision.Unit, len(containers))
	result := buildResult(len(containers), units)
	for _, container := range containers {
		containersGroup.Add(1)
		go collectUnit(container, units, &containersGroup)
	}
	containersGroup.Wait()
	close(units)
	return <-result, nil
}

func collectUnit(container container, units chan<- provision.Unit, wg *sync.WaitGroup) {
	defer wg.Done()
	unit := provision.Unit{
		Name:    container.ID,
		AppName: container.AppName,
		Type:    container.Type,
	}
	if container.available() {
		unit.Ip = container.HostAddr
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
		log.Debugf("collected data for [container %s] - [app %s]", container.ID, container.AppName)
		units <- unit
	}
}

func buildResult(maxSize int, units <-chan provision.Unit) <-chan []provision.Unit {
	ch := make(chan []provision.Unit, 1)
	go func() {
		result := make([]provision.Unit, 0, maxSize)
		for unit := range units {
			result = append(result, unit)
			log.Debugf("result for [container %s] - [app %s]", unit.Name, unit.AppName)
		}
		ch <- result
	}()
	return ch
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
