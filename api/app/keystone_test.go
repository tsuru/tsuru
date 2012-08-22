package app

import (
	"fmt"
	"github.com/timeredbull/tsuru/config"
	"github.com/timeredbull/tsuru/fs/testing"
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
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ec2Regexp := regexp.MustCompile(`/users/([\w-]+)/credentials/OS-EC2`)
		tenantsRegexp := regexp.MustCompile(`/tenants`)
		usersRegexp := regexp.MustCompile(`/users`)
		delEc2Regexp := regexp.MustCompile(`/users/([\w-]+)/credentials/OS-EC2/(\w+)`)
		delUsersRegexp := regexp.MustCompile(`/users/([\w-]+)`)
		delTenantsRegexp := regexp.MustCompile(`/tenants/([\w-]+)`)
		if r.URL.Path == "/tokens" {
			handleTokens(w, r)
			return
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
	authUrl = ts.URL
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

func handleTokens(w http.ResponseWriter, r *http.Request) {
	body, err := ioutil.ReadAll(r.Body)
	defer r.Body.Close()
	if err != nil {
		panic(err)
	}
	requestJson = body
	called["token"] = true
	w.Write([]byte(`{"access": {"token": {"id": "token-id-987"}}}`))
}

func (s *S) TestGetAuth(c *C) {
	authUrl = ""
	expectedUrl, err := config.GetString("nova:auth-url")
	c.Assert(err, IsNil)
	getAuth()
	c.Assert(authUrl, Equals, expectedUrl)
}

func (s *S) TestGetAuthShouldNotGetTheConfigEveryTimeItsCalled(c *C) {
	authUrl = ""
	expectedUrl, err := config.GetString("nova:auth-url")
	c.Assert(err, IsNil)
	getAuth()
	c.Assert(authUrl, Equals, expectedUrl)
	config.Set("nova:auth-url", "some-url.com")
	getAuth()
	c.Assert(authUrl, Equals, expectedUrl)
	defer config.Set("nova:auth-url", expectedUrl)
}

func (s *S) TestGetAuthShouldSetAuthUser(c *C) {
	expectedUser, err := config.GetString("nova:user")
	c.Assert(err, IsNil)
	getAuth()
	c.Assert(authUser, Equals, expectedUser)
}

func (s *S) TestGetAuthShouldNotResetAuthUserWhenItIsAlreadySet(c *C) {
	expectedUser, err := config.GetString("nova:user")
	c.Assert(err, IsNil)
	getAuth()
	c.Assert(authUser, Equals, expectedUser)
	config.Set("nova:user", "some-other-user")
	defer config.Set("nova:user", expectedUser)
	getAuth()
	c.Assert(authUser, Equals, expectedUser)
}

func (s *S) TestGetAuthShouldSerAuthPass(c *C) {
	expected, err := config.GetString("nova:password")
	c.Assert(err, IsNil)
	getAuth()
	c.Assert(authPass, Equals, expected)
}

func (s *S) TestGetAuthShouldNotResetAuthPass(c *C) {
	expected, err := config.GetString("nova:password")
	c.Assert(err, IsNil)
	getAuth()
	c.Assert(authPass, Equals, expected)
	config.Set("nova:password", "ultra-secret-pass")
	getAuth()
	c.Assert(authPass, Equals, expected)
	defer config.Set("nova:password", expected)
}

func (s *S) TestGetAuthShouldSerAuthTenant(c *C) {
	expected, err := config.GetString("nova:tenant")
	c.Assert(err, IsNil)
	getAuth()
	c.Assert(authTenant, Equals, expected)
}

func (s *S) TestGetAuthShouldNotResetAuthTenant(c *C) {
	expected, err := config.GetString("nova:tenant")
	c.Assert(err, IsNil)
	getAuth()
	c.Assert(authTenant, Equals, expected)
	config.Set("nova:tenant", "some-other-tenant")
	getAuth()
	c.Assert(authTenant, Equals, expected)
	defer config.Set("nova:tenant", expected)
}

func (s *S) TestGetAuthShouldReturnErrorWhenAuthUrlConfigDoesntExists(c *C) {
	getAuth()
	url := authUrl
	defer config.Set("nova:auth-url", url)
	defer func() { authUrl = url }()
	authUrl = ""
	config.Unset("nova:auth-url")
	err := getAuth()
	c.Assert(err, Not(IsNil))
}

func (s *S) TestGetAuthShouldReturnErrorWhenAuthUserConfigDoesntExists(c *C) {
	getAuth()
	u := authUser
	defer config.Set("nova:user", u)
	defer func() { authUser = u }()
	authUser = ""
	config.Unset("nova:user")
	err := getAuth()
	c.Assert(err, Not(IsNil))
}

func (s *S) TestGetAuthShouldReturnErrorWhenAuthPassConfigDoesntExists(c *C) {
	defer config.Set("nova:password", "tsuru")
	defer func() { authPass = "tsuru" }()
	authPass = ""
	config.Unset("nova:password")
	err := getAuth()
	c.Assert(err, Not(IsNil))
}

func (s *S) TestGetClientShouldReturnClientWithAuthConfs(c *C) {
	err := getClient()
	c.Assert(err, IsNil)
	req := string(requestJson)
	c.Assert(req, Not(Equals), "")
	expected := fmt.Sprintf(`{"auth": {"passwordCredentials": {"username": "%s", "password":"%s"}, "tenantName": "%s"}}`, authUser, authPass, authTenant)
	c.Assert(req, Equals, expected)
}

func (s *S) TestGetClientShouldNotResetClient(c *C) {
	called["token"] = false
	Client.Token = ""
	err := getClient()
	c.Assert(err, IsNil)
	c.Assert(called["token"], Equals, true)
	called["token"] = false
	err = getClient()
	c.Assert(err, IsNil)
	c.Assert(called["token"], Equals, false)
}

// func (s *S) TestNewTenantUsesNovaUserPasswordAndTenantFromTsuruConf(c *C) {
// 	ts := s.mockServer(`{"tenant": {"id": "uuid123", "name": "tenant name", "description": "tenant desc"}}`, "")
// 	defer ts.Close()
// 	a := App{Name: "myapp"}
// 	err := db.Session.Apps().Insert(a)
// 	c.Assert(err, IsNil)
// 	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
// 	_, err = NewTenant(&a)
// 	c.Assert(err, IsNil)
// 	req := string(requestJson)
// 	c.Assert(req, Not(Equals), "")
// 	expected := fmt.Sprintf(`{"auth": {"passwordCredentials": {"username": "%s", "password":"%s"}, "tenantName": "%s"}}`, authUser, authPass, authTenant)
// 	c.Assert(req, Equals, expected)
// }

func (s *S) TestNewKeystoneEnv(c *C) {
	s.ts.Close()
	tenantBody := `{"tenant": {"id": "uuid123", "name": "still", "description": "tenant desc"}}`
	userBody := `{"user": {"id": "uuid321", "name": "still", "email": "appname@foo.bar"}}`
	ec2Body := `{"credential": {"access": "access-key-here", "secret": "secret-key-here"}}`
	ts := s.mockServer(tenantBody, userBody, ec2Body, "")
	authUrl = ts.URL
	password := make([]byte, 64)
	for i := 0; i < len(password); i++ {
		password[i] = 'a'
	}
	fsystem := &testing.RecordingFs{FileContent: string(password)}
	defer func() {
		ts.Close()
		authUrl = ""
		fsystem = s.rfs
	}()
	env, err := newKeystoneEnv("still")
	c.Assert(err, IsNil)
	c.Assert(env.TenantId, Equals, "uuid123")
	c.Assert(env.UserId, Equals, "uuid321")
	c.Assert(env.AccessKey, Equals, "access-key-here")
	c.Assert(env.secretKey, Equals, "secret-key-here")
}

func (s *S) TestDestroyKeystoneEnv(c *C) {
	s.ts.Close()
	ts := s.mockServer("", "", "", "")
	authUrl = ts.URL
	defer func() {
		ts.Close()
		authUrl = ""
	}()
	k := KeystoneEnv{
		TenantId:  "e60d1f0a-ee74-411c-b879-46aee9502bf9",
		UserId:    "1b4d1195-7890-4274-831f-ddf8141edecc",
		AccessKey: "91232f6796b54ca2a2b87ef50548b123",
	}
	err := destroyKeystoneEnv(&k)
	c.Assert(err, IsNil)
	c.Assert(called["delete-ec2-creds"], Equals, true)
	c.Assert(params["ec2-access"], Equals, k.AccessKey)
	c.Assert(params["ec2-user"], Equals, k.UserId)
	c.Assert(called["delete-user"], Equals, true)
	c.Assert(params["user"], Equals, k.UserId)
	c.Assert(called["delete-tenant"], Equals, true)
	c.Assert(params["tenant"], Equals, k.TenantId)
}

func (s *S) TestDestroyKeystoneEnvWithoutEc2Creds(c *C) {
	k := KeystoneEnv{
		TenantId: "e60d1f0a-ee74-411c-b879-46aee9502bf9",
		UserId:   "1b4d1195-7890-4274-831f-ddf8141edecc",
	}
	err := destroyKeystoneEnv(&k)
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "Missing EC2 credentials.")
}

func (s *S) TestDestroyKeystoneEnvWithoutUserId(c *C) {
	k := KeystoneEnv{
		TenantId:  "e60d1f0a-ee74-411c-b879-46aee9502bf9",
		AccessKey: "91232f6796b54ca2a2b87ef50548b123",
	}
	err := destroyKeystoneEnv(&k)
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "Missing user.")
}

func (s *S) TestDestroyKeystoneEnvWithoutTenantId(c *C) {
	k := KeystoneEnv{
		UserId:    "1b4d1195-7890-4274-831f-ddf8141edecc",
		AccessKey: "91232f6796b54ca2a2b87ef50548b123",
	}
	err := destroyKeystoneEnv(&k)
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "Missing tenant.")
}
