//
// This package provide test helpers for various actions
//
package testing

import (
	"github.com/globocom/config"
	"github.com/globocom/tsuru/db"
	"labix.org/v2/mgo/bson"
	. "launchpad.net/gocheck"
)

func (t *T) CreateAdminUserAndTeam(c *C) {
	t.Admin = user{Email: "superuser@gmail.com", Password: "123"}
	err := db.Session.Users().Insert(&t.Admin)
	c.Assert(err, IsNil)
	adminTeamName, err := config.GetString("admin-team")
	c.Assert(err, IsNil)
	t.AdminTeam = team{Name: adminTeamName, Users: []string{t.Admin.Email}}
	err = db.Session.Teams().Insert(t.AdminTeam)
	c.Assert(err, IsNil)
}

func (t *T) RemoveAdminUserAndTeam(c *C) {
	err := db.Session.Teams().RemoveId(t.AdminTeam.Name)
	c.Assert(err, IsNil)
	err = db.Session.Users().Remove(bson.M{"name": t.Admin.Email})
	c.Assert(err, IsNil)
}

func (t *T) CreateAppForAdmin(c *C) {
	t.AdminApp = app{Name: "testApp", Teams: []string{"notAdmin", "noSuperUser"}}
	err := db.Session.Apps().Insert(&t.AdminApp)
	c.Assert(err, IsNil)
}

func (t *T) RemoveAppForAdmin(c *C) {
	err := db.Session.Apps().Remove(bson.M{"name": t.AdminApp.Name})
	c.Assert(err, IsNil)
}
