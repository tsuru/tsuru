// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package juju

import (
	"github.com/globocom/config"
	"github.com/globocom/tsuru/db"
	"github.com/globocom/tsuru/log"
	"github.com/globocom/tsuru/provision"
	"labix.org/v2/mgo"
)

// ELBManager manages load balances within Amazon Elastic Load Balancer.
//
// If juju:use-elb is true on tsuru.conf, this manager will be used for
// managing load balancers on tsuru.
//
// It uses db package and adds a new collection to tsuru's DB. The name of the
// collection is also defined in the configuration file (juju:elb-collection).
type ELBManager struct {
	p *JujuProvisioner
}

func (m *ELBManager) collection() *mgo.Collection {
	name, err := config.GetString("juju:elb-collection")
	if err != nil {
		log.Fatal("juju:elb-collection is undefined on config file.")
	}
	return db.Session.Collection(name)
}

func (m *ELBManager) Create(app provision.App) error {
	return nil
}

func (m *ELBManager) Destroy(app provision.App) error {
	return nil
}

func (m *ELBManager) Register(app provision.App, unit provision.Unit) error {
	return nil
}

func (m *ELBManager) Deregister(app provision.App, unit provision.Unit) error {
	return nil
}
