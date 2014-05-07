// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"errors"
)

type Token interface {
	GetValue() string
	GetAppName() string
	GetUserName() string
	IsAppToken() bool
	User() (*User, error)
}

var ErrInvalidToken = errors.New("Invalid token")
