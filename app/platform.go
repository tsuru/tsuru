// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"strconv"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/db"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/provision"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

type Platform struct {
	Name     string `bson:"_id"`
	Disabled bool   `bson:",omitempty"`
}

var (
	ErrPlatformNameMissing    = errors.New("Platform name is required.")
	ErrPlatformNotFound       = errors.New("Platform doesn't exist.")
	DuplicatePlatformError    = errors.New("Duplicate platform")
	InvalidPlatformError      = errors.New("Invalid platform")
	ErrDeletePlatformWithApps = errors.New("Platform has apps. You should remove them before remove the platform.")
)

// Platforms returns the list of available platforms.
func Platforms(enabledOnly bool) ([]Platform, error) {
	var platforms []Platform
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
func PlatformAdd(opts provision.PlatformOptions) error {
	if opts.Name == "" {
		return ErrPlatformNameMissing
	}
	provisioners, err := provision.Registry()
	if err != nil {
		return err
	}
	p := Platform{Name: opts.Name}
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = conn.Platforms().Insert(p)
	if err != nil {
		if mgo.IsDup(err) {
			return DuplicatePlatformError
		}
		return err
	}
	for _, p := range provisioners {
		if extensibleProv, ok := p.(provision.ExtensibleProvisioner); ok {
			err = extensibleProv.PlatformAdd(opts)
			if err != nil {
				break
			}
		}
	}
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

func PlatformUpdate(opts provision.PlatformOptions) error {
	provisioners, err := provision.Registry()
	if err != nil {
		return err
	}
	var platform Platform
	if opts.Name == "" {
		return ErrPlatformNameMissing
	}
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = conn.Platforms().Find(bson.M{"_id": opts.Name}).One(&platform)
	if err != nil {
		if err == mgo.ErrNotFound {
			return ErrPlatformNotFound
		}
		return err
	}
	if opts.Args["dockerfile"] != "" || opts.Input != nil {
		for _, p := range provisioners {
			if extensibleProv, ok := p.(provision.ExtensibleProvisioner); ok {
				err = extensibleProv.PlatformUpdate(opts)
				if err != nil {
					return err
				}
			}
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
	provisioners, err := provision.Registry()
	if err != nil {
		return err
	}
	if name == "" {
		return ErrPlatformNameMissing
	}
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	apps, _ := conn.Apps().Find(bson.M{"framework": name}).Count()
	if apps > 0 {
		return ErrDeletePlatformWithApps
	}
	for _, p := range provisioners {
		if extensibleProv, ok := p.(provision.ExtensibleProvisioner); ok {
			err = extensibleProv.PlatformRemove(name)
			if err != nil {
				log.Errorf("Failed to remove platform from provisioner %q: %s", p.GetName(), err)
			}
		}
	}
	err = conn.Platforms().Remove(bson.M{"_id": name})
	if err == mgo.ErrNotFound {
		return ErrPlatformNotFound
	}
	return err
}

func GetPlatform(name string) (*Platform, error) {
	var p Platform
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	err = conn.Platforms().Find(bson.M{"_id": name}).One(&p)
	if err != nil {
		return nil, InvalidPlatformError
	}
	return &p, nil
}
