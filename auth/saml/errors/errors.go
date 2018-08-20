// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package errors

import (
	"github.com/tsuru/tsuru/errors"
)

var (
	ErrMissingRequestIdError        = &errors.ValidationError{Message: "You must provide RequestID to login"}
	ErrMissingFormValueError        = &errors.ValidationError{Message: "SAMLResponse form value missing"}
	ErrParseResponseError           = &errors.ValidationError{Message: "SAMLResponse parse error"}
	ErrEmptyIDPResponseError        = &errors.ValidationError{Message: "SAMLResponse form value missing"}
	ErrRequestWaitingForCredentials = &errors.ValidationError{Message: "Waiting credentials from IDP"}
)
