package app

import (
	"fmt"
	"github.com/timeredbull/keystone"
	"github.com/timeredbull/tsuru/db"
	"labix.org/v2/mgo/bson"
)

type KeystoneEnv struct {
	TenantId  string
	UserId    string
	AccessKey string
}

var Client *keystone.Client
var authUrl string // should be set on init() from tsuru.conf

func NewTenant(a *App) (tId string, err error) {
	Client, err = keystone.NewClient("foo", "bar", "tsuru", authUrl)
	if err != nil {
		return
	}
	desc := fmt.Sprintf("Tenant for %s", a.Name)
	t, err := Client.NewTenant("foo", desc, true)
	if err != nil {
		return
	}
	tId = t.Id
	a.KeystoneEnv.TenantId = tId
	err = db.Session.Apps().Update(bson.M{"name": a.Name}, &a)
	return
}
