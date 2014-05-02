// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

// TODO: These interfaces are a Work In Progress
// Everything could change in minutes, please don't
// rely on them until this notice is gone.

type Token interface {
	GetValue() string
	GetAppName() string
	GetUserName() string
	IsAppToken() bool
	User() (*User, error)
}

type Scheme interface {
	AppLogin(appName string) (Token, error)
	Login(params map[string]string) (Token, error)
	Logout(token string) error
	Auth(token string) (Token, error)
}

// type ManagedScheme interface {
// 	Scheme
// 	Create(email string, password string) (*User, error)
// 	Remove(token Token) error
// 	ResetPassword(token Token) error
// 	ChangePassword(token Token, newPassword string) error
// }
