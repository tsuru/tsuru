// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package native

import (
	"crypto"
	"errors"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db"
	"labix.org/v2/mgo/bson"
	"time"
)

type passwordToken struct {
	Token     string `bson:"_id"`
	UserEmail string
	Creation  time.Time
	Used      bool
}

func createPasswordToken(u *auth.User) (*passwordToken, error) {
	if u == nil {
		return nil, errors.New("User is nil")
	}
	if u.Email == "" {
		return nil, errors.New("User email is empty")
	}
	t := passwordToken{
		Token:     token(u.Email, crypto.SHA256),
		UserEmail: u.Email,
		Creation:  time.Now(),
	}
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	err = conn.PasswordTokens().Insert(t)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (t *passwordToken) user() (*auth.User, error) {
	return auth.GetUserByEmail(t.UserEmail)
}

func getPasswordToken(token string) (*passwordToken, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var t passwordToken
	err = conn.PasswordTokens().Find(bson.M{"_id": token, "used": false}).One(&t)
	if err != nil {
		return nil, auth.ErrInvalidToken
	}
	if t.Creation.Add(24*time.Hour).Sub(time.Now()) < time.Minute {
		return nil, auth.ErrInvalidToken
	}
	return &t, nil
}
