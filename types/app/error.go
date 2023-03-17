// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"errors"
	"fmt"

	tsuruErrors "github.com/tsuru/tsuru/errors"
)

var (
	ErrAppNotFound            = errors.New("App not found")
	ErrPlanNotFound           = errors.New("plan not found")
	ErrPlanAlreadyExists      = errors.New("plan already exists")
	ErrPlanDefaultAmbiguous   = errors.New("more than one default plan found")
	ErrPlanDefaultNotFound    = errors.New("default plan not found")
	ErrLimitOfMemory          = errors.New("The minimum allowed memory is 4MB")
	ErrPlatformNameMissing    = errors.New("Platform name is required.")
	ErrPlatformImageMissing   = errors.New("Platform image is required.")
	ErrPlatformNotFound       = errors.New("Platform doesn't exist.")
	ErrDuplicatePlatform      = errors.New("Duplicate platform")
	ErrInvalidPlatform        = errors.New("Invalid platform")
	ErrMissingFileContent     = errors.New("Missing file content.")
	ErrDeletePlatformWithApps = errors.New("Platform has apps. You must remove them before remove the platform.")
	ErrInvalidPlatformName    = &tsuruErrors.ValidationError{
		Message: "Invalid platform name, should have at most 40 " +
			"characters, containing only lower case letters, numbers or dashes, " +
			"starting with a letter.",
	}
)

type AppCreationError struct {
	App string
	Err error
}

func (e *AppCreationError) Error() string {
	return fmt.Sprintf("tsuru failed to create the app %q: %s", e.App, e.Err)
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
	return "You belong to more than one team, choose one to be owner for this app."
}

type PlanValidationError struct {
	Field string
}

func (p PlanValidationError) Error() string {
	return fmt.Sprintf("invalid value for %s", p.Field)
}
