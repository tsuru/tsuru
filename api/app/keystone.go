package app

import (
	"errors"
	"github.com/timeredbull/openstack/keystone"
	"github.com/timeredbull/openstack/nova"
	"github.com/timeredbull/tsuru/config"
	"github.com/timeredbull/tsuru/log"
)

var (
	Client     keystone.Client
	authUrl    string
	authUser   string
	authPass   string
	authTenant string
)

type keystoneEnv struct {
	TenantId  string
	UserId    string
	AccessKey string
	secretKey string
	novaApi   nova.NetworkDisassociator
}

func (k *keystoneEnv) disassociate() error {
	err := k.disassociator().DisassociateNetwork(k.TenantId)
	if err == nova.ErrNoNetwork {
		return nil
	}
	return err
}

func (k *keystoneEnv) disassociator() nova.NetworkDisassociator {
	if k.novaApi == nil {
		k.novaApi = &nova.Client{KeystoneClient: &Client}
	}
	return k.novaApi
}

// getAuth retrieves information about openstack nova authentication. Uses the
// following confs:
//
//   - nova:
//     auth-url
//     user
//     password
//     tenant
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

func newKeystoneEnv(name string) (keystoneEnv, error) {
	err := getClient()
	if err != nil {
		return keystoneEnv{}, err
	}
	desc := "Tenant for " + name
	log.Printf("DEBUG: attempting to create tenant %s via keystone api...", name)
	tenant, err := Client.NewTenant(name, desc, true)
	if err != nil {
		log.Printf("ERROR: %s", err)
		return keystoneEnv{}, err
	}
	// var memberRole string
	// memberRole, err = config.GetString("nova:member-role")
	// if err != nil {
	// 	return keystoneEnv{}, err
	// }
	userId, err := config.GetString("nova:user-id")
	if err != nil {
		return keystoneEnv{}, err
	}
	creds, err := Client.NewEc2(userId, tenant.Id)
	if err != nil {
		log.Printf("ERROR: %s", err)
		return keystoneEnv{}, err
	}
	env := keystoneEnv{
		TenantId:  tenant.Id,
		UserId:    userId,
		AccessKey: creds.Access,
		secretKey: creds.Secret,
	}
	return env, nil
}

func destroyKeystoneEnv(env *keystoneEnv) error {
	if env.AccessKey == "" {
		return errors.New("Missing EC2 credentials.")
	}
	if env.UserId == "" {
		return errors.New("Missing user.")
	}
	if env.TenantId == "" {
		return errors.New("Missing tenant.")
	}
	var memberRole string
	memberRole, err := config.GetString("nova:member-role")
	if err != nil {
		return err
	}
	err = getClient()
	if err != nil {
		return err
	}
	err = Client.RemoveEc2(env.UserId, env.AccessKey)
	if err != nil {
		return err
	}
	err = Client.RemoveUser(env.UserId, env.TenantId, memberRole)
	if err != nil {
		return err
	}
	return Client.RemoveTenant(env.TenantId)
}
