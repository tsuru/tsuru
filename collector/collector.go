// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"github.com/globocom/tsuru/app"
	"github.com/globocom/tsuru/db"
	"github.com/globocom/tsuru/log"
	"github.com/globocom/tsuru/provision"
	"labix.org/v2/mgo/bson"
)

func update(units []provision.Unit) {
	log.Print("updating status from provisioner")
	for _, unit := range units {
		a := app.App{Name: unit.AppName}
		err := a.Get()
		if err != nil {
			log.Printf("collector: app %q not found. Skipping.\n", unit.AppName)
			continue
		}
		u := app.Unit{}
		u.Name = unit.Name
		u.Type = unit.Type
		u.Machine = unit.Machine
		u.Ip = unit.Ip
		u.State = string(unit.Status)
		a.State = string(unit.Status)
		a.Ip = unit.Ip
		a.AddUnit(&u)
		db.Session.Apps().Update(bson.M{"name": a.Name}, a)
	}
}
