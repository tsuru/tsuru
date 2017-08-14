// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"strconv"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/builder"
	"github.com/tsuru/tsuru/db"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/log"
	appTypes "github.com/tsuru/tsuru/types/app"
	"github.com/tsuru/tsuru/validation"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

func validatePlatform(p appTypes.Platform) error {
	if p.Name == "" {
		return appTypes.ErrPlatformNameMissing
	}
	if !validation.ValidateName(p.Name) {
		return appTypes.ErrInvalidPlatformName
	}
	return nil
}

// Platforms returns the list of available platforms.
func Platforms(enabledOnly bool) ([]appTypes.Platform, error) {
	var platforms []appTypes.Platform
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var query bson.M
	if enabledOnly {
		query = bson.M{"$or": []bson.M{{"disabled": false}, {"disabled": bson.M{"$exists": false}}}}
	}
	err = conn.Platforms().Find(query).All(&platforms)
	return platforms, err
}

// PlatformAdd add a new platform to tsuru
func PlatformAdd(opts builder.PlatformOptions) error {
	p := appTypes.Platform{Name: opts.Name}
	if err := validatePlatform(p); err != nil {
		return err
	}
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = conn.Platforms().Insert(p)
	if err != nil {
		if mgo.IsDup(err) {
			return appTypes.DuplicatePlatformError
		}
		return err
	}
	err = builder.PlatformAdd(opts)
	if err != nil {
		dbErr := conn.Platforms().RemoveId(p.Name)
		if dbErr != nil {
			return tsuruErrors.NewMultiError(
				errors.Wrapf(dbErr, "unable to rollback platform add"),
				errors.Wrapf(err, "original platform add error"),
			)
		}
		return err
	}
	return nil
}

func PlatformUpdate(opts builder.PlatformOptions) error {
	var platform appTypes.Platform
	if opts.Name == "" {
		return appTypes.ErrPlatformNameMissing
	}
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = conn.Platforms().Find(bson.M{"_id": opts.Name}).One(&platform)
	if err != nil {
		if err == mgo.ErrNotFound {
			return appTypes.ErrPlatformNotFound
		}
		return err
	}
	if opts.Args["dockerfile"] != "" || opts.Input != nil {
		err = builder.PlatformUpdate(opts)
		if err != nil {
			return err
		}
		var apps []App
		err = conn.Apps().Find(bson.M{"framework": opts.Name}).All(&apps)
		if err != nil {
			return err
		}
		for _, app := range apps {
			app.SetUpdatePlatform(true)
		}
	}
	if opts.Args["disabled"] != "" {
		disableBool, err := strconv.ParseBool(opts.Args["disabled"])
		if err != nil {
			return err
		}
		err = conn.Platforms().Update(bson.M{"_id": opts.Name}, bson.M{"$set": bson.M{"disabled": disableBool}})
		if err != nil {
			return err
		}
	}
	return nil
}

func PlatformRemove(name string) error {
	if name == "" {
		return appTypes.ErrPlatformNameMissing
	}
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	apps, _ := conn.Apps().Find(bson.M{"framework": name}).Count()
	if apps > 0 {
		return appTypes.ErrDeletePlatformWithApps
	}
	err = builder.PlatformRemove(name)
	if err != nil {
		log.Errorf("Failed to remove platform: %s", err)
	}
	err = conn.Platforms().Remove(bson.M{"_id": name})
	if err == mgo.ErrNotFound {
		return appTypes.ErrPlatformNotFound
	}
	return err
}

func GetPlatform(name string) (*appTypes.Platform, error) {
	var p appTypes.Platform
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	err = conn.Platforms().Find(bson.M{"_id": name}).One(&p)
	if err != nil {
		return nil, appTypes.InvalidPlatformError
	}
	return &p, nil
}
