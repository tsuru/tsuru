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
	Name            string
	Version         int
	ExtraTags       []string
	Args            map[string]string
	Input           io.Reader
	Output          io.Writer
	Data            []byte
	RollbackVersion int
}

type PlatformService interface {
	Create(context.Context, PlatformOptions) error
	List(context.Context, bool) ([]Platform, error)
	FindByName(context.Context, string) (*Platform, error)
	Update(context.Context, PlatformOptions) error
	Remove(context.Context, string) error
	Rollback(context.Context, PlatformOptions) error
}

type PlatformStorage interface {
	Insert(context.Context, Platform) error
	FindByName(context.Context, string) (*Platform, error)
	FindAll(context.Context) ([]Platform, error)
	FindEnabled(context.Context) ([]Platform, error)
	Update(context.Context, Platform) error
	Delete(context.Context, Platform) error
}
