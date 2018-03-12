// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"strconv"

	"github.com/globalsign/mgo/bson"
	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/builder"
	"github.com/tsuru/tsuru/db"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/storage"
	appTypes "github.com/tsuru/tsuru/types/app"
	"github.com/tsuru/tsuru/validation"
)

func PlatformService() appTypes.PlatformStorage {
	dbDriver, err := storage.GetCurrentDbDriver()
	if err != nil {
		dbDriver, err = storage.GetDefaultDbDriver()
		if err != nil {
			return nil
		}
	}
	return dbDriver.PlatformStorage
}

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
	if enabledOnly {
		return PlatformService().FindEnabled()
	}
	return PlatformService().FindAll()
}

// PlatformAdd adds a new platform to tsuru
func PlatformAdd(opts builder.PlatformOptions) error {
	p := appTypes.Platform{Name: opts.Name}
	if err := validatePlatform(p); err != nil {
		return err
	}
	err := PlatformService().Insert(p)
	if err != nil {
		return err
	}
	err = builder.PlatformAdd(opts)
	if err != nil {
		dbErr := PlatformService().Delete(p)
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
	if opts.Name == "" {
		return appTypes.ErrPlatformNameMissing
	}
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = PlatformService().FindByName(opts.Name)
	if err != nil {
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
		return PlatformService().Update(appTypes.Platform{Name: opts.Name, Disabled: disableBool})
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
	return PlatformService().Delete(appTypes.Platform{Name: name})
}

func GetPlatform(name string) (*appTypes.Platform, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	p, err := PlatformService().FindByName(name)
	if err != nil {
		return nil, appTypes.ErrInvalidPlatform
	}
	return p, nil
}
