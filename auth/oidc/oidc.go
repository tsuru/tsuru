// Copyright 2023 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package oidc

import (
	"context"
	"crypto/rsa"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/auth"
	internalConfig "github.com/tsuru/tsuru/config"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/set"
	authTypes "github.com/tsuru/tsuru/types/auth"
)

var (
	errNoJWKSURLS                    = errors.New("no jwks URLs")
	errMissingEmailClaim             = errors.New("email claim is missing")
	errNotImplemented                = errors.New("not implemented")
	_                    auth.Scheme = &oidcScheme{}
)

type errKIDNotFound struct {
	kid string
}

func (err *errKIDNotFound) Error() string {
	return fmt.Sprintf("unable to find key %q", err.kid)
}

type errInvalidClaim struct {
	claim string
}

func (err *errInvalidClaim) Error() string {
	return fmt.Sprintf("invalid claim %q", err.claim)
}

func init() {
	auth.RegisterScheme("oidc", &oidcScheme{})
}

type oidcScheme struct {
	jwksURL             string
	cache               *jwk.Cache
	validClaims         map[string]interface{}
	initialized         sync.Once
	registrationEnabled bool
	groupsInClaims      bool
}

func (s *oidcScheme) Login(ctx context.Context, params map[string]string) (auth.Token, error) {
	return nil, nil
}

func (s *oidcScheme) Logout(ctx context.Context, token string) error {
	return nil
}

func (s *oidcScheme) Auth(ctx context.Context, token string) (auth.Token, error) {
	err := s.lazyInitialize(ctx)
	if err != nil {
		return nil, err
	}

	if s.jwksURL == "" {
		return nil, errNoJWKSURLS
	}

	identity := &extendedClaims{}

	if strings.HasPrefix(token, "Bearer ") || strings.HasPrefix(token, "bearer ") {
		token = token[len("Bearer "):]
	}

	parsedJWTToken, err := jwt.ParseWithClaims(token, identity, s.jwtGetKey(ctx))
	if err != nil {
		return nil, err
	}

	if !parsedJWTToken.Valid {
		return nil, errors.New("Token invalid")
	}

	if len(s.validClaims) > 0 {
		for claim, value := range s.validClaims {
			if identity.MapClaims[claim] != value {
				return nil, &errInvalidClaim{claim}
			}
		}
	}

	if identity.Email == "" {
		return nil, errMissingEmailClaim
	}

	user, err := auth.GetUserByEmail(identity.Email)
	if err == authTypes.ErrUserNotFound {
		if s.registrationEnabled {
			user = &auth.User{Email: identity.Email, Groups: identity.Groups}
			err = user.Create()
			if err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}

	if user.Disabled {
		return nil, auth.ErrUserDisabled
	}

	if s.groupsInClaims {
		dbGroups := set.FromSlice(user.Groups)
		providerGroups := set.FromSlice(identity.Groups)
		if !dbGroups.Equal(providerGroups) {
			user.Groups = identity.Groups
			err = user.Update()
			if err != nil {
				return nil, err
			}
		}
	}

	authUser, _ := auth.ConvertOldUser(user, nil)

	return &jwtToken{
		AuthUser: authUser,
		Identity: identity,
		Raw:      token,
	}, nil
}

func (s *oidcScheme) Info(ctx context.Context) (*auth.SchemeInfo, error) {
	clientID, err := config.GetString("auth:oidc:client-id")
	if err != nil {
		return nil, err
	}
	scopes, err := config.GetList("auth:oidc:scopes")
	if err != nil {
		return nil, err
	}
	authURL, err := config.GetString("auth:oidc:auth-url")
	if err != nil {
		return nil, err
	}
	tokenURL, err := config.GetString("auth:oidc:token-url")
	if err != nil {
		return nil, err
	}
	callbackPort, err := config.GetInt("auth:oidc:callback-port")
	if err != nil {
		log.Debugf("auth:oidc:callback-port not found using random port: %s", err)
	}
	return &auth.SchemeInfo{
		Name: "oidc",
		Data: map[string]interface{}{
			"clientID": clientID,
			"authURL":  authURL,
			"tokenURL": tokenURL,
			"scopes":   scopes,
			"port":     strconv.Itoa(callbackPort),
		},
	}, nil
}

func (s *oidcScheme) Create(ctx context.Context, user *auth.User) (*auth.User, error) {
	return nil, errNotImplemented
}

func (s *oidcScheme) Remove(ctx context.Context, user *auth.User) error {
	user.Disabled = true
	return user.Update()
}

func (s *oidcScheme) lazyInitialize(ctx context.Context) error {
	var err error
	s.initialized.Do(func() {
		s.cache = jwk.NewCache(context.Background())
		s.jwksURL, _ = config.GetString("auth:oidc:jwks-url")
		if s.jwksURL == "" {
			return
		}

		s.registrationEnabled, _ = config.GetBool("auth:user-registration")
		s.groupsInClaims, _ = config.GetBool("auth:oidc:groups-in-claims")

		s.validClaims = map[string]interface{}{}
		internalConfig.UnmarshalConfig("auth:oidc:valid-claims", &s.validClaims)

		var refreshIntervalErr error
		var refreshInterval time.Duration
		refreshInterval, refreshIntervalErr = config.GetDuration("auth:oidc:jwks-refresh-interval")
		if refreshIntervalErr != nil {
			log.Errorf(`Failed to fetch "auth:oidc:jwks-refresh-interval", falling on default setting (15m), error: %s`, refreshIntervalErr.Error())
		}
		if refreshInterval == 0 {
			refreshInterval = 15 * time.Minute
		}

		err = s.cache.Register(s.jwksURL, jwk.WithMinRefreshInterval(refreshInterval))
		if err != nil {
			return
		}
		_, err = s.cache.Refresh(ctx, s.jwksURL)
		if err != nil {
			return
		}
	})
	return err
}

func (s *oidcScheme) jwtGetKey(ctx context.Context) jwt.Keyfunc {
	return func(token *jwt.Token) (interface{}, error) {
		var err error

		jwkKeySet, err := s.cache.Get(ctx, s.jwksURL)
		if err != nil {
			return nil, err
		}

		keyID, ok := token.Header["kid"].(string)
		if !ok {
			keyID = "default"
		}

		jwkKey, found := jwkKeySet.LookupKeyID(keyID)
		if !found {
			return nil, &errKIDNotFound{keyID}
		}

		// TODO: support other kind of keys
		raw := &rsa.PublicKey{}
		err = jwkKey.Raw(raw)
		if err != nil {
			return nil, err
		}
		return raw, nil
	}
}
