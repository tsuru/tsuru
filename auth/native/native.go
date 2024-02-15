// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package native

import (
	"context"

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

var (
	_ auth.Scheme        = &NativeScheme{}
	_ auth.AppScheme     = &NativeScheme{}
	_ auth.ManagedScheme = &NativeScheme{}
)

func (s NativeScheme) WebLogin(_ context.Context, _ string, _ string) error {
	//	TODO
	return nil
}

func (s NativeScheme) Login(ctx context.Context, params map[string]string) (auth.Token, error) {
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

func (s NativeScheme) Auth(ctx context.Context, token string) (auth.Token, error) {
	return getToken(token)
}

func (s NativeScheme) Logout(ctx context.Context, token string) error {
	return deleteToken(token)
}

func (s NativeScheme) AppLogin(ctx context.Context, appName string) (auth.Token, error) {
	return createApplicationToken(appName)
}

func (s NativeScheme) AppLogout(ctx context.Context, token string) error {
	return s.Logout(ctx, token)
}

func (s NativeScheme) Create(ctx context.Context, user *auth.User) (*auth.User, error) {
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

func (s NativeScheme) ChangePassword(ctx context.Context, token auth.Token, oldPassword string, newPassword string) error {
	user, err := auth.ConvertNewUser(token.User())
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

func (s NativeScheme) StartPasswordReset(ctx context.Context, user *auth.User) error {
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
func (s NativeScheme) ResetPassword(ctx context.Context, user *auth.User, resetToken string) error {
	if resetToken == "" {
		return auth.ErrInvalidToken
	}
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
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

func (s NativeScheme) Remove(ctx context.Context, u *auth.User) error {
	err := deleteAllTokens(u.Email)
	if err != nil {
		return err
	}
	return u.Delete()
}

func (s NativeScheme) Info(ctx context.Context) (*auth.SchemeInfo, error) {
	return &auth.SchemeInfo{Name: "native"}, nil
}
