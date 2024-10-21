// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"context"

	"github.com/tsuru/tsuru/types/permission"
)

type Token interface {
	GetValue() string
	GetUserName() string
	User(ctx context.Context) (*User, error)
	Engine() string
	Permissions(ctx context.Context) ([]permission.Permission, error)
}

type NamedToken interface {
	GetTokenName() string
}
