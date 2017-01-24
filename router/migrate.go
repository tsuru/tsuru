// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package router

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/storage"
	"gopkg.in/mgo.v2/bson"
)

func Initialize() error {
	_, err := collection()
	return err
}

// MigrateUniqueCollection only exists because old versions of tsuru (<1.2.0)
// allowed the insertion of incorrect duplicated entries in the db.routers
// collection. This migration tries its best to fix the inconsistencies in this
// collection and fails if that's not possible.
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
	byAppMap, err := allDupEntries(coll)
	if err != nil {
		return err
	}
	var toRemove []bson.ObjectId
	for appName, appEntries := range byAppMap {
		toRemoveDups := checkAllDups(appEntries)
		toRemove = append(toRemove, toRemoveDups...)
		var toRemoveAddrs []bson.ObjectId
		toRemoveAddrs, err = checkAppAddr(appColl, appName, appEntries)
		if err != nil {
			return err
		}
		toRemove = append(toRemove, toRemoveAddrs...)
	}
	_, err = coll.RemoveAll(bson.M{"_id": bson.M{"$in": toRemove}})
	if err != nil {
		return err
	}
	var routerAppNames []string
	err = coll.Find(nil).Distinct("app", &routerAppNames)
	if err != nil {
		return err
	}
	var missingApps []string
	err = appColl.Find(bson.M{"name": bson.M{"$nin": routerAppNames}}).Distinct("name", &missingApps)
	if err != nil {
		return err
	}
	for _, missingEntry := range missingApps {
		err = coll.Insert(routerAppEntry{
			App:    missingEntry,
			Router: missingEntry,
		})
		if err != nil {
			return err
		}
	}
	byAppMap, err = allDupEntries(coll)
	if err != nil {
		return err
	}
	if len(byAppMap) == 0 {
		_, err = collection()
		return err
	}
	errBuf := bytes.NewBuffer(nil)
	fmt.Fprintln(errBuf, `ERROR: The following entries in 'db.routers' collection have inconsistent
duplicated entries that could not be fixed automatically. This could have
happened after running app-swap due to a bug in previous tsuru versions. You'll
have to manually check if the apps are swapped or not and remove the duplicated
entries accordingly:`)
	for appName, entries := range byAppMap {
		fmt.Fprintf(errBuf, "app %q:\n", appName)
		for _, e := range entries {
			fmt.Fprintf(errBuf, "  %#v\n", e)
		}
	}
	return errors.New(errBuf.String())
}

func checkAllDups(appEntries []routerAppEntry) []bson.ObjectId {
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
		return toRemoveApp
	}
	return nil
}

func checkAppAddr(appColl *storage.Collection, appName string, appEntries []routerAppEntry) ([]bson.ObjectId, error) {
	var appAddr []string
	err := appColl.Find(bson.M{"name": appName}).Distinct("ip", &appAddr)
	if err != nil {
		return nil, err
	}
	if len(appAddr) != 1 {
		return nil, errors.Errorf("invalid app addr size %q: %d", appName, len(appAddr))
	}
	var tempToRemove []bson.ObjectId
	allowRemoval := false
	for _, entry := range appEntries {
		if allowRemoval || !strings.HasPrefix(appAddr[0], entry.Router+".") {
			tempToRemove = append(tempToRemove, entry.ID)
		} else {
			allowRemoval = true
		}
	}
	if allowRemoval {
		return tempToRemove, nil
	}
	return nil, nil
}

func allDupEntries(coll *storage.Collection) (map[string][]routerAppEntry, error) {
	var entries []routerAppEntry
	err := coll.Find(nil).All(&entries)
	if err != nil {
		return nil, err
	}
	byAppMap := map[string][]routerAppEntry{}
	for _, r := range entries {
		byAppMap[r.App] = append(byAppMap[r.App], r)
	}
	for appName, appEntries := range byAppMap {
		if len(appEntries) == 1 {
			delete(byAppMap, appName)
		}
	}
	return byAppMap, nil
}
