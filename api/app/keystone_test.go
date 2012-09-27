package app

import (
	"errors"
	"fmt"
	"github.com/timeredbull/openstack/nova"
	"github.com/timeredbull/tsuru/config"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
	"regexp"
)

var (
	requestJson []byte
	// flags to detect when tenant url and user url are called
	called = make(map[string]bool)
	params = make(map[string]string)
)

func (s *S) mockServer(tenantBody, userBody, ec2Body, prefix string) *httptest.Server {
	var serverUrl string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		addRoleToUserRegexp := regexp.MustCompile(`/tenants/([\w-]+)/users/([\w-]+)/roles/OS-KSADM/([\w-]+)`)
		ec2Regexp := regexp.MustCompile(`/users/([\w-]+)/credentials/OS-EC2`)
		tenantsRegexp := regexp.MustCompile(`/tenants`)
		usersRegexp := regexp.MustCompile(`/users`)
		delEc2Regexp := regexp.MustCompile(`/users/([\w-]+)/credentials/OS-EC2/(\w+)`)
		delUsersRegexp := regexp.MustCompile(`/users/([\w-]+)`)
		delTenantsRegexp := regexp.MustCompile(`/tenants/([\w-]+)`)
		if r.URL.Path == "/tokens" {
			body := fmt.Sprintf(string(s.tokenBody), serverUrl)
			handleTokens(w, r, []byte(body))
			return
		}
		if m, _ := regexp.MatchString(`^/v2/[\w-]+/os-networks`, r.URL.Path); m {
			handleNova(w, r)
			return
		}
		if r.Method == "PUT" {
			if addRoleToUserRegexp.MatchString(r.URL.Path) {
				called["add-role-to-user"] = true
			} else {
				w.WriteHeader(http.StatusNotFound)
			}
		}
		if r.Method == "POST" {
			switch {
			case ec2Regexp.MatchString(r.URL.Path):
				handleCreds(w, r, ec2Body)
			case tenantsRegexp.MatchString(r.URL.Path):
				handleTenants(w, r, tenantBody)
			case usersRegexp.MatchString(r.URL.Path):
				handleUsers(w, r, userBody)
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}
		if r.Method == "DELETE" {
			switch {
			case delEc2Regexp.MatchString(r.URL.Path):
				called[prefix+"delete-ec2-creds"] = true
				submatches := delEc2Regexp.FindStringSubmatch(r.URL.Path)
				params[prefix+"ec2-user"], params[prefix+"ec2-access"] = submatches[1], submatches[2]
			case delUsersRegexp.MatchString(r.URL.Path):
				called[prefix+"delete-user"] = true
				submatches := delUsersRegexp.FindStringSubmatch(r.URL.Path)
				params[prefix+"user"] = submatches[1]
			case delTenantsRegexp.MatchString(r.URL.Path):
				called[prefix+"delete-tenant"] = true
				submatches := delTenantsRegexp.FindStringSubmatch(r.URL.Path)
				params[prefix+"tenant"] = submatches[1]
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}
	}))
	oldAuthUrl, _ := config.GetString("nova:auth-url")
	s.oldAuthUrl = oldAuthUrl
	config.Set("nova:auth-url", ts.URL)
	serverUrl = ts.URL
	return ts
}

func handleTenants(w http.ResponseWriter, r *http.Request, b string) {
	if b == "" {
		b = `{"tenant": {"id": "uuid123", "name": "tenant name", "description": "tenant desc"}}`
	}
	called["tenants"] = true
	w.Write([]byte(b))
}

func handleUsers(w http.ResponseWriter, r *http.Request, b string) {
	if b == "" {
		b = `{"user": {"id": "uuid321", "name": "appname", "email": "appname@foo.bar"}}`
	}
	called["users"] = true
	w.Write([]byte(b))
}

func handleCreds(w http.ResponseWriter, r *http.Request, b string) {
	if b == "" {
		b = `{"credential": {"access": "access-key-here", "secret": "secret-key-here"}}`
	}
	called["ec2-creds"] = true
	w.Write([]byte(b))
}

func handleTokens(w http.ResponseWriter, r *http.Request, b []byte) {
	body, err := ioutil.ReadAll(r.Body)
	defer r.Body.Close()
	if err != nil {
		panic(err)
	}
	requestJson = body
	called["token"] = true
	w.Write(b)
}

