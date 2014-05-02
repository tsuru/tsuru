// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package native

import (
	"github.com/tsuru/tsuru/auth"
	tsuruErrors "github.com/tsuru/tsuru/errors"
)

var ErrMissingPasswordError error = &tsuruErrors.ValidationError{Message: "You must provide a password to login"}
var ErrMissingEmailError error = &tsuruErrors.ValidationError{Message: "You must provide a email to login"}

type NativeScheme struct{}

func (s NativeScheme) Login(params map[string]string) (auth.Tokener, error) {
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

func (s NativeScheme) Auth(token string) (auth.Tokener, error) {
	return nil, nil
}
