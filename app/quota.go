// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"errors"
	"github.com/globocom/tsuru/db"
	"github.com/globocom/tsuru/quota"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
)

func reserveUnits(app *App, quantity int) error {
	err := checkAppLimit(app, quantity)
	if err != nil {
		return err
	}
	conn, err := db.NewStorage()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = conn.Apps().Update(
		bson.M{"name": app.Name, "quota.inuse": app.Quota.InUse},
		bson.M{"$inc": bson.M{"quota.inuse": quantity}},
	)
	for err == mgo.ErrNotFound {
		err = checkAppLimit(app, quantity)
		if err != nil {
			return err
		}
		err = conn.Apps().Update(
			bson.M{"name": app.Name, "quota.inuse": app.Quota.InUse},
			bson.M{"$inc": bson.M{"quota.inuse": quantity}},
		)
	}
	return err
}

func checkAppLimit(app *App, quantity int) error {
	err := app.Get()
	if err != nil {
		return err
	}
	if app.Quota.Limit > -1 && app.Quota.InUse+quantity > app.Quota.Limit {
		return &quota.QuotaExceededError{
			Available: uint(app.Quota.Limit - app.Quota.InUse),
			Requested: uint(quantity),
		}
	}
	return nil
}

func releaseUnits(app *App, quantity int) error {
	err := checkAppUsage(app, quantity)
	if err != nil {
		return err
	}
	conn, err := db.NewStorage()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = conn.Apps().Update(
		bson.M{"name": app.Name, "quota.inuse": app.Quota.InUse},
		bson.M{"$inc": bson.M{"quota.inuse": -1 * quantity}},
	)
	for err == mgo.ErrNotFound {
		err = checkAppUsage(app, quantity)
		if err != nil {
			return err
		}
		err = conn.Apps().Update(
			bson.M{"name": app.Name, "quota.inuse": app.Quota.InUse},
			bson.M{"$inc": bson.M{"quota.inuse": -1 * quantity}},
		)
	}
	return err
}

func checkAppUsage(app *App, quantity int) error {
	err := app.Get()
	if err != nil {
		return err
	}
	if app.Quota.InUse-quantity < 0 {
		return errors.New("Not enough reserved units")
	}
	return nil
}
