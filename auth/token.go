// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"crypto"
	"crypto/rand"
	"errors"
	"fmt"
	"github.com/tsuru/tsuru/db"
	"labix.org/v2/mgo/bson"
	"time"
)

const keySize = 32

var ErrInvalidToken = errors.New("Invalid token")

type passwordToken struct {
	Token     string `bson:"_id"`
	UserEmail string
	Creation  time.Time
	Used      bool
}

func token(data string, hash crypto.Hash) string {
	var tokenKey [keySize]byte
	n, err := rand.Read(tokenKey[:])
	for n < keySize || err != nil {
		n, err = rand.Read(tokenKey[:])
	}
	h := hash.New()
	h.Write([]byte(data))
	h.Write(tokenKey[:])
	h.Write([]byte(time.Now().Format(time.RFC3339Nano)))
	return fmt.Sprintf("%x", h.Sum(nil))
}

func createPasswordToken(u *User) (*passwordToken, error) {
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

func (t *passwordToken) user() (*User, error) {
	return GetUserByEmail(t.UserEmail)
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
		return nil, ErrInvalidToken
	}
	if t.Creation.Add(24*time.Hour).Sub(time.Now()) < time.Minute {
		return nil, ErrInvalidToken
	}
	return &t, nil
}
