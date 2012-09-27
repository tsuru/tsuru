package app

import (
	"errors"
	"github.com/timeredbull/openstack/keystone"
	"github.com/timeredbull/openstack/nova"
	"github.com/timeredbull/tsuru/config"
	"github.com/timeredbull/tsuru/log"
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
		client, _ := getClient()
		k.novaApi = &nova.Client{KeystoneClient: client}
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
func getAuth() (url string, user string, pass string, tenant string, err error) {
	url, err = config.GetString("nova:auth-url")
	if err != nil {
		return
	}
	user, err = config.GetString("nova:user")
	if err != nil {
		return
	}
	pass, err = config.GetString("nova:password")
	if err != nil {
		return
	}
	tenant, err = config.GetString("nova:tenant")
	if err != nil {
		return
	}
	return
}

// getClient returns a new keystone.Client
// Uses the conf variables from the getAuth function.
func getClient() (*keystone.Client, error) {
	authUrl, authUser, authPass, authTenant, err := getAuth()
	if err != nil {
		return &keystone.Client{}, err
	}
	c, err := keystone.NewClient(authUser, authPass, authTenant, authUrl)
	if err != nil {
		return &keystone.Client{}, err
	}
	return c, nil
}

// Creates a tenant and associate it with tsuru user.
// Also associate the user and tenant with a pre defined role.
// Confs used:
//  - nova:user-id
//  - nova:role-id
func newKeystoneEnv(name string) (keystoneEnv, error) {
	client, err := getClient()
	if err != nil {
		return keystoneEnv{}, err
	}
	desc := "Tenant for " + name
	log.Printf("DEBUG: attempting to create tenant %s via keystone api...", name)
	tenant, err := client.NewTenant(name, desc, true)
	if err != nil {
		log.Printf("ERROR: %s", err)
		return keystoneEnv{}, err
	}
	var roleId string
	roleId, err = config.GetString("nova:role-id")
	if err != nil {
		return keystoneEnv{}, err
	}
	userId, err := config.GetString("nova:user-id")
	if err != nil {
		return keystoneEnv{}, err
	}
	access, secret, err := newCredentials(tenant.Id, userId, roleId)
	env := keystoneEnv{
		TenantId:  tenant.Id,
		UserId:    userId,
		AccessKey: access,
		secretKey: secret,
	}
	return env, nil
}

func newCredentials(tenantId, userId, roleId string) (accessKey string, secretKey string, err error) {
	client, err := getClient()
	if err != nil {
		return
	}
	err = client.AddRoleToUser(tenantId, userId, roleId)
	if err != nil {
		return
	}
	creds, err := client.NewEc2(userId, tenantId)
	if err != nil {
		return
	}
	accessKey = creds.Access
	secretKey = creds.Secret
	return
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
	var roleId string
	roleId, err := config.GetString("nova:role-id")
	if err != nil {
		return err
	}
	client, err := getClient()
	if err != nil {
		return err
	}
	err = client.RemoveEc2(env.UserId, env.AccessKey)
	if err != nil {
		return err
	}
	err = client.RemoveUser(env.UserId, env.TenantId, roleId)
	if err != nil {
		return err
	}
	return client.RemoveTenant(env.TenantId)
}
