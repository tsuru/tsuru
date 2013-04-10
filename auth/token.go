// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"crypto/sha512"
	"errors"
	"fmt"
	"time"
)

type Token struct {
	Token      string
	ValidUntil time.Time
	UserEmail  string
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
	h := sha512.New()
	h.Write([]byte(u.Email))
	h.Write([]byte(tokenKey))
	h.Write([]byte(time.Now().Format(time.UnixDate)))
	t := Token{}
	t.ValidUntil = time.Now().Add(tokenExpire)
	t.Token = fmt.Sprintf("%x", h.Sum(nil))
	t.UserEmail = u.Email
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
