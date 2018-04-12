// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"github.com/globalsign/mgo"
	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/permission"
	authTypes "github.com/tsuru/tsuru/types/auth"
)

type APIToken struct {
	Token     string `json:"token" bson:"apikey"`
	UserEmail string `json:"email" bson:"email"`
}

func (t *APIToken) GetValue() string {
	return t.Token
}

func (t *APIToken) User() (*authTypes.User, error) {
	return ConvertOldUser(GetUserByEmail(t.UserEmail))
}

func (t *APIToken) IsAppToken() bool {
	return false
}

func (t *APIToken) GetUserName() string {
	return t.UserEmail
}

func (t *APIToken) GetAppName() string {
	return ""
}

func (t *APIToken) Permissions() ([]permission.Permission, error) {
	return BaseTokenPermission(t)
}

func getAPIToken(header string) (*APIToken, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var t APIToken
	token, err := ParseToken(header)
	if err != nil {
		return nil, err
	}
	err = conn.Users().Find(bson.M{"apikey": token}).One(&t)
	if err != nil {
		if err == mgo.ErrNotFound {
			return nil, ErrInvalidToken
		}
		return nil, err
	}
	return &t, nil
}

func APIAuth(token string) (*APIToken, error) {
	return getAPIToken(token)
}
