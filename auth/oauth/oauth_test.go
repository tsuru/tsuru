// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package oauth

import (
	"bytes"
	goauth2 "code.google.com/p/goauth2/oauth"
	"github.com/tsuru/config"
	"io/ioutil"
	"launchpad.net/gocheck"
	"net/http"
)

func (s *S) TestOAuthLoginWithoutCode(c *gocheck.C) {
	scheme := OAuthScheme{}
	params := make(map[string]string)
	params["redirectUrl"] = "http://localhost"
	_, err := scheme.Login(params)
	c.Assert(err, gocheck.Equals, ErrMissingCodeError)
}

func (s *S) TestOAuthLoginWithoutRedirectUrl(c *gocheck.C) {
	scheme := OAuthScheme{}
	params := make(map[string]string)
	params["code"] = "abcdefg"
	_, err := scheme.Login(params)
	c.Assert(err, gocheck.Equals, ErrMissingCodeRedirectUrl)
}

func (s *S) TestOAuthLogin(c *gocheck.C) {
	scheme := OAuthScheme{}
	s.rsps["/token"] = `access_token=my_token`
	s.rsps["/user"] = `{"email":"rand@althor.com"}`
	params := make(map[string]string)
	params["code"] = "abcdefg"
	params["redirectUrl"] = "http://localhost"
	token, err := scheme.Login(params)
	c.Assert(err, gocheck.IsNil)
	c.Assert(token.GetValue(), gocheck.Equals, "my_token")
	c.Assert(token.GetUserName(), gocheck.Equals, "rand@althor.com")
	c.Assert(token.IsAppToken(), gocheck.Equals, false)
	u, err := token.User()
	c.Assert(err, gocheck.IsNil)
	c.Assert(u.Email, gocheck.Equals, "rand@althor.com")
	c.Assert(len(s.reqs), gocheck.Equals, 2)
	c.Assert(s.reqs[0].URL.Path, gocheck.Equals, "/token")
	c.Assert(s.bodies[0], gocheck.Equals, "client_id=clientid&client_secret=clientsecret&code=abcdefg&grant_type=authorization_code&redirect_uri=http%3A%2F%2Flocalhost&scope=myscope")
	c.Assert(s.reqs[1].URL.Path, gocheck.Equals, "/user")
	c.Assert(s.reqs[1].Header.Get("Authorization"), gocheck.Equals, "Bearer my_token")
}

func (s *S) TestOAuthLoginEmptyToken(c *gocheck.C) {
	scheme := OAuthScheme{}
	s.rsps["/token"] = `access_token=`
	params := make(map[string]string)
	params["code"] = "abcdefg"
	params["redirectUrl"] = "http://localhost"
	_, err := scheme.Login(params)
	c.Assert(err, gocheck.Equals, ErrEmptyAccessToken)
	c.Assert(len(s.reqs), gocheck.Equals, 1)
	c.Assert(s.reqs[0].URL.Path, gocheck.Equals, "/token")
}

func (s *S) TestOAuthLoginEmptyEmail(c *gocheck.C) {
	scheme := OAuthScheme{}
	s.rsps["/token"] = `access_token=my_token`
	s.rsps["/user"] = `{"email":""}`
	params := make(map[string]string)
	params["code"] = "abcdefg"
	params["redirectUrl"] = "http://localhost"
	_, err := scheme.Login(params)
	c.Assert(err, gocheck.Equals, ErrEmptyUserEmail)
	c.Assert(len(s.reqs), gocheck.Equals, 2)
	c.Assert(s.reqs[0].URL.Path, gocheck.Equals, "/token")
	c.Assert(s.reqs[1].URL.Path, gocheck.Equals, "/user")
}

func (s *S) TestOAuthName(c *gocheck.C) {
	scheme := OAuthScheme{}
	name := scheme.Name()
	c.Assert(name, gocheck.Equals, "oauth")
}

func (s *S) TestOAuthInfo(c *gocheck.C) {
	scheme := OAuthScheme{}
	info, err := scheme.Info()
	c.Assert(err, gocheck.IsNil)
	c.Assert(info["authorizeUrl"], gocheck.Matches, s.server.URL+"/auth.*")
	c.Assert(info["authorizeUrl"], gocheck.Matches, ".*client_id=clientid.*")
	c.Assert(info["authorizeUrl"], gocheck.Matches, ".*redirect_uri=%25s.*")
	c.Assert(info["port"], gocheck.Equals, "")
}

func (s *S) TestOAuthInfoWithPort(c *gocheck.C) {
	config.Set("auth:oauth:callback-port", "9009")
	defer config.Set("auth:oauth:callback-port", nil)
	scheme := OAuthScheme{}
	info, err := scheme.Info()
	c.Assert(err, gocheck.IsNil)
	c.Assert(info["port"], gocheck.Equals, "9009")
}

func (s *S) TestOAuthParse(c *gocheck.C) {
	b := ioutil.NopCloser(bytes.NewBufferString(`{"email":"x@x.com"}`))
	rsp := &http.Response{Body: b}
	parser := OAuthParser(&OAuthScheme{})
	email, err := parser.Parse(rsp)
	c.Assert(err, gocheck.IsNil)
	c.Assert(email, gocheck.Equals, "x@x.com")
}

func (s *S) TestOAuthParseInvalid(c *gocheck.C) {
	b := ioutil.NopCloser(bytes.NewBufferString(`{xxxxxxx}`))
	rsp := &http.Response{Body: b}
	parser := OAuthParser(&OAuthScheme{})
	_, err := parser.Parse(rsp)
	c.Assert(err, gocheck.NotNil)
}

func (s *S) TestOAuthAuth(c *gocheck.C) {
	existing := Token{Token: goauth2.Token{AccessToken: "myvalidtoken"}, UserEmail: "x@x.com"}
	err := existing.save()
	c.Assert(err, gocheck.IsNil)
	scheme := OAuthScheme{}
	token, err := scheme.Auth("bearer myvalidtoken")
	c.Assert(err, gocheck.IsNil)
	c.Assert(len(s.reqs), gocheck.Equals, 1)
	c.Assert(s.reqs[0].URL.Path, gocheck.Equals, "/user")
	c.Assert(token.GetValue(), gocheck.Equals, "myvalidtoken")
}

func (s *S) TestOAuthAppLogin(c *gocheck.C) {
	scheme := OAuthScheme{}
	token, err := scheme.AppLogin("myApp")
	c.Assert(err, gocheck.IsNil)
	c.Assert(token.IsAppToken(), gocheck.Equals, true)
	c.Assert(token.GetAppName(), gocheck.Equals, "myApp")
}

func (s *S) TestOAuthAuthWithAppToken(c *gocheck.C) {
	scheme := OAuthScheme{}
	appToken, err := scheme.AppLogin("myApp")
	c.Assert(err, gocheck.IsNil)
	token, err := scheme.Auth("bearer " + appToken.GetValue())
	c.Assert(err, gocheck.IsNil)
	c.Assert(len(s.reqs), gocheck.Equals, 0)
	c.Assert(token.IsAppToken(), gocheck.Equals, true)
	c.Assert(token.GetAppName(), gocheck.Equals, "myApp")
	c.Assert(token.GetValue(), gocheck.Equals, appToken.GetValue())
}