func handleNova(w http.ResponseWriter, r *http.Request) {
	response := `{"networks": [{"bridge": "br1808", "vpn_public_port": 1000, "dhcp_start": "172.25.8.3", "bridge_interface": "eth1", "updated_at": "2012-05-12 02:16:48", "id": "ef0aa0c4-48d8-4d9e-903a-61486cd60805", "cidr_v6": null, "deleted_at": null, "gateway": "172.25.8.1", "label": "private_0", "project_id": "%s", "vpn_private_address": "172.25.8.2", "deleted": false, "vlan": 1808, "broadcast": "172.25.8.255", "netmask": "255.255.255.0", "injected": false, "cidr": "172.25.8.0/24", "vpn_public_address": "10.170.0.14", "multi_host": true, "dns1": null, "host": null, "gateway_v6": null, "netmask_v6": null, "created_at": "2012-05-12 02:13:17"}]}`
	re := regexp.MustCompile(`/v2/([\w-]+)/os-networks`)
	switch r.Method {
	case "GET":
		tenant := re.FindStringSubmatch(r.URL.Path)[1]
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, response, tenant)
	case "POST":
		w.WriteHeader(http.StatusAccepted)
	}
}

func (s *S) TestGetAuth(c *C) {
	/* authUrl = "" */
	expectedUrl, err := config.GetString("nova:auth-url")
	c.Assert(err, IsNil)
	expectedUser, err := config.GetString("nova:user")
	c.Assert(err, IsNil)
	expectedPass, err := config.GetString("nova:password")
	c.Assert(err, IsNil)
	expectedTenant, err := config.GetString("nova:tenant")
	c.Assert(err, IsNil)
	url, user, pass, tenant, err := getAuth()
	c.Assert(err, IsNil)
	c.Assert(url, Equals, expectedUrl)
	c.Assert(user, Equals, expectedUser)
	c.Assert(pass, Equals, expectedPass)
	c.Assert(tenant, Equals, expectedTenant)
}

func (s *S) TestGetAuthShouldReturnErrorWhenAuthUrlConfigDoesntExists(c *C) {
	url, err := config.GetString("nova:auth-url")
	c.Assert(err, IsNil)
	defer config.Set("nova:auth-url", url)
	config.Unset("nova:auth-url")
	_, _, _, _, err = getAuth()
	c.Assert(err, NotNil)
}

func (s *S) TestGetAuthShouldReturnErrorWhenAuthUserConfigDoesntExists(c *C) {
	url, err := config.GetString("nova:user")
	c.Assert(err, IsNil)
	defer config.Set("nova:user", url)
	config.Unset("nova:user")
	_, _, _, _, err = getAuth()
	c.Assert(err, NotNil)
}

func (s *S) TestGetAuthShouldReturnErrorWhenAuthPassConfigDoesntExists(c *C) {
	url, err := config.GetString("nova:password")
	c.Assert(err, IsNil)
	defer config.Set("nova:password", url)
	config.Unset("nova:password")
	_, _, _, _, err = getAuth()
	c.Assert(err, NotNil)
}

func (s *S) TestGetClientShouldReturnClientWithAuthConfs(c *C) {
	_, authUser, authPass, authTenant, err := getAuth()
	c.Assert(err, IsNil)
	_, err = getClient()
	c.Assert(err, IsNil)
	req := string(requestJson)
	c.Assert(req, Not(Equals), "")
	expected := fmt.Sprintf(`{"auth": {"passwordCredentials": {"username": "%s", "password":"%s"}, "tenantName": "%s"}}`, authUser, authPass, authTenant)
	c.Assert(req, Equals, expected)
}

func (s *S) TestNewCredentials(c *C) {
	userId, err := config.GetString("nova:user-id")
	c.Assert(err, IsNil)
	roleId, err := config.GetString("nova:role-id")
	c.Assert(err, IsNil)
	accessKey, secretKey, err := newCredentials("uuid123", userId, roleId)
	c.Assert(err, IsNil)
	c.Assert(accessKey, Equals, "access-key-here")
	c.Assert(secretKey, Equals, "secret-key-here")
}

func (s *S) TestNewOpenstackEnv(c *C) {
	s.ts.Close()
	tenantBody := `{"tenant": {"id": "uuid123", "name": "still", "description": "tenant desc"}}`
	userBody := `{"user": {"id": "uuid321", "name": "still", "email": "appname@foo.bar"}}`
	ec2Body := `{"credential": {"access": "access-key-here", "secret": "secret-key-here"}}`
	s.ts = s.mockServer(tenantBody, userBody, ec2Body, "")
	env, err := newOpenstackEnv("still")
	c.Assert(err, IsNil)
	c.Assert(env.TenantId, Equals, "uuid123")
	userId, err := config.GetString("nova:user-id")
	c.Assert(err, IsNil)
	c.Assert(env.UserId, Equals, userId)
	c.Assert(env.AccessKey, Equals, "access-key-here")
	c.Assert(env.secretKey, Equals, "secret-key-here")
}

