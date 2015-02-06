// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package native

import (
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/validation"
)

var (
	ErrMissingPasswordError = &errors.ValidationError{Message: "you must provide a password to login"}
	ErrMissingEmailError    = &errors.ValidationError{Message: "you must provide a email to login"}
	ErrInvalidEmail         = &errors.ValidationError{Message: "invalid email"}
	ErrInvalidPassword      = &errors.ValidationError{Message: "password length should be least 6 characters and at most 50 characters"}
	ErrEmailRegistered      = &errors.ConflictError{Message: "this email is already registered"}
	ErrPasswordMismatch     = &errors.NotAuthorizedError{Message: "the given password didn't match the user's current password"}
)

type NativeScheme struct{}

func init() {
	auth.RegisterScheme("native", NativeScheme{})
}

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
	return getToken(token)
}

func (s NativeScheme) Logout(token string) error {
	return deleteToken(token)
}

func (s NativeScheme) AppLogin(appName string) (auth.Token, error) {
	return createApplicationToken(appName)
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
	if err := user.Create(); err != nil {
		return nil, err
	}
	return user, nil
}

func (s NativeScheme) ChangePassword(token auth.Token, oldPassword string, newPassword string) error {
	user, err := token.User()
	if err != nil {
		return err
	}
	if err = checkPassword(user.Password, oldPassword); err != nil {
		return ErrPasswordMismatch
	}
	if !validation.ValidateLength(newPassword, passwordMinLen, passwordMaxLen) {
		return ErrInvalidPassword
	}
	user.Password = newPassword
	hashPassword(user)
	return user.Update()
}

func (s NativeScheme) StartPasswordReset(user *auth.User) error {
	passToken, err := createPasswordToken(user)
	if err != nil {
		return err
	}
	go sendResetPassword(user, passToken)
	return nil
}

// ResetPassword actually resets the password of the user. It needs the token
// string. The new password will be a random string, that will be then sent to
// the user email.
func (s NativeScheme) ResetPassword(user *auth.User, resetToken string) error {
	if resetToken == "" {
		return auth.ErrInvalidToken
	}
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	passToken, err := getPasswordToken(resetToken)
	if err != nil {
		return err
	}
	if passToken.UserEmail != user.Email {
		return auth.ErrInvalidToken
	}
	password := generatePassword(12)
	user.Password = password
	hashPassword(user)
	go sendNewPassword(user, password)
	passToken.Used = true
	conn.PasswordTokens().UpdateId(passToken.Token, passToken)
	return user.Update()
}

func (s NativeScheme) Remove(u *auth.User) error {
	err := deleteAllTokens(u.Email)
	if err != nil {
		return err
	}
	return u.Delete()
}

func (s NativeScheme) Name() string {
	return "native"
}

func (s NativeScheme) Info() (auth.SchemeInfo, error) {
	return nil, nil
}
