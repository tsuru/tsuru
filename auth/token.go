// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"errors"
	"strings"

	"github.com/tsuru/tsuru/permission"
)

type Token interface {
	GetValue() string
	GetAppName() string
	GetUserName() string
	IsAppToken() bool
	User() (*User, error)
	Permissions() ([]permission.Permission, error)
}

var ErrInvalidToken = errors.New("Invalid token")

// ParseToken extracts token from a header:
// 'type token' or 'token'
func ParseToken(header string) (string, error) {
	s := strings.Split(header, " ")
	var value string
	if len(s) < 3 {
		value = s[len(s)-1]
	}
	if value != "" {
		return value, nil
	}
	return value, ErrInvalidToken
}

func BaseTokenPermission(t Token) ([]permission.Permission, error) {
	if t.IsAppToken() {
		return nil, nil
	}
	user, err := t.User()
	if err != nil {
		return nil, err
	}
	return user.Permissions()
}
