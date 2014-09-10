// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import "fmt"

type SchemeInfo map[string]interface{}

type Scheme interface {
	AppLogin(appName string) (Token, error)
	Login(params map[string]string) (Token, error)
	Logout(token string) error
	Auth(token string) (Token, error)
	Info() (SchemeInfo, error)
	Name() string
	Create(user *User) (*User, error)
	Remove(token Token) error
}

type ManagedScheme interface {
	Scheme
	StartPasswordReset(user *User) error
	ResetPassword(user *User, resetToken string) error
	ChangePassword(token Token, oldPassword string, newPassword string) error
}

type AuthenticationFailure struct {
	Message string
}

func (a AuthenticationFailure) Error() string {
	if a.Message != "" {
		return a.Message
	}
	return "Authentication failed, wrong password."
}

var schemes = make(map[string]Scheme)

func RegisterScheme(name string, scheme Scheme) {
	schemes[name] = scheme
}

func UnregisterScheme(name string) {
	delete(schemes, name)
}

func GetScheme(name string) (Scheme, error) {
	scheme, ok := schemes[name]
	if !ok {
		return nil, fmt.Errorf("Unknown scheme: %q.", name)
	}
	return scheme, nil
}
