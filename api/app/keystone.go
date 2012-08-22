package app

import (
	"errors"
	"fmt"
	"github.com/timeredbull/keystone"
	"github.com/timeredbull/tsuru/config"
	"github.com/timeredbull/tsuru/log"
)

type KeystoneEnv struct {
	TenantId  string
	UserId    string
	AccessKey string
	secretKey string
}

var (
	Client     keystone.Client
	authUrl    string
	authUser   string
	authPass   string
	authTenant string
)

// getAuth retrieves information about openstack nova authentication. Uses the
// following confs:
//
//  - nova:
//  - auth-url
//  - user
//  - password
//  - tenant
//
// Returns error in case of failure obtaining any of the previous confs.
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

// getClient fills global Client variable with the returned value from
// keystone.NewClient.
//
// Uses the conf variables filled by getAuth function.
func getClient() (err error) {
	if Client.Token != "" {
		return
	}
	err = getAuth()
	if err != nil {
		return
	}
	c, err := keystone.NewClient(authUser, authPass, authTenant, authUrl)
	if err != nil {
		log.Printf("ERROR: a problem occurred while trying to obtain keystone's client: %s", err.Error())
		return
	}
	Client = *c
	return
}

func newKeystoneEnv(name string) (env KeystoneEnv, err error) {
	err = getClient()
	if err != nil {
		return
	}
	desc := "Tenant for " + name
	log.Printf("DEBUG: attempting to create tenant %s via keystone api...", name)
	tenant, err := Client.NewTenant(name, desc, true)
	if err != nil {
		log.Printf("ERROR: %s", err)
		return
	}
	password := name
	if random, err := randomBytes(64); err == nil {
		password = fmt.Sprintf("%X", random)
	}
	user, err := Client.NewUser(name, password, "", tenant.Id, true)
	if err != nil {
		log.Printf("ERROR: %s", err)
		return
	}
	creds, err := Client.NewEc2(user.Id, tenant.Id)
	if err != nil {
		log.Printf("ERROR: %s", err)
		return
	}
	env = KeystoneEnv{
		TenantId:  tenant.Id,
		UserId:    user.Id,
		AccessKey: creds.Access,
		secretKey: creds.Secret,
	}
	return
}

func destroyKeystoneEnv(env *KeystoneEnv) error {
	if env.AccessKey == "" {
		return errors.New("Missing EC2 credentials.")
	}
	if env.UserId == "" {
		return errors.New("Missing user.")
	}
	if env.TenantId == "" {
		return errors.New("Missing tenant.")
	}
	err := getClient()
	if err != nil {
		return err
	}
	err = Client.RemoveEc2(env.UserId, env.AccessKey)
	if err != nil {
		return err
	}
	err = Client.RemoveTenant(env.TenantId)
	if err != nil {
		return err
	}
	return Client.RemoveUser(env.UserId)
}
