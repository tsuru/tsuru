// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package oauth

import (
	goauth2 "code.google.com/p/goauth2/oauth"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/storage"
	"github.com/tsuru/tsuru/log"
	"labix.org/v2/mgo/bson"
)

type Token struct {
	goauth2.Token
	UserEmail string `json:"email"`
}

func (t *Token) GetValue() string {
	return t.AccessToken
}

func (t *Token) User() (*auth.User, error) {
	return auth.GetUserByEmail(t.UserEmail)
}

func (t *Token) IsAppToken() bool {
	return false
}

func (t *Token) GetUserName() string {
	return t.UserEmail
}

func (t *Token) GetAppName() string {
	return ""
}

func getToken(header string) (*Token, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var t Token
	token, err := auth.ParseToken(header)
	if err != nil {
		return nil, err
	}
	coll := collection()
	defer coll.Close()
	err = coll.Find(bson.M{"token.accesstoken": token}).One(&t)
	if err != nil {
		return nil, auth.ErrInvalidToken
	}
	return &t, nil
}

func deleteToken(token string) error {
	coll := collection()
	defer coll.Close()
	return coll.Remove(bson.M{"token.accesstoken": token})
}

func (t *Token) save() error {
	coll := collection()
	defer coll.Close()
	return coll.Insert(t)
}

func collection() *storage.Collection {
	name, err := config.GetString("auth:oauth:collection")
	if err != nil {
		log.Fatal(err.Error())
	}
	conn, err := db.Conn()
	if err != nil {
		log.Errorf("Failed to connect to the database: %s", err)
	}
	return conn.Collection(name)
}
