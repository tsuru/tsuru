// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"errors"
	"fmt"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/provision"
	"io"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
)

type Platform struct {
	Name string `bson:"_id"`
}

// Platforms returns the list of available platforms.
func Platforms() ([]Platform, error) {
	var platforms []Platform
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	err = conn.Platforms().Find(nil).All(&platforms)
	return platforms, err
}

// PlatformAdd add a new platform to tsuru
func PlatformAdd(name string, args map[string]string, w io.Writer) error {
	var (
		provisioner provision.ExtensibleProvisioner
		ok          bool
	)
	if provisioner, ok = Provisioner.(provision.ExtensibleProvisioner); !ok {
		return errors.New("Provisioner is not extensible")
	}
	if name == "" {
		return errors.New("Platform name is required.")
	}
	p := Platform{Name: name}
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	err = conn.Platforms().Insert(p)
	if err != nil {
		if mgo.IsDup(err) {
			return DuplicatePlatformError{}
		}
		return err
	}
	err = provisioner.PlatformAdd(name, args, w)
	if err != nil {
		db_err := conn.Platforms().RemoveId(p.Name)
		if db_err != nil {
			return fmt.Errorf("Caused by: %s and %s", err.Error(), db_err.Error())
		}
		return err
	}
	return nil
}

type DuplicatePlatformError struct{}

func (DuplicatePlatformError) Error() string {
	return "Duplicate platform"
}

func PlatformUpdate(name string, args map[string]string, w io.Writer) error {
	var (
		provisioner provision.ExtensibleProvisioner
		platform    Platform
		ok          bool
	)
	if provisioner, ok = Provisioner.(provision.ExtensibleProvisioner); !ok {
		return errors.New("Provisioner is not extensible")
	}
	if name == "" {
		return errors.New("Platform name is required.")
	}
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	err = conn.Platforms().Find(bson.M{"_id": name}).One(&platform)
	if err != nil {
		if err == mgo.ErrNotFound {
			return errors.New("Platform doesn't exist.")
		}
		return err
	}
	err = provisioner.PlatformUpdate(name, args, w)
	if err != nil {
		return err
	}
	var apps []App
	err = conn.Apps().Find(bson.M{"framework": name}).All(&apps)
	if err != nil {
		return err
	}
	for _, app := range apps {
		app.SetUpdatePlatform(true)
	}
	return nil
}

func getPlatform(name string) (*Platform, error) {
	var p Platform
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	err = conn.Platforms().Find(bson.M{"_id": name}).One(&p)
	if err != nil {
		return nil, InvalidPlatformError{}
	}
	return &p, nil
}

type InvalidPlatformError struct{}

func (InvalidPlatformError) Error() string {
	return "Invalid platform"
}
