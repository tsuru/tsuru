// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

// TODO: These interfaces are a Work In Progress
// Everything could change in minutes, please don't
// rely on them until this notice is gone.

type Tokener interface {
	GetValue() string
	GetAppName() string
	IsAppToken() bool
	User() (*User, error)
}

type Scheme interface {
	Login(params map[string]string) (Tokener, error)
	Auth(token string) (Tokener, error)
	// CreateApplicationToken(appName string) (*Tokener, error)
}

// type ManagedScheme interface {
// 	Scheme
// 	Create(email string, password string) (*User, error)
// 	Remove(token Tokener) error
// 	ResetPassword(token Tokener) error
// 	ChangePassword(token Tokener, newPassword string) error
// }
