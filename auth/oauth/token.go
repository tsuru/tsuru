// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package oauth

import (
	"github.com/globalsign/mgo"
	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/storage"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/permission"
	authTypes "github.com/tsuru/tsuru/types/auth"
	"golang.org/x/oauth2"
)

var _ authTypes.Token = &tokenWrapper{}

type tokenWrapper struct {
	oauth2.Token
	UserEmail string `json:"email"`
}

func (t *tokenWrapper) GetValue() string {
	return t.AccessToken
}

func (t *tokenWrapper) User() (*authTypes.User, error) {
	return auth.ConvertOldUser(auth.GetUserByEmail(t.UserEmail))
}

func (t *tokenWrapper) GetUserName() string {
	return t.UserEmail
}

func (t *tokenWrapper) Engine() string {
	return "oauth"
}

func (t *tokenWrapper) Permissions() ([]permission.Permission, error) {
	return auth.BaseTokenPermission(t)
}

func getToken(header string) (*tokenWrapper, error) {
	var t tokenWrapper
	token, err := auth.ParseToken(header)
	if err != nil {
		return nil, err
	}
	coll := collection()
	defer coll.Close()
	err = coll.Find(bson.M{"token.accesstoken": token}).One(&t)
	if err != nil {
		if err == mgo.ErrNotFound {
			return nil, auth.ErrInvalidToken
		}
		return nil, err
	}
	return &t, nil
}

func deleteToken(token string) error {
	coll := collection()
	defer coll.Close()
	return coll.Remove(bson.M{"token.accesstoken": token})
}

func deleteAllTokens(email string) error {
	coll := collection()
	defer coll.Close()
	_, err := coll.RemoveAll(bson.M{"useremail": email})
	return err
}

func (t *tokenWrapper) save() error {
	coll := collection()
	defer coll.Close()
	return coll.Insert(t)
}

func collection() *storage.Collection {
	name, err := config.GetString("auth:oauth:collection")
	if err != nil {
		name = "oauth_tokens"
		log.Debugf("auth:oauth:collection not found using default value: %s.", name)
	}
	conn, err := db.Conn()
	if err != nil {
		log.Errorf("Failed to connect to the database: %s", err)
	}
	coll := conn.Collection(name)
	coll.EnsureIndex(mgo.Index{Key: []string{"token.accesstoken"}})
	return coll
}
