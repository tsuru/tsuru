// Copyright 2023 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package oidc

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/auth"
	check "gopkg.in/check.v1"
)

func Test(t *testing.T) { check.TestingT(t) }

type AuthSuite struct {
	scheme *oidcScheme

	jwkKeySet      jwk.Set
	fakeJWKSServer *httptest.Server
}

var _ = check.Suite(&AuthSuite{})

func (s *AuthSuite) SetUpSuite(c *check.C) {
	s.scheme = &oidcScheme{}

	s.jwkKeySet = jwk.NewSet()
	s.fakeJWKSServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(s.jwkKeySet)
	}))

	config.Set("database:url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("database:name", "oidc_tests")

	config.Set("auth:oidc:jwks-url", s.fakeJWKSServer.URL)
	config.Set("auth:user-registration", true)

	err := s.scheme.lazyInitialize(context.Background())
	c.Check(err, check.IsNil)
}

func (s *AuthSuite) TearDownSuite(c *check.C) {
	s.fakeJWKSServer.Close()
	config.Unset("auth:oidc:jwks-url")
}

func (s *AuthSuite) TestLoginNoJWKSURLDefined(c *check.C) {
	scheme := oidcScheme{
		jwksURL:     "",
		initialized: sync.Once{},
	}

	scheme.initialized.Do(func() {})

	token, err := scheme.Auth(context.TODO(), "TOKEN")
	c.Assert(err, check.ErrorMatches, "no jwks URLs")
	c.Assert(token, check.IsNil)
}

func (s *AuthSuite) TestNotImplementUserScheme(c *check.C) {
	var scheme auth.Scheme = &oidcScheme{}

	_, implements := scheme.(auth.UserScheme)
	c.Assert(implements, check.Equals, false)
}

func (s *AuthSuite) TestLoginWithRSAKey(c *check.C) {
	kid := "rsa-test-123"
	privateRSAKey, err := s.generateNewPrivateRSAKey(kid)
	c.Assert(err, check.IsNil)

	userEmail := "bar@company.com"
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"email": userEmail,
	})
	token.Header["kid"] = kid
	tokenString, err := token.SignedString(privateRSAKey)
	c.Assert(err, check.IsNil)

	tsuruToken, err := s.scheme.Auth(context.TODO(), tokenString)
	c.Assert(err, check.IsNil)
	c.Assert(tsuruToken, check.Not(check.IsNil))
	c.Assert(tsuruToken.GetUserName(), check.Equals, userEmail)

	authUser, err := auth.GetUserByEmail(context.TODO(), userEmail)
	c.Assert(err, check.IsNil)
	c.Assert(authUser.Email, check.Equals, userEmail)
}

func (s *AuthSuite) TestLoginWithRSAKeyWithBearer(c *check.C) {
	kid := "rsa-test-bearer-123"
	privateRSAKey, err := s.generateNewPrivateRSAKey(kid)
	c.Assert(err, check.IsNil)

	userEmail := "bar@company.com"
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"email": userEmail,
	})
	token.Header["kid"] = kid
	tokenString, err := token.SignedString(privateRSAKey)
	c.Assert(err, check.IsNil)

	tsuruToken, err := s.scheme.Auth(context.TODO(), "Bearer "+tokenString)
	c.Assert(err, check.IsNil)
	c.Assert(tsuruToken, check.Not(check.IsNil))
	c.Assert(tsuruToken.GetUserName(), check.Equals, userEmail)

	authUser, err := auth.GetUserByEmail(context.TODO(), userEmail)
	c.Assert(err, check.IsNil)
	c.Assert(authUser.Email, check.Equals, userEmail)
}

