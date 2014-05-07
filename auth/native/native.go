// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package native

import (
	"github.com/tsuru/tsuru/auth"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/validation"
)

var (
	ErrMissingPasswordError = &tsuruErrors.ValidationError{Message: "You must provide a password to login"}
	ErrMissingEmailError    = &tsuruErrors.ValidationError{Message: "You must provide a email to login"}
	ErrInvalidEmail         = &tsuruErrors.ValidationError{Message: "Invalid email."}
	ErrInvalidPassword      = &tsuruErrors.ValidationError{Message: "Password length should be least 6 characters and at most 50 characters."}
	ErrEmailRegistered      = &tsuruErrors.ConflictError{Message: "This email is already registered."}
)

type NativeScheme struct{}

func (s NativeScheme) Login(params map[string]string) (auth.Token, error) {
	email, ok := params["email"]
	if !ok {
		return nil, ErrMissingEmailError
	}
	password, ok := params["password"]
	if !ok {
		return nil, ErrMissingPasswordError
	}
	user, err := auth.GetUserByEmail(email)
	if err != nil {
		return nil, err
	}
	token, err := createToken(user, password)
	if err != nil {
		return nil, err
	}
	return token, nil
}

func (s NativeScheme) Auth(token string) (auth.Token, error) {
	return GetToken(token)
}

func (s NativeScheme) Logout(token string) error {
	return DeleteToken(token)
}

func (s NativeScheme) AppLogin(appName string) (auth.Token, error) {
	return CreateApplicationToken(appName)
}

func (s NativeScheme) Create(user *auth.User) (*auth.User, error) {
	if !validation.ValidateEmail(user.Email) {
		return nil, ErrInvalidEmail
	}
	if !validation.ValidateLength(user.Password, passwordMinLen, passwordMaxLen) {
		return nil, ErrInvalidPassword
	}
	if _, err := auth.GetUserByEmail(user.Email); err == nil {
		return nil, ErrEmailRegistered
	}

	if err := hashPassword(user); err != nil {
		return nil, err
	}
	user.Create()
	return user, nil
}

func (s NativeScheme) ChangePassword(token auth.Token, oldPassword string, newPassword string) error {
	return nil
}

func (s NativeScheme) Remove(token auth.Token) error {
	return nil
}

func (s NativeScheme) ResetPassword(token auth.Token) error {
	return nil
}
