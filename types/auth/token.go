// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"context"

	"github.com/tsuru/tsuru/types/permission"
)

// TsuruTokenEmailDomain is the e-mail domain used to fake users from a team
// token. This TLD is unlikely to be used world-wide, so regular Tsuru users
// should not be able to register using it.
const TsuruTokenEmailDomain = "tsuru-team-token"

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
