package app

import (
	"fmt"
	"github.com/timeredbull/keystone"
	"github.com/timeredbull/tsuru/config"
	"github.com/timeredbull/tsuru/db"
	"labix.org/v2/mgo/bson"
)

type KeystoneEnv struct {
	TenantId  string
	UserId    string
	AccessKey string
}

var (
	Client     *keystone.Client
	authUrl    string
	authUser   string
	authPass   string
	authTenant string
)

func getAuth() (err error) {
	// TODO (flaviamissi) do not panic on errors, return
	// and log
	if authUrl == "" {
		authUrl, err = config.GetString("nova:auth-url")
		if err != nil {
			return
		}
	}
	if authUser == "" {
		authUser, err = config.GetString("nova:user")
		if err != nil {
			return
		}
	}
	if authPass == "" {
		authPass, err = config.GetString("nova:password")
		if err != nil {
			return
		}
	}
	if authTenant == "" {
		authTenant, err = config.GetString("nova:tenant")
		if err != nil {
			return
		}
	}
	return
}

func NewTenant(a *App) (tId string, err error) {
	err = getAuth()
	Client, err = keystone.NewClient(authUser, authPass, authTenant, authUrl)
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
