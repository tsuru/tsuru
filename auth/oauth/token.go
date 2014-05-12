// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package oauth

import (
	"code.google.com/p/goauth2/oauth"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/storage"
	"github.com/tsuru/tsuru/log"
)

type Token struct {
	oauth.Token
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