func (s *AuthSuite) TestLoginWithGroupsRSAKey(c *check.C) {
	s.scheme.groupsInClaims = true
	defer func() {
		s.scheme.groupsInClaims = false
	}()
	kid := "rsa-with-groups-123"
	privateRSAKey, err := s.generateNewPrivateRSAKey(kid)
	c.Assert(err, check.IsNil)

	userEmail := "bar@company.com"
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"email":  userEmail,
		"groups": []string{"group1", "group2"},
	})
	token.Header["kid"] = kid
	tokenString, err := token.SignedString(privateRSAKey)
	c.Assert(err, check.IsNil)

	tsuruToken, err := s.scheme.Auth(context.TODO(), tokenString)
	c.Assert(err, check.IsNil)
	c.Assert(tsuruToken, check.Not(check.IsNil))
	c.Assert(tsuruToken.GetUserName(), check.Equals, userEmail)

	authUser, err := auth.GetUserByEmail(context.TODO(), userEmail)
	c.Assert(err, check.IsNil)
	c.Assert(authUser.Email, check.Equals, userEmail)
	c.Assert(authUser.Groups, check.DeepEquals, []string{"group1", "group2"})
}

func (s *AuthSuite) TestLoginWithMissingEmail(c *check.C) {
	kid := "rsa-missing-email"
	privateRSAKey, err := s.generateNewPrivateRSAKey(kid)
	c.Assert(err, check.IsNil)

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{})
	token.Header["kid"] = kid
	tokenString, err := token.SignedString(privateRSAKey)
	c.Assert(err, check.IsNil)

	_, err = s.scheme.Auth(context.TODO(), tokenString)
	c.Assert(err, check.Not(check.IsNil))
	c.Assert(err.Error(), check.Equals, "email claim is missing")
}

func (s *AuthSuite) TestLoginWithClaimValidation(c *check.C) {
	s.scheme.validClaims = map[string]interface{}{
		"azp": "my-azp",
	}
	defer func() {
		s.scheme.validClaims = nil
	}()

	kid := "rsa-azp-validation"
	privateRSAKey, err := s.generateNewPrivateRSAKey(kid)
	c.Assert(err, check.IsNil)

	invalidToken := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"azp":   "invalid-azp",
		"email": "bar@company.com",
	})
	invalidToken.Header["kid"] = kid
	tokenString, err := invalidToken.SignedString(privateRSAKey)
	c.Assert(err, check.IsNil)

	_, err = s.scheme.Auth(context.TODO(), tokenString)
	c.Assert(err, check.Not(check.IsNil))
	c.Assert(err.Error(), check.Equals, "invalid claim \"azp\"")

	validToken := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"azp":   "my-azp",
		"email": "bar@company.com",
	})
	validToken.Header["kid"] = kid
	tokenString, err = validToken.SignedString(privateRSAKey)
	c.Assert(err, check.IsNil)

	_, err = s.scheme.Auth(context.TODO(), tokenString)
	c.Assert(err, check.IsNil)
}

func (s *AuthSuite) TestLoginWithDisabledUser(c *check.C) {
	kid := "rsa-disabled-user"
	privateRSAKey, err := s.generateNewPrivateRSAKey(kid)
	c.Assert(err, check.IsNil)

	userEmail := "disabled-user-test@company.com"

	user := &auth.User{Email: userEmail, Disabled: true}
	user.Delete(context.TODO()) // remove from previous crashed tests
	err = user.Create(context.TODO())
	c.Assert(err, check.IsNil)

	defer user.Delete(context.TODO())

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"email": userEmail,
	})
	token.Header["kid"] = kid
	tokenString, err := token.SignedString(privateRSAKey)
	c.Assert(err, check.IsNil)

	tsuruToken, err := s.scheme.Auth(context.TODO(), tokenString)
	c.Assert(err, check.Equals, auth.ErrUserDisabled)
	c.Assert(tsuruToken, check.IsNil)
}

func (s *AuthSuite) generateNewPrivateRSAKey(kid string) (*rsa.PrivateKey, error) {
	sharedKey := make([]byte, 2048)
	_, err := io.ReadFull(rand.Reader, sharedKey)
	if err != nil {
		return nil, err
	}

	privateRSAKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}

	jwkKey, err := jwk.FromRaw(privateRSAKey.PublicKey)
	if err != nil {
		return nil, err
	}
	jwkKey.Set(jwk.KeyUsageKey, "sig")
	jwkKey.Set(jwk.KeyIDKey, kid)

	err = s.jwkKeySet.AddKey(jwkKey)
	if err != nil {
		return nil, err
	}

	_, err = s.scheme.cache.Refresh(context.Background(), s.scheme.jwksURL)
	if err != nil {
		return nil, err
	}
	return privateRSAKey, nil
}
