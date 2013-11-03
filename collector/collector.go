// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package collector

import (
	"github.com/globocom/tsuru/app"
	"github.com/globocom/tsuru/db"
	"github.com/globocom/tsuru/log"
	"github.com/globocom/tsuru/provision"
	"labix.org/v2/mgo/bson"
	"sort"
)

// AppList is a list of apps. It's not thread safe.
type AppList []*app.App

func (l AppList) Search(name string) (*app.App, int) {
	index := sort.Search(len(l), func(i int) bool {
		return l[i].Name >= name
	})
	if index < len(l) && l[index].Name == name {
		return l[index], -1
	} else if index < len(l) {
		return &app.App{Name: name}, index
	}
	return &app.App{Name: name}, len(l)
}

func (l *AppList) Add(a *app.App, index int) {
	length := len(*l)
	*l = append(*l, a)
	if index < length {
		for i := length; i > index; i-- {
			(*l)[i] = (*l)[i-1]
		}
		(*l)[index] = a
	}
}

func update(units []provision.Unit) {
	log.Debug("updating status from provisioner")
	var l AppList
	for _, unit := range units {
		a, index := l.Search(unit.AppName)
		if index > -1 {
			err := a.Get()
			if err != nil {
				log.Errorf("collector: app %q not found. Skipping.\n", unit.AppName)
				continue
			}
			l.Add(a, index)
		}
		u := app.Unit{}
		u.Name = unit.Name
		u.Type = unit.Type
		u.Machine = unit.Machine
		u.InstanceId = unit.InstanceId
		u.Ip = unit.Ip
		if unit.Status == provision.StatusStarted && a.State == "" {
			a.State = "ready"
		}
		u.State = string(unit.Status)
		a.AddUnit(&u)
	}
	conn, err := db.Conn()
	if err != nil {
		log.Errorf("collector failed to connect to the database: %s", err)
		return
	}
	defer conn.Close()
	for _, a := range l {
		a.Ip, err = app.Provisioner.Addr(a)
		if err != nil {
			log.Errorf("collector failed to get app (%q) address: %s", a.Name, err)
		}
		conn.Apps().Update(bson.M{"name": a.Name}, a)
	}
}