func (s *S) TestDestroyOpenstackEnv(c *C) {
	s.ts.Close()
	s.ts = s.mockServer("", "", "", "")
	k := openstackEnv{
		TenantId:  "e60d1f0a-ee74-411c-b879-46aee9502bf9",
		UserId:    "1b4d1195-7890-4274-831f-ddf8141edecc",
		AccessKey: "91232f6796b54ca2a2b87ef50548b123",
	}
	err := destroyOpenstackEnv(&k)
	c.Assert(err, IsNil)
	c.Assert(called["delete-ec2-creds"], Equals, true)
	c.Assert(params["ec2-access"], Equals, k.AccessKey)
	c.Assert(params["ec2-user"], Equals, k.UserId)
	c.Assert(called["delete-user"], Equals, true)
	c.Assert(params["user"], Equals, k.UserId)
	c.Assert(called["delete-tenant"], Equals, true)
	c.Assert(params["tenant"], Equals, k.TenantId)
}

func (s *S) TestDestroyOpenstackEnvWithoutEc2Creds(c *C) {
	k := openstackEnv{
		TenantId: "e60d1f0a-ee74-411c-b879-46aee9502bf9",
		UserId:   "1b4d1195-7890-4274-831f-ddf8141edecc",
	}
	err := destroyOpenstackEnv(&k)
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "Missing EC2 credentials.")
}

func (s *S) TestDestroyOpenstackEnvWithoutUserId(c *C) {
	k := openstackEnv{
		TenantId:  "e60d1f0a-ee74-411c-b879-46aee9502bf9",
		AccessKey: "91232f6796b54ca2a2b87ef50548b123",
	}
	err := destroyOpenstackEnv(&k)
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "Missing user.")
}

func (s *S) TestDestroyOpenstackEnvWithoutTenantId(c *C) {
	k := openstackEnv{
		UserId:    "1b4d1195-7890-4274-831f-ddf8141edecc",
		AccessKey: "91232f6796b54ca2a2b87ef50548b123",
	}
	err := destroyOpenstackEnv(&k)
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "Missing tenant.")
}

func (s *S) TestDisassociate(c *C) {
	disassociator := fakeDisassociator{}
	k := openstackEnv{
		TenantId: "123tenant",
		novaApi:  &disassociator,
	}
	err := k.disassociate()
	c.Assert(err, IsNil)
	c.Assert(disassociator.actions, DeepEquals, []string{"disassociate network from tenant 123tenant"})
}

func (s *S) TestDisassociateFromTenantWithoutNetwork(c *C) {
	disassociator := noNetworkDisassociator{}
	k := openstackEnv{
		TenantId: "123tenant",
		novaApi:  &disassociator,
	}
	err := k.disassociate()
	c.Assert(err, IsNil)
	c.Assert(disassociator.actions, DeepEquals, []string{"disassociate network from tenant 123tenant"})
}

func (s *S) TestDisassociateUnknownError(c *C) {
	disassociator := unknownErrorDisassociator{}
	k := openstackEnv{
		TenantId: "123tenant",
		novaApi:  &disassociator,
	}
	err := k.disassociate()
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^unknown error$")
}

func (s *S) TestDisassociator(c *C) {
	getClient()
	k := openstackEnv{}
	client, err := getClient()
	c.Assert(err, IsNil)
	expected := &nova.Client{KeystoneClient: client}
	c.Assert(k.disassociator(), DeepEquals, expected)
	fake := &fakeDisassociator{}
	k.novaApi = fake
	c.Assert(k.disassociator(), DeepEquals, fake)
}

type fakeDisassociator struct {
	actions []string
}

func (a *fakeDisassociator) DisassociateNetwork(tenantId string) error {
	a.actions = append(a.actions, "disassociate network from tenant "+tenantId)
	return nil
}

type noNetworkDisassociator struct {
	fakeDisassociator
}

func (a *noNetworkDisassociator) DisassociateNetwork(tenantId string) error {
	a.fakeDisassociator.DisassociateNetwork(tenantId)
	return nova.ErrNoNetwork
}

type unknownErrorDisassociator struct{}

func (a *unknownErrorDisassociator) DisassociateNetwork(tenantId string) error {
	return errors.New("unknown error")
}
