// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package oauth

import (
	"bytes"
	"io/ioutil"
	"net/http"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/repository/repositorytest"
	"golang.org/x/oauth2"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

func (s *S) TestOAuthLoginWithoutCode(c *check.C) {
	scheme := OAuthScheme{}
	params := make(map[string]string)
	params["redirectUrl"] = "http://localhost"
	_, err := scheme.Login(params)
	c.Assert(err, check.Equals, ErrMissingCodeError)
}

func (s *S) TestOAuthLoginWithoutRedirectUrl(c *check.C) {
	scheme := OAuthScheme{}
	params := make(map[string]string)
	params["code"] = "abcdefg"
	_, err := scheme.Login(params)
	c.Assert(err, check.Equals, ErrMissingCodeRedirectUrl)
}

func (s *S) TestOAuthLogin(c *check.C) {
	scheme := OAuthScheme{}
	s.rsps["/token"] = `access_token=my_token`
	s.rsps["/user"] = `{"email":"rand@althor.com"}`
	params := make(map[string]string)
	params["code"] = "abcdefg"
	params["redirectUrl"] = "http://localhost"
	token, err := scheme.Login(params)
	c.Assert(err, check.IsNil)
	c.Assert(token.GetValue(), check.Equals, "my_token")
	c.Assert(token.GetUserName(), check.Equals, "rand@althor.com")
	c.Assert(token.IsAppToken(), check.Equals, false)
	u, err := token.User()
	c.Assert(err, check.IsNil)
	c.Assert(u.Email, check.Equals, "rand@althor.com")
	c.Assert(len(s.reqs), check.Equals, 2)
	c.Assert(s.reqs[0].URL.Path, check.Equals, "/token")
	c.Assert(s.bodies[0], check.Equals, "client_id=clientid&code=abcdefg&grant_type=authorization_code&redirect_uri=http%3A%2F%2Flocalhost&scope=myscope")
	c.Assert(s.reqs[1].URL.Path, check.Equals, "/user")
	c.Assert(s.reqs[1].Header.Get("Authorization"), check.Equals, "Bearer my_token")
	dbToken, err := getToken("my_token")
	c.Assert(err, check.IsNil)
	c.Assert(dbToken.AccessToken, check.Equals, "my_token")
	c.Assert(dbToken.UserEmail, check.Equals, "rand@althor.com")
	c.Assert(dbToken.Scope, check.Equals, "myscope")
}

func (s *S) TestOAuthLoginScope(c *check.C) {
	scheme := OAuthScheme{}
	s.rsps["/token"] = `access_token=my_token`
	s.rsps["/user"] = `{"email":"rand@althor.com"}`
	params := make(map[string]string)
	params["code"] = "abcdefg"
	params["redirectUrl"] = "http://localhost"
	params["scope"] = "user-create team-user-create"
	token, err := scheme.Login(params)
	c.Assert(err, check.IsNil)
	c.Assert(token.GetValue(), check.Equals, "my_token")
	c.Assert(token.GetUserName(), check.Equals, "rand@althor.com")
	c.Assert(token.IsAppToken(), check.Equals, false)
	u, err := token.User()
	c.Assert(err, check.IsNil)
	c.Assert(u.Email, check.Equals, "rand@althor.com")
	c.Assert(len(s.reqs), check.Equals, 2)
	c.Assert(s.reqs[0].URL.Path, check.Equals, "/token")
	c.Assert(s.bodies[0], check.Equals, "client_id=clientid&code=abcdefg&grant_type=authorization_code&redirect_uri=http%3A%2F%2Flocalhost&scope=user-create+team-user-create")
	c.Assert(s.reqs[1].URL.Path, check.Equals, "/user")
	c.Assert(s.reqs[1].Header.Get("Authorization"), check.Equals, "Bearer my_token")
	dbToken, err := getToken("my_token")
	c.Assert(err, check.IsNil)
	c.Assert(dbToken.AccessToken, check.Equals, "my_token")
	c.Assert(dbToken.UserEmail, check.Equals, "rand@althor.com")
	c.Assert(dbToken.Scope, check.Equals, "user-create team-user-create")
}

func (s *S) TestOAuthLoginRegistrationDisabled(c *check.C) {
	config.Set("auth:user-registration", false)
	defer config.Set("auth:user-registration", true)
	scheme := OAuthScheme{}
	s.rsps["/token"] = `access_token=my_token`
	s.rsps["/user"] = `{"email":"rand@althor.com"}`
	params := make(map[string]string)
	params["code"] = "abcdefg"
	params["redirectUrl"] = "http://localhost"
	_, err := scheme.Login(params)
	c.Assert(err, check.Equals, auth.ErrUserNotFound)
}

func (s *S) TestOAuthLoginEmptyToken(c *check.C) {
	scheme := OAuthScheme{}
	s.rsps["/token"] = `access_token=`
	params := make(map[string]string)
	params["code"] = "abcdefg"
	params["redirectUrl"] = "http://localhost"
	_, err := scheme.Login(params)
	c.Assert(err, check.Equals, ErrEmptyAccessToken)
	c.Assert(len(s.reqs), check.Equals, 1)
	c.Assert(s.reqs[0].URL.Path, check.Equals, "/token")
}

func (s *S) TestOAuthLoginEmptyEmail(c *check.C) {
	scheme := OAuthScheme{}
	s.rsps["/token"] = `access_token=my_token`
	s.rsps["/user"] = `{"email":""}`
	params := make(map[string]string)
	params["code"] = "abcdefg"
	params["redirectUrl"] = "http://localhost"
	_, err := scheme.Login(params)
	c.Assert(err, check.Equals, ErrEmptyUserEmail)
	c.Assert(len(s.reqs), check.Equals, 2)
	c.Assert(s.reqs[0].URL.Path, check.Equals, "/token")
	c.Assert(s.reqs[1].URL.Path, check.Equals, "/user")
}

func (s *S) TestOAuthName(c *check.C) {
	scheme := OAuthScheme{}
	name := scheme.Name()
	c.Assert(name, check.Equals, "oauth")
}

func (s *S) TestOAuthInfo(c *check.C) {
	scheme := OAuthScheme{}
	info, err := scheme.Info()
	c.Assert(err, check.IsNil)
	c.Assert(info["authorizeUrl"], check.Matches, s.server.URL+"/auth.*")
	c.Assert(info["authorizeUrl"], check.Matches, ".*client_id=clientid.*")
	c.Assert(info["authorizeUrl"], check.Matches, ".*redirect_uri=__redirect_url__.*")
	c.Assert(info["port"], check.Equals, "0")
}

func (s *S) TestOAuthInfoWithPort(c *check.C) {
	config.Set("auth:oauth:callback-port", 9009)
	defer config.Set("auth:oauth:callback-port", nil)
	scheme := OAuthScheme{}
	info, err := scheme.Info()
	c.Assert(err, check.IsNil)
	c.Assert(info["port"], check.Equals, "9009")
}

func (s *S) TestOAuthParse(c *check.C) {
	b := ioutil.NopCloser(bytes.NewBufferString(`{"email":"x@x.com"}`))
	rsp := &http.Response{Body: b}
	parser := OAuthParser(&OAuthScheme{})
	email, err := parser.Parse(rsp)
	c.Assert(err, check.IsNil)
	c.Assert(email, check.Equals, "x@x.com")
}

func (s *S) TestOAuthParseInvalid(c *check.C) {
	b := ioutil.NopCloser(bytes.NewBufferString(`{xxxxxxx}`))
	rsp := &http.Response{Body: b}
	parser := OAuthParser(&OAuthScheme{})
	_, err := parser.Parse(rsp)
	c.Assert(err, check.NotNil)
}

func (s *S) TestOAuthAuth(c *check.C) {
	existing := Token{Token: oauth2.Token{AccessToken: "myvalidtoken"}, UserEmail: "x@x.com"}
	err := existing.save()
	c.Assert(err, check.IsNil)
	scheme := OAuthScheme{}
	token, err := scheme.Auth("bearer myvalidtoken")
	c.Assert(err, check.IsNil)
	c.Assert(len(s.reqs), check.Equals, 1)
	c.Assert(s.reqs[0].URL.Path, check.Equals, "/user")
	c.Assert(token.GetValue(), check.Equals, "myvalidtoken")
}

func (s *S) TestOAuthAppLogin(c *check.C) {
	scheme := OAuthScheme{}
	token, err := scheme.AppLogin("myApp")
	c.Assert(err, check.IsNil)
	c.Assert(token.IsAppToken(), check.Equals, true)
	c.Assert(token.GetAppName(), check.Equals, "myApp")
}

func (s *S) TestOAuthAuthWithAppToken(c *check.C) {
	scheme := OAuthScheme{}
	appToken, err := scheme.AppLogin("myApp")
	c.Assert(err, check.IsNil)
	token, err := scheme.Auth("bearer " + appToken.GetValue())
	c.Assert(err, check.IsNil)
	c.Assert(len(s.reqs), check.Equals, 0)
	c.Assert(token.IsAppToken(), check.Equals, true)
	c.Assert(token.GetAppName(), check.Equals, "myApp")
	c.Assert(token.GetValue(), check.Equals, appToken.GetValue())
}

func (s *S) TestOAuthCreate(c *check.C) {
	scheme := OAuthScheme{}
	user := auth.User{Email: "x@x.com", Password: "something"}
	_, err := scheme.Create(&user)
	c.Assert(err, check.IsNil)
	dbUser, err := auth.GetUserByEmail(user.Email)
	c.Assert(err, check.IsNil)
	c.Assert(dbUser.Email, check.Equals, user.Email)
	c.Assert(dbUser.Password, check.Equals, "")
	c.Assert(repositorytest.Users(), check.DeepEquals, []string{user.Email})
}

func (s *S) TestOAuthRemove(c *check.C) {
	scheme := OAuthScheme{}
	s.rsps["/token"] = `access_token=my_token`
	s.rsps["/user"] = `{"email":"rand@althor.com"}`
	params := make(map[string]string)
	params["code"] = "abcdefg"
	params["redirectUrl"] = "http://localhost"
	token, err := scheme.Login(params)
	c.Assert(err, check.IsNil)
	u, err := token.User()
	c.Assert(err, check.IsNil)
	err = scheme.Remove(u)
	c.Assert(err, check.IsNil)
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	var tokens []Token
	coll := collection()
	defer coll.Close()
	err = coll.Find(bson.M{"useremail": "rand@althor.com"}).All(&tokens)
	c.Assert(err, check.IsNil)
	c.Assert(len(tokens), check.Equals, 0)
	_, err = auth.GetUserByEmail("rand@althor.com")
	c.Assert(err, check.Equals, auth.ErrUserNotFound)
}
