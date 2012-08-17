package app

import (
	"errors"
	"fmt"
	"github.com/timeredbull/keystone"
	"github.com/timeredbull/tsuru/config"
	"github.com/timeredbull/tsuru/db"
	"github.com/timeredbull/tsuru/log"
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

// Retrieve information about openstack nova authentication
// Uses the following confs:
// - nova:
//  - auth-url
//  - user
//  - password
//  - tenant
// Returns error in case of failure obtaining any of the previous confs
func getAuth() (err error) {
	if authUrl == "" {
		authUrl, err = config.GetString("nova:auth-url")
		if err != nil {
			log.Printf("ERROR: %s", err.Error())
			return
		}
	}
	if authUser == "" {
		authUser, err = config.GetString("nova:user")
		if err != nil {
			log.Printf("ERROR: %s", err.Error())
			return
		}
	}
	if authPass == "" {
		authPass, err = config.GetString("nova:password")
		if err != nil {
			log.Printf("ERROR: %s", err.Error())
			return
		}
	}
	if authTenant == "" {
		authTenant, err = config.GetString("nova:tenant")
		if err != nil {
			log.Printf("ERROR: %s", err.Error())
			return
		}
	}
	return
}

// Fills global Client variable with the returned value from
// keystone.NewClient
// Uses the conf variables filled by getAuth function
func getClient() (err error) {
	err = getAuth()
	if err != nil {
		return
	}
	Client, err = keystone.NewClient(authUser, authPass, authTenant, authUrl)
	if err != nil {
		log.Printf("ERROR: a problem occurred while trying to obtain keystone's client: %s", err.Error())
		return
	}
	return
}

// Creates a tenant using keystone api
// and stores it in database embedded in
// the app document
// Returns the id of the created tenant in
// case of success and error in case of failure
func NewTenant(a *App) (tId string, err error) {
	err = getClient()
	if err != nil {
		return
	}
	desc := fmt.Sprintf("Tenant for %s", a.Name)
	log.Print(fmt.Sprintf("DEBUG: attempting to create tenant %s via keystone api...", a.Name))
	t, err := Client.NewTenant(a.Name, desc, true)
	if err != nil {
		log.Printf("ERROR: %s", err.Error())
		return
	}
	tId = t.Id
	a.KeystoneEnv.TenantId = tId
	err = db.Session.Apps().Update(bson.M{"name": a.Name}, &a)
	if err != nil {
		log.Printf("ERROR: %s", err.Error())
		return
	}
	log.Printf("DEBUG: tenant %s successfuly created.", a.Name)
	return
}

func NewUser(a *App) (uId string, err error) {
	if a.KeystoneEnv.TenantId == "" {
		err = errors.New("App should have an associated keystone tenant to create an user.")
		log.Printf("ERROR: %s", err.Error())
		return
	}
	err = getClient()
	if err != nil {
		return
	}
	log.Print("DEBUG: attempting to create user %s via keystone api...", a.Name)
	// TODO(flaviamissi): should generate a random password
	u, err := Client.NewUser(a.Name, a.Name, "", a.KeystoneEnv.TenantId, true)
	if err != nil {
		log.Printf("ERROR: %s", err.Error())
		return
	}
	uId = u.Id
	a.KeystoneEnv.UserId = uId
	err = db.Session.Apps().Update(bson.M{"name": a.Name}, &a)
	return
}

func NewEC2Creds(a *App) (access, secret string, err error) {
	if a.KeystoneEnv.TenantId == "" {
		err = errors.New("App should have an associated keystone tenant to create an user.")
		log.Printf("ERROR: %s", err.Error())
		return
	}
	if a.KeystoneEnv.UserId == "" {
		err = errors.New("App should have an associated keystone user to create an user.")
		log.Printf("ERROR: %s", err.Error())
		return
	}
	err = getClient()
	if err != nil {
		return
	}
	ec2, err := Client.NewEc2(a.KeystoneEnv.UserId, a.KeystoneEnv.TenantId)
	if err != nil {
		log.Printf("ERROR: %s", err.Error())
		return
	}
	access = ec2.Access
	secret = ec2.Secret
	a.KeystoneEnv.AccessKey = access
	err = db.Session.Apps().Update(bson.M{"name": a.Name}, &a)
	return
}
