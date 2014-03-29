// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"fmt"
)

type AppCreationError struct {
	app string
	Err error
}

func (e *AppCreationError) Error() string {
	return fmt.Sprintf("Tsuru failed to create the app %q: %s", e.app, e.Err)
}

// NoTeamsError is the error returned when one tries to create an app without
// any team.
type NoTeamsError struct{}

func (err NoTeamsError) Error() string {
	return "Cannot create app without teams."
}

// ManyTeamsError is the error returned when the user has more than one team and tries to
// create an app without specify a app team owner.
type ManyTeamsError struct{}

func (err ManyTeamsError) Error() string {
	return "You belongs to more than one team, choose one to be owner for this app."
}
