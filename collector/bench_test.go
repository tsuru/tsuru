// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package collector

import (
	"fmt"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/provision"
	ttesting "github.com/tsuru/tsuru/testing"
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
		}
	}
	return units
}

func getFakeApps(conn *db.Storage) ([]app.App, []string) {
	apps := make([]app.App, 20)
	names := make([]string, len(apps))
	for i := range apps {
		name := fmt.Sprintf("app%d", i+1)
		names[i] = name
		apps[i] = app.App{
			Name:     name,
			Platform: "python",
		}
		err := conn.Apps().Insert(apps[i])
		if err != nil {
			panic(err)
		}
	}
	return apps, names
}

func BenchmarkUpdate(b *testing.B) {
	b.StopTimer()
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "collector-benchmark")
	conn, err := db.Conn()
	if err != nil {
		panic(err)
	}
	defer conn.Apps().Database.DropDatabase()
	_, names := getFakeApps(conn)
	app.Provisioner = ttesting.NewFakeProvisioner()
	fakeUnits := getFakeUnits(names)
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		update(fakeUnits)
	}
}
