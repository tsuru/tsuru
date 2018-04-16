// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import "github.com/tsuru/tsuru/permission"

type Token interface {
	GetValue() string
	GetAppName() string
	GetUserName() string
	IsAppToken() bool
	User() (*User, error)
	Permissions() ([]permission.Permission, error)
}

type NamedToken interface {
	GetTokenName() string
}
