// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"fmt"
)

type appCreationError struct {
	app string
	err error
}

func (e *appCreationError) Error() string {
	return fmt.Sprintf("Tsuru failed to create the app %q: %s", e.app, e.err)
}

// ValidationError is an error implementation used whenever a ValidationError
// occurs in the app.
type ValidationError struct {
	Message string
}

func (err *ValidationError) Error() string {
	return err.Message
}

// NoTeamsError is the error returned when one tries to create an app without
// any team.
type NoTeamsError struct{}

func (err NoTeamsError) Error() string {
	return "Cannot create app without teams."
}
