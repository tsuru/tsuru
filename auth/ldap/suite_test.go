// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ldap

import (
	"strings"
	"testing"

	"github.com/bradleypeabody/godap"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/auth/authtest"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	_ "github.com/tsuru/tsuru/storage/mongodb"
	authTypes "github.com/tsuru/tsuru/types/auth"
	"golang.org/x/crypto/bcrypt"
	"gopkg.in/check.v1"
)

func Test(t *testing.T) { check.TestingT(t) }

type S struct {
	conn   *db.Storage
	user   *auth.User
	team   *authTypes.Team
	server *authtest.SMTPServer
	token  auth.Token
}

var _ = check.Suite(&S{})

var nativeScheme = LdapNativeScheme{}

func runLdapServer(ldapServer *godap.LDAPServer) {
	err := ldapServer.ListenAndServe("127.0.0.1:10000")
	if err != nil {
		panic(err.Error())
	}
}

func (s *S) SetUpSuite(c *check.C) {
	config.Set("log:disable-syslog", true)
	config.Set("auth:token-expire-days", 2)
	config.Set("auth:hash-cost", bcrypt.MinCost)
	config.Set("database:url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("database:name", "tsuru_auth_native_test")
	var err error
	s.server, err = authtest.NewSMTPServer()
	c.Assert(err, check.IsNil)
	config.Set("smtp:server", s.server.Addr())
	config.Set("smtp:user", "root")

	// Mockup LDAP server
	hs := make([]godap.LDAPRequestHandler, 0)

	// use a LDAPBindFuncHandler to provide a callback function to respond
	// to bind requests
	hs = append(hs, &godap.LDAPBindFuncHandler{LDAPBindFunc: func(binddn string, bindpw []byte) bool {
		if strings.HasPrefix(binddn, "cn=admin,") && string(bindpw) == "blew" {
			return true
		}
		if (strings.HasPrefix(binddn, "cn=timeredbull,") ||
			strings.HasPrefix(binddn, "cn=para,") ||
			strings.HasPrefix(binddn, "cn=wolverine,")) && string(bindpw) == "123456" {
			return true
		}
		return false
	}})

	// use a LDAPSimpleSearchFuncHandler to reply to search queries
	hs = append(hs, &godap.LDAPSimpleSearchFuncHandler{LDAPSimpleSearchFunc: func(req *godap.LDAPSimpleSearchRequest) []*godap.LDAPSimpleSearchResultEntry {

		ret := make([]*godap.LDAPSimpleSearchResultEntry, 0, 1)

		// here we produce a single search result that matches whatever
		// they are searching for
		if req.FilterAttr == "email" {
			ret = append(ret, &godap.LDAPSimpleSearchResultEntry{
				DN: "cn=" + strings.Split(req.FilterValue, "@")[0] + "," + req.BaseDN,
				Attrs: map[string]interface{}{
					"sn":            strings.Split(req.FilterValue, "@")[0],
					"cn":            strings.Split(req.FilterValue, "@")[0],
					"uid":           strings.Split(req.FilterValue, "@")[0],
					"email":         req.FilterValue,
					"homeDirectory": "/home/" + req.FilterValue,
					"objectClass": []string{
						"top",
						"posixAccount",
						"inetOrgPerson",
					},
				},
			})
		} else if req.FilterAttr == "memberUid" {
			ret = append(ret, &godap.LDAPSimpleSearchResultEntry{})
		}

		return ret

	}})

	ldapServer := &godap.LDAPServer{
		Handlers: hs,
	}

	go runLdapServer(ldapServer)

	config.Set("auth:ldap:host", "127.0.0.1")
	config.Set("auth:ldap:port", "10000")
	config.Set("auth:ldap:skiptls", true)
	config.Set("auth:ldap:usessl", false)
	config.Set("auth:ldap:basedn", "dc=mamail,dc=ltd")
	config.Set("auth:ldap:binddn", "cn=admin,dc=mamail,dc=ltd")
	config.Set("auth:ldap:bindpassword", "blew")
}

func (s *S) SetUpTest(c *check.C) {
	s.conn, _ = db.Conn()
	s.user = &auth.User{Email: "timeredbull@globo.com", Password: "123456"}
	_, err := nativeScheme.Create(s.user)
	c.Assert(err, check.IsNil)
	s.token, err = nativeScheme.Login(map[string]string{"email": s.user.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
	s.team = &authTypes.Team{Name: "cobrateam"}
}

func (s *S) TearDownTest(c *check.C) {
	err := dbtest.ClearAllCollections(s.conn.Users().Database)
	c.Assert(err, check.IsNil)
	s.conn.Close()
	cost = 0
	tokenExpire = 0
}

func (s *S) TearDownSuite(c *check.C) {
	s.server.Stop()
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	conn.Apps().Database.DropDatabase()
}
