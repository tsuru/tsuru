// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package router

import (
	"bytes"
	"fmt"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/db"
	"gopkg.in/mgo.v2/bson"
)

func MigrateUniqueCollection() error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	coll := conn.Collection("routers")
	appColl := conn.Apps()
	var appNames []string
	err = appColl.Find(nil).Distinct("name", &appNames)
	if err != nil {
		return err
	}
	_, err = coll.RemoveAll(bson.M{"app": bson.M{"$nin": appNames}, "router": bson.M{"$nin": appNames}})
	if err != nil {
		return err
	}
	var entries []routerAppEntry
	err = coll.Find(nil).All(&entries)
	if err != nil {
		return err
	}
	byAppMap := map[string][]routerAppEntry{}
	for _, r := range entries {
		byAppMap[r.App] = append(byAppMap[r.App], r)
	}
	var toRemove []bson.ObjectId
	for appName, appEntries := range byAppMap {
		if len(appEntries) == 1 {
			delete(byAppMap, appName)
			continue
		}
		remove := true
		var toRemoveApp []bson.ObjectId
		for i := 1; i < len(appEntries); i++ {
			toRemoveApp = append(toRemoveApp, appEntries[i].ID)
			if appEntries[i].Router != appEntries[i-1].Router {
				remove = false
				break
			}
		}
		if remove {
			toRemove = append(toRemove, toRemoveApp...)
			delete(byAppMap, appName)
		}
	}
	_, err = coll.RemoveAll(bson.M{"_id": bson.M{"$in": toRemove}})
	if err != nil {
		return err
	}
	if len(byAppMap) == 0 {
		_, err = collection()
		return err
	}
	errBuf := bytes.NewBuffer(nil)
	fmt.Fprintln(errBuf, `WARNING: The following entries in 'db.routers' collection have inconsistent
duplicated entries that could not be fixed automatically. This could have
happened after running app-swap due to a bug in previous tsuru versions. You'll
have to manually check which if the apps are swapped or not and remove the
duplicated entries accordingly:`)
	for appName, entries := range byAppMap {
		fmt.Fprintf(errBuf, "app %q:\n", appName)
		for _, e := range entries {
			fmt.Fprintf(errBuf, "  %#v\n", e)
		}
	}
	return errors.New(errBuf.String())
}
