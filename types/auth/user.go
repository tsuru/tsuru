// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"errors"

	"github.com/tsuru/tsuru/quota"
)

type User struct {
	quota.Quota
	Email    string
	Password string
	APIKey   string
	Roles    []RoleInstance
}

type RoleInstance struct {
	Name         string
	ContextValue string
}

var (
	ErrUserNotFound = errors.New("user not found")
	ErrInvalidKey   = errors.New("invalid key")
	ErrKeyDisabled  = errors.New("key management is disabled")
)
