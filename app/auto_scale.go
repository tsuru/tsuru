// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

const (
	cpuMax = 80
	cpuMin = 20
)

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
