// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package juju

import (
	"github.com/globocom/tsuru/log"
	"github.com/globocom/tsuru/provision"
	"github.com/globocom/tsuru/queue"
	"sort"
	"sync"
)

const (
	addUnitToLoadBalancer = "add-unit-to-lb"
	queueName             = "tsuru-provision-juju"
)

type qApp struct {
	name string
}

func (a *qApp) GetName() string {
	return a.name
}

func handle(msg *queue.Message) {
	if msg.Action == addUnitToLoadBalancer {
		if len(msg.Args) < 1 {
			log.Errorf("Failed to handle %q: it requires at least one argument.", msg.Action)
			return
		}
		a := qApp{name: msg.Args[0]}
		unitNames := msg.Args[1:]
		sort.Strings(unitNames)
		status, err := (&JujuProvisioner{}).collectStatus()
		if err != nil {
			log.Errorf("Failed to handle %q: juju status failed.\n%s.", msg.Action, err)
			return
		}
		var units []provision.Unit
		for _, u := range status {
			if u.AppName != a.name {
				continue
			}
			n := sort.SearchStrings(unitNames, u.Name)
			if len(unitNames) == 0 ||
				n < len(unitNames) && unitNames[n] == u.Name {
				units = append(units, u)
			}
		}
		if len(units) == 0 {
			log.Errorf("Failed to handle %q: units not found.", msg.Action)
			return
		}
		var noID []string
		var ok []provision.Unit
		for _, u := range units {
			if u.InstanceId == "pending" || u.InstanceId == "" {
				noID = append(noID, u.Name)
			} else {
				ok = append(ok, u)
			}
		}
		if len(noID) == len(units) {
			getQueue(queueName).Put(msg, 0)
		} else {
			router, _ := Router()
			for _, u := range units {
				router.AddRoute(a.GetName(), u.InstanceId)
			}
			if len(noID) > 0 {
				args := []string{a.name}
				args = append(args, noID...)
				msg := queue.Message{
					Action: msg.Action,
					Args:   args,
				}
				getQueue(queueName).Put(&msg, 1e9)
			}
		}
	}
}

var (
	qfactory queue.QFactory
	_handler queue.Handler
	o        sync.Once
)

func setQueue() {
	var err error
	qfactory, err = queue.Factory()
	if err != nil {
		log.Fatalf("Failed to get the queue instance: %s", err)
	}
	_handler, err = qfactory.Handler(handle, queueName)
	if err != nil {
		log.Fatalf("Failed to create the queue handler: %s", err)
	}
}

func handler() queue.Handler {
	o.Do(setQueue)
	return _handler
}

func getQueue(name string) queue.Q {
	o.Do(setQueue)
	q, err := qfactory.Get(name)
	if err != nil {
		log.Fatalf("Failed to get queue %q: %s", name, err)
	}
	return q
}

func enqueue(msg *queue.Message) {
	getQueue(queueName).Put(msg, 0)
	handler().Start()
}
