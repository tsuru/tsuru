// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"github.com/globocom/tsuru/app"
	"github.com/globocom/tsuru/db"
	"github.com/globocom/tsuru/provision"
	"labix.org/v2/mgo/bson"
	"testing"
)

func getFakeUnits(apps []string) []provision.Unit {
	units := make([]provision.Unit, 10*len(apps))
	for i := range units {
		app := apps[i%len(apps)]
		units[i] = provision.Unit{
			Name:    fmt.Sprintf("%s/%d", app, i+1),
			AppName: app,
			Type:    "python",
			Ip:      fmt.Sprintf("10.10.%d.%d", i%255, i+1),
			Status:  provision.StatusStarted,
			Machine: i + 1,
		}
	}
	return units
}

func getFakeApps() ([]app.App, []string) {
	apps := make([]app.App, 20)
	names := make([]string, len(apps))
	for i := range apps {
		name := fmt.Sprintf("app%d", i+1)
		names[i] = name
		apps[i] = app.App{
			Name:      name,
			Framework: "python",
			State:     provision.StatusStarted.String(),
		}
		err := db.Session.Apps().Insert(apps[i])
		if err != nil {
			panic(err)
		}
	}
	return apps, names
}

func BenchmarkUpdate(b *testing.B) {
	b.StopTimer()
	var err error
	db.Session, err = db.Open("127.0.0.1:27017", "collector-benchmark")
	if err != nil {
		panic(err)
	}
	defer db.Session.Close()
	defer db.Session.Apps().Database.DropDatabase()
	_, names := getFakeApps()
	defer db.Session.Apps().Remove(bson.M{"name": bson.M{"$in": names}})
	fakeUnits := getFakeUnits(names)
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		update(fakeUnits)
	}
}
