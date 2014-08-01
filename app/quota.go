// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"errors"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/quota"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

func reserveUnits(app *App, quantity int) error {
	app, err := checkAppLimit(app.Name, quantity)
	if err != nil {
		return err
	}
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = conn.Apps().Update(
		bson.M{"name": app.Name, "quota.inuse": app.Quota.InUse},
		bson.M{"$inc": bson.M{"quota.inuse": quantity}},
	)
	for err == mgo.ErrNotFound {
		app, err = checkAppLimit(app.Name, quantity)
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

func checkAppLimit(name string, quantity int) (*App, error) {
	app, err := GetByName(name)
	if err != nil {
		return nil, err
	}
	if app.Quota.Limit > -1 && app.Quota.InUse+quantity > app.Quota.Limit {
		return nil, &quota.QuotaExceededError{
			Available: uint(app.Quota.Limit - app.Quota.InUse),
			Requested: uint(quantity),
		}
	}
	return app, nil
}

func releaseUnits(app *App, quantity int) error {
	app, err := checkAppUsage(app.Name, quantity)
	if err != nil {
		return err
	}
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = conn.Apps().Update(
		bson.M{"name": app.Name, "quota.inuse": app.Quota.InUse},
		bson.M{"$inc": bson.M{"quota.inuse": -1 * quantity}},
	)
	for err == mgo.ErrNotFound {
		app, err = checkAppUsage(app.Name, quantity)
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

func checkAppUsage(name string, quantity int) (*App, error) {
	app, err := GetByName(name)
	if err != nil {
		return nil, err
	}
	if app.Quota.InUse-quantity < 0 {
		return nil, errors.New("Not enough reserved units")
	}
	return app, nil
}
