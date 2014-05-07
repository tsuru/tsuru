// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package oauth

import (
	"github.com/tsuru/tsuru/auth"
	tsuruErrors "github.com/tsuru/tsuru/errors"
)

var (
	ErrMissingCodeError = &tsuruErrors.ValidationError{Message: "You must provide code to login"}
)

type OAuthScheme struct{}

func (s OAuthScheme) Login(params map[string]string) (auth.Token, error) {
	_, ok := params["code"]
	if !ok {
		return nil, ErrMissingCodeError
	}
	return nil, nil
}
