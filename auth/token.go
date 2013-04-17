// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"crypto/rand"
	"crypto/sha1"
	"errors"
	"fmt"
	"github.com/globocom/tsuru/db"
	"labix.org/v2/mgo/bson"
	"time"
)

const keySize = 32 // size of the key, in bytes

type Token struct {
	Token      string    `json:"token"`
	ValidUntil time.Time `json:"valid-until"`
	UserEmail  string    `json:"email"`
	AppName    string    `json:"app"`
}

func (t *Token) User() (*User, error) {
	return GetUserByEmail(t.UserEmail)
}

func token(data string) string {
	var tokenKey [keySize]byte
	n, err := rand.Read(tokenKey[:])
	for n < keySize || err != nil {
		n, err = rand.Read(tokenKey[:])
	}
	h := sha1.New()
	h.Write([]byte(data))
	h.Write(tokenKey[:])
	h.Write([]byte(time.Now().Format(time.RFC3339Nano)))
	return fmt.Sprintf("%x", h.Sum(nil))
}

func newUserToken(u *User) (*Token, error) {
	if u == nil {
		return nil, errors.New("User is nil")
	}
	if u.Email == "" {
		return nil, errors.New("Impossible to generate tokens for users without email")
	}
	if err := loadConfig(); err != nil {
		return nil, err
	}
	t := Token{}
	t.ValidUntil = time.Now().Add(tokenExpire)
	t.Token = token(u.Email)
	t.UserEmail = u.Email
	return &t, nil
}

func GetToken(token string) (*Token, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var t Token
	err = conn.Tokens().Find(bson.M{"token": token}).One(&t)
	if err != nil {
		return nil, errors.New("Token not found")
	}
	if t.ValidUntil.Sub(time.Now()) < 1 {
		return nil, errors.New("Token has expired")
	}
	return &t, nil
}

func DeleteToken(token string) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	return conn.Tokens().Remove(bson.M{"token": token})
}

func CreateApplicationToken(appName string) (*Token, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	t := Token{
		ValidUntil: time.Now().Add(365 * 24 * time.Hour),
		Token:      token(appName),
		AppName:    appName,
	}
	err = conn.Tokens().Insert(t)
	if err != nil {
		return nil, err
	}
	return &t, nil
}
