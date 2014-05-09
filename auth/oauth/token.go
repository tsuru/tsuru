// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package oauth

import (
	"code.google.com/p/goauth2/oauth"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db"
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
	return t.Extra["email"]
}

func (t *Token) GetAppName() string {
	return ""
}

func (t *Token) save() error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	return conn.Tokens().Insert(t)
}
