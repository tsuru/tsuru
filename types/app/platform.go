// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"errors"
	"io"

	tsuruErrors "github.com/tsuru/tsuru/errors"
)

type Platform struct {
	Name     string
	Disabled bool
}

type PlatformOptions struct {
	Name   string
	Args   map[string]string
	Input  io.Reader
	Output io.Writer
}

type PlatformService interface {
	Create(PlatformOptions) error
	List(bool) ([]Platform, error)
	FindByName(string) (*Platform, error)
	Update(PlatformOptions) error
	Remove(string) error
}

type PlatformStorage interface {
	Insert(Platform) error
	FindByName(string) (*Platform, error)
	FindAll() ([]Platform, error)
	FindEnabled() ([]Platform, error)
	Update(Platform) error
	Delete(Platform) error
}

var (
	ErrPlatformNameMissing    = errors.New("Platform name is required.")
	ErrPlatformNotFound       = errors.New("Platform doesn't exist.")
	ErrDuplicatePlatform      = errors.New("Duplicate platform")
	ErrInvalidPlatform        = errors.New("Invalid platform")
	ErrDeletePlatformWithApps = errors.New("Platform has apps. You should remove them before remove the platform.")
	ErrInvalidPlatformName    = &tsuruErrors.ValidationError{
		Message: "Invalid platform name, should have at most 63 " +
			"characters, containing only lower case letters, numbers or dashes, " +
			"starting with a letter.",
	}
)
