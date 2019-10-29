// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"context"
	"io"
)

type Platform struct {
	Name     string
	Disabled bool
}

type PlatformOptions struct {
	Name      string
	ImageName string
	ExtraTags []string
	Args      map[string]string
	Input     io.Reader
	Output    io.Writer
	Data      []byte
	Ctx       context.Context
}

type PlatformService interface {
	Create(PlatformOptions) error
	List(bool) ([]Platform, error)
	FindByName(string) (*Platform, error)
	Update(PlatformOptions) error
	Remove(string) error
	Rollback(PlatformOptions) error
}

type PlatformStorage interface {
	Insert(Platform) error
	FindByName(string) (*Platform, error)
	FindAll() ([]Platform, error)
	FindEnabled() ([]Platform, error)
	Update(Platform) error
	Delete(Platform) error
}
