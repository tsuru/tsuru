// Copyright 2023 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package oidc

import (
	"context"
	"crypto/rsa"
	"errors"
	"fmt"
	"sync"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/auth"
	internalConfig "github.com/tsuru/tsuru/config"
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

func (s *oidcScheme) Name() string {
	return "oidc"
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

	// TODO: how to disable user

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

func (s *oidcScheme) Info(ctx context.Context) (auth.SchemeInfo, error) {
	return nil, errNotImplemented
}

func (s *oidcScheme) Create(ctx context.Context, user *auth.User) (*auth.User, error) {
	return nil, errNotImplemented
}

func (s *oidcScheme) Remove(ctx context.Context, user *auth.User) error {
	// TODO: logic deletion ?
	return errNotImplemented
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

		err = s.cache.Register(s.jwksURL, jwk.WithMinRefreshInterval(15*time.Minute))
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
