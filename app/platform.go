// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"github.com/tsuru/tsuru/db"
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
func PlatformAdd(name string, file string) error {
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
	err = Provisioner.PlatformAdd(name, file)
	return nil
}

type DuplicatePlatformError struct{}

func (DuplicatePlatformError) Error() string {
	return "Duplicate platform"
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
