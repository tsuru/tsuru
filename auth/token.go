// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"crypto/sha512"
	"errors"
	"fmt"
	"github.com/globocom/tsuru/db"
	"labix.org/v2/mgo/bson"
	"time"
)

type Token struct {
	Token      string    `json:"token"`
	ValidUntil time.Time `json:"valid-until"`
	UserEmail  string    `json:"email"`
	AppName    string    `json:"app"`
}

func token(data string) string {
	h := sha512.New()
	h.Write([]byte(data))
	h.Write([]byte(tokenKey))
	h.Write([]byte(time.Now().Format(time.UnixDate)))
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

func getToken(token string) (*Token, error) {
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

func CheckToken(token string) (*User, error) {
	if token == "" {
		return nil, errors.New("You must provide the token")
	}
	u, err := GetUserByToken(token)
	if err != nil {
		return nil, errors.New("Invalid token")
	}
	return u, nil
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
	}
	err = conn.Tokens().Insert(t)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func CheckApplicationToken(token string) (*Token, error) {
	t, err := getToken(token)
	if err != nil || t.UserEmail != "" {
		return nil, errors.New("Invalid token.")
	}
	return t, nil
}
