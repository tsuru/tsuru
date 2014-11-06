// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"time"

	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/log"
)

const (
	cpuMax = 80
	cpuMin = 20
)

func allApps() ([]App, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var apps []App
	err = conn.Apps().Find(nil).All(&apps)
	if err != nil {
		return nil, err
	}
	return apps, nil
}

func runAutoScaleOnce() {
	apps, err := allApps()
	if err != nil {
		log.Error(err.Error())
	}
	for _, app := range apps {
		err := scaleApplicationIfNeeded(&app)
		if err != nil {
			log.Error(err.Error())
		}
	}
}

func runAutoScale() {
	for {
		runAutoScaleOnce()
		time.Sleep(30 * time.Second)
	}
}

func scaleApplicationIfNeeded(app *App) error {
	cpu, err := app.Cpu()
	if err != nil {
		return err
	}
	if cpu > cpuMax {
		_, err := AcquireApplicationLock(app.Name, InternalAppName, "auto-scale")
		if err != nil {
			return err
		}
		defer ReleaseApplicationLock(app.Name)
		return app.AddUnits(1, nil)
	}
	if cpu < cpuMin {
		_, err := AcquireApplicationLock(app.Name, InternalAppName, "auto-scale")
		if err != nil {
			return err
		}
		defer ReleaseApplicationLock(app.Name)
		return app.RemoveUnits(1)
	}
	return nil
}
