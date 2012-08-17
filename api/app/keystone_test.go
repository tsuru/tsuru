package app

import (
	"fmt"
	"github.com/timeredbull/tsuru/config"
	"github.com/timeredbull/tsuru/db"
	"io/ioutil"
	"labix.org/v2/mgo/bson"
	. "launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
	"strings"
)

var (
	requestJson []byte
	// flags to detect when tenant url and user url are called
	tCalled, uCalled, credCalled bool
)

func (s *S) mockServer(b string) *httptest.Server {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/tokens" {
			body, err := ioutil.ReadAll(r.Body)
			defer r.Body.Close()
			if err != nil {
				panic(err)
			}
			requestJson = body
			w.Write([]byte(`{"access": {"token": {"id": "token-id-987"}}}`))
		} else if r.Method == "POST" && r.URL.Path == "/tenants" {
			tCalled = true
			w.Write([]byte(b))
		} else if r.Method == "POST" && r.URL.Path == "/users" {
			uCalled = true
			w.Write([]byte(b))
		} else if r.Method == "POST" && strings.Contains(r.URL.Path, "/credentials/OS-EC2") {
			credCalled = true
			w.Write([]byte(b))
		}
	}))
	authUrl = ts.URL
	return ts
}

func (s *S) TestGetAuth(c *C) {
	expectedUrl, err := config.GetString("nova:auth-url")
	c.Assert(err, IsNil)
	getAuth()
	c.Assert(authUrl, Equals, expectedUrl)
}

func (s *S) TestGetAuthShouldNotGetTheConfigEveryTimeItsCalled(c *C) {
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
	ts := s.mockServer("")
	defer ts.Close()
	err := getClient()
	c.Assert(err, IsNil)
	req := string(requestJson)
	c.Assert(req, Not(Equals), "")
	expected := fmt.Sprintf(`{"auth": {"passwordCredentials": {"username": "%s", "password":"%s"}, "tenantName": "%s"}}`, authUser, authPass, authTenant)
	c.Assert(req, Equals, expected)
}

func (s *S) TestNewTenantCallsKeystoneApi(c *C) {
	ts := s.mockServer(`{"tenant": {"id": "uuid123", "name": "tenant name", "description": "tenant desc"}}`)
	defer ts.Close()
	a := App{Name: "myapp"}
	err := db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	_, err = NewTenant(&a)
	c.Assert(err, IsNil)
	c.Assert(tCalled, Equals, true)
}

func (s *S) TestNewTenantSavesInDb(c *C) {
	ts := s.mockServer(`{"tenant": {"id": "uuid123", "name": "tenant name", "description": "tenant desc"}}`)
	defer ts.Close()
	a := App{Name: "myapp"}
	err := db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	t, err := NewTenant(&a)
	c.Assert(err, IsNil)
	err = a.Get()
	c.Assert(err, IsNil)
	c.Assert(t, Equals, "uuid123")
	c.Assert(a.KeystoneEnv.TenantId, DeepEquals, t)
}

func (s *S) TestNewTenantUsesNovaUserPasswordAndTenantFromTsuruConf(c *C) {
	ts := s.mockServer(`{"tenant": {"id": "uuid123", "name": "tenant name", "description": "tenant desc"}}`)
	defer ts.Close()
	a := App{Name: "myapp"}
	err := db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	_, err = NewTenant(&a)
	c.Assert(err, IsNil)
	req := string(requestJson)
	c.Assert(req, Not(Equals), "")
	expected := fmt.Sprintf(`{"auth": {"passwordCredentials": {"username": "%s", "password":"%s"}, "tenantName": "%s"}}`, authUser, authPass, authTenant)
	c.Assert(req, Equals, expected)
}

func (s *S) TestNewUserCallsKeystoneApi(c *C) {
	ts := s.mockServer(`{"user": {"id": "uuid321", "name": "appname", "email": "appname@foo.bar"}}`)
	defer ts.Close()
	a := App{Name: "myapp"}
	a.KeystoneEnv.TenantId = "uuid123"
	err := db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	_, err = NewUser(&a)
	c.Assert(err, IsNil)
	c.Assert(uCalled, Equals, true)
}

func (s *S) TestNewUserShouldFailIfAppHasNoTenantId(c *C) {
	ts := s.mockServer(`{"user": {"id": "uuid321", "name": "appname", "email": "appname@foo.bar"}}`)
	defer ts.Close()
	a := App{Name: "myapp"}
	err := db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	_, err = NewUser(&a)
	c.Assert(err, ErrorMatches, "^App should have an associated keystone tenant to create an user.$")
}

func (s *S) TestNewUserShouldStoreUserInDb(c *C) {
	ts := s.mockServer(`{"user": {"id": "uuid321", "name": "appname", "email": "appname@foo.bar"}}`)
	defer ts.Close()
	a := App{Name: "myapp"}
	a.KeystoneEnv.TenantId = "uuid123"
	err := db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	uId, err := NewUser(&a)
	c.Assert(err, IsNil)
	c.Assert(uId, Equals, "uuid321")
	db.Session.Apps().Find(bson.M{"name": a.Name}).One(&a)
	c.Assert(a.KeystoneEnv.UserId, Equals, uId)
}

func (s *S) TestNewEC2CredsShouldCallKeystoneApi(c *C) {
	ts := s.mockServer(`{"credential": {"access": "access-key-here", "secret": "secret-key-here"}}`)
	defer ts.Close()
	a := App{Name: "myapp"}
	a.KeystoneEnv.TenantId = "uuid123"
	a.KeystoneEnv.UserId = "uuid321"
	err := db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	_, _, err = NewEC2Creds(&a)
	c.Assert(err, IsNil)
	c.Assert(credCalled, Equals, true)
}

func (s *S) TestNewEC2CredsShouldFailIfAppHasNoTenantId(c *C) {
	ts := s.mockServer(`{"credential": {"access": "access-key-here", "secret": "secret-key-here"}}`)
	defer ts.Close()
	a := App{Name: "myapp"}
	a.KeystoneEnv.UserId = "uuid321"
	err := db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	_, _, err = NewEC2Creds(&a)
	c.Assert(err, ErrorMatches, "^App should have an associated keystone tenant to create an user.$")
}

func (s *S) TestNewEC2CredsShouldFailIfAppHasNoUserId(c *C) {
	ts := s.mockServer(`{"credential": {"access": "access-key-here", "secret": "secret-key-here"}}`)
	defer ts.Close()
	a := App{Name: "myapp"}
	a.KeystoneEnv.TenantId = "uuid123"
	err := db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	_, _, err = NewEC2Creds(&a)
	c.Assert(err, ErrorMatches, "^App should have an associated keystone user to create an user.$")
}

func (s *S) TestNewEC2CredsShouldSaveAccessKeyInDbAndReturnAccessAndSecretKeys(c *C) {
	ts := s.mockServer(`{"credential": {"access": "access-key-here", "secret": "secret-key-here"}}`)
	defer ts.Close()
	a := App{Name: "myapp"}
	a.KeystoneEnv.TenantId = "uuid123"
	a.KeystoneEnv.UserId = "uuid321"
	err := db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	aKey, sKey, err := NewEC2Creds(&a)
	c.Assert(err, IsNil)
	c.Assert(aKey, Equals, "access-key-here")
	c.Assert(sKey, Equals, "secret-key-here")
	err = db.Session.Apps().Find(bson.M{"name": a.Name}).One(&a)
	c.Assert(err, IsNil)
	c.Assert(a.KeystoneEnv.AccessKey, Equals, aKey)
}
