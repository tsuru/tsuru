// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"labix.org/v2/mgo/bson"
)

func getContainer(id string) (*container, error) {
	var c container
	coll := collection()
	defer coll.Close()
	err := coll.Find(bson.M{"id": id}).One(&c)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func listAppContainers(appName string) ([]container, error) {
	var containers []container
	coll := collection()
	defer coll.Close()
	err := coll.Find(bson.M{"appname": appName}).All(&containers)
	return containers, err
}

func getOneContainerByAppName(appName string) (*container, error) {
	var c container
	coll := collection()
	defer coll.Close()
	err := coll.Find(bson.M{"appname": appName}).One(&c)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func getContainerCountForAppName(appName string) (int, error) {
	coll := collection()
	defer coll.Close()
	return coll.Find(bson.M{"appname": appName}).Count()
}
