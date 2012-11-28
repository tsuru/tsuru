// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"github.com/globocom/tsuru/db"
	"labix.org/v2/mgo/bson"
)

func allowedApps(email string) ([]string, error) {
	var teams []Team
	var alwdApps []map[string]string
	if err := db.Session.Teams().Find(bson.M{"users": email}).Select(bson.M{"_id": 1}).All(&teams); err != nil {
		return []string{}, err
	}
	teamNames := GetTeamsNames(teams)
	if err := db.Session.Apps().Find(bson.M{"teams": bson.M{"$in": teamNames}}).Select(bson.M{"name": 1}).All(&alwdApps); err != nil {
		return []string{}, err
	}
	appNames := make([]string, len(alwdApps))
	for i, v := range alwdApps {
		appNames[i] = v["name"]
	}
	return appNames, nil
}
