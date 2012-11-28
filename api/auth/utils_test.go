// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"github.com/globocom/tsuru/db"
	"labix.org/v2/mgo/bson"
	. "launchpad.net/gocheck"
)

type testApp struct {
	Name  string
	Teams []string
}

func (s *S) TestAllowedAppsShouldReturnAllAppsTheUserHasAccess(c *C) {
	team := Team{Name: "teamname", Users: []string{s.user.Email}}
	err := db.Session.Teams().Insert(&team)
	c.Assert(err, IsNil)
	a := testApp{Name: "myapp", Teams: []string{s.team.Name}}
	err = db.Session.Apps().Insert(&a)
	c.Assert(err, IsNil)
	a2 := testApp{Name: "myotherapp", Teams: []string{team.Name}}
	err = db.Session.Apps().Insert(&a2)
	c.Assert(err, IsNil)
	defer func() {
		db.Session.Apps().Remove(bson.M{"name": bson.M{"$in": []string{a.Name, a2.Name}}})
		db.Session.Teams().RemoveId(team.Name)
	}()
	aApps, err := allowedApps(s.user.Email)
	c.Assert(aApps, DeepEquals, []string{a.Name, a2.Name})
}
