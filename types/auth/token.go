// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import "github.com/tsuru/tsuru/permission"

type Token interface {
	GetValue() string
	GetUserName() string
	User() (*User, error)
	Engine() string
	Permissions() ([]permission.Permission, error)
}

type NamedToken interface {
	GetTokenName() string
}
