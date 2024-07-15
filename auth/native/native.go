// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package native

import (
	"context"

	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db/storagev2"
	"github.com/tsuru/tsuru/errors"
	authTypes "github.com/tsuru/tsuru/types/auth"
	"github.com/tsuru/tsuru/validation"
	mongoBSON "go.mongodb.org/mongo-driver/bson"
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
	_ auth.UserScheme    = &NativeScheme{}
	_ auth.ManagedScheme = &NativeScheme{}
)

func (s NativeScheme) Login(ctx context.Context, params map[string]string) (auth.Token, error) {
	email, ok := params["email"]
	if !ok {
		return nil, ErrMissingEmailError
	}
	password, ok := params["password"]
	if !ok {
		return nil, ErrMissingPasswordError
	}
	user, err := auth.GetUserByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	token, err := createToken(ctx, user, password)
	if err != nil {
		return nil, err
	}
	return token, nil
}

func (s NativeScheme) Auth(ctx context.Context, token string) (auth.Token, error) {
	return getToken(ctx, token)
}

func (s NativeScheme) Logout(ctx context.Context, token string) error {
	return deleteToken(ctx, token)
}

func (s NativeScheme) Create(ctx context.Context, user *auth.User) (*auth.User, error) {
	if !validation.ValidateEmail(user.Email) {
		return nil, ErrInvalidEmail
	}
	if !validation.ValidateLength(user.Password, passwordMinLen, passwordMaxLen) {
		return nil, ErrInvalidPassword
	}
	if _, err := auth.GetUserByEmail(ctx, user.Email); err == nil {
		return nil, ErrEmailRegistered
	}
	if err := hashPassword(user); err != nil {
		return nil, err
	}
	if err := user.Create(ctx); err != nil {
		return nil, err
	}
	return user, nil
}

func (s NativeScheme) ChangePassword(ctx context.Context, token auth.Token, oldPassword string, newPassword string) error {
	user, err := auth.ConvertNewUser(token.User(ctx))
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
	return user.Update(ctx)
}

func (s NativeScheme) StartPasswordReset(ctx context.Context, user *auth.User) error {
	passToken, err := createPasswordToken(ctx, user)
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
	collection, err := storagev2.PasswordTokensCollection()
	if err != nil {
		return err
	}
	passToken, err := getPasswordToken(ctx, resetToken)
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

	_, err = collection.UpdateOne(ctx, mongoBSON.M{"_id": passToken.Token}, mongoBSON.M{"$set": mongoBSON.M{"used": true}})
	if err != nil {
		return err
	}
	return user.Update(ctx)
}

func (s NativeScheme) Remove(ctx context.Context, u *auth.User) error {
	err := deleteAllTokens(ctx, u.Email)
	if err != nil {
		return err
	}
	return u.Delete(ctx)
}

func (s NativeScheme) Info(ctx context.Context) (*authTypes.SchemeInfo, error) {
	return &authTypes.SchemeInfo{Name: "native"}, nil
}
