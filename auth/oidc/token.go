// Copyright 2023 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package oidc

import (
	"context"
	"encoding/json"

	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/permission"
	authTypes "github.com/tsuru/tsuru/types/auth"
)

var _ authTypes.Token = &jwtToken{}

type jwtToken struct {
	AuthUser *authTypes.User
	Raw      string
	Identity *extendedClaims
}

type extendedClaims struct {
	jwt.MapClaims `json:"-"`
	Email         string   `json:"email,omitempty"`
	Groups        []string `json:"groups,omitempty"`
}

func (f *extendedClaims) UnmarshalJSON(b []byte) error {
	claims := jwt.MapClaims{}

	err := json.Unmarshal(b, &claims)
	if err != nil {
		return err
	}

	if email, ok := claims["email"].(string); ok {
		f.Email = email
	}

	if groups, ok := claims["groups"].([]interface{}); ok {

		f.Groups = make([]string, len(groups))

		for i, group := range groups {
			f.Groups[i], _ = group.(string)
		}
	}
	delete(claims, "email")
	delete(claims, "groups")

	f.MapClaims = claims

	return err
}

func (t *jwtToken) GetValue() string {
	return t.Raw
}

func (t *jwtToken) User(ctx context.Context) (*authTypes.User, error) {
	return t.AuthUser, nil
}

func (t *jwtToken) GetUserName() string {
	return t.AuthUser.Email
}

func (t *jwtToken) Engine() string {
	return "oidc"
}

func (t *jwtToken) Permissions(ctx context.Context) ([]permission.Permission, error) {
	return auth.BaseTokenPermission(ctx, t)
}
