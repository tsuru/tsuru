// Copyright 2026 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package migrate

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/servicemanager"
)

func MigrateAppAutoScaleToDB(args []string) error {
	ctx := context.Background()

	if len(args) == 0 {
		return errors.New("No pool name provided on args")
	}
	poolName := args[0]

	pool, err := servicemanager.Pool.FindByName(ctx, poolName)
	if err != nil {
		return errors.Wrapf(err, "could not get pool %s", poolName)
	}

	apps, err := app.List(ctx, &app.Filter{
		Pools: []string{pool.Name},
	})
	if err != nil {
		return errors.Wrapf(err, "could not list apps for pool %s", pool.Name)
	}

	for _, a := range apps {
		fmt.Println("Migrating app", a.Name, "in pool", a.Pool)
		count, err := app.MigrateAutoScaleToDB(ctx, a)
		if err != nil {
			fmt.Printf("Failed to migrate app %s: %v\n", a.Name, err)
			continue
		}
		fmt.Printf("Migrated %d autoscale specs for app %s\n", count, a.Name)
	}
	return nil
}
