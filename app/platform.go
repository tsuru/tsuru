// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"errors"
	"github.com/tsuru/tsuru/db"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
    "io"
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
	return Provisioner.PlatformAdd(name, args, w)
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
