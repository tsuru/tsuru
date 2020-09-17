// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package image

import (
	"context"
	"errors"
)

type PlatformImage struct {
	Name   string
	Images []string
	Count  int
}

type PlatformImageService interface {
	NewImage(context.Context, string) (string, error)
	CurrentImage(context.Context, string) (string, error)
	AppendImage(context.Context, string, string) error
	DeleteImages(context.Context, string) error
	ListImages(context.Context, string) ([]string, error)
	ListImagesOrDefault(context.Context, string) ([]string, error)
	FindImage(context.Context, string, string) (string, error)
}

type PlatformImageStorage interface {
	Upsert(context.Context, string) (*PlatformImage, error)
	FindByName(context.Context, string) (*PlatformImage, error)
	Append(context.Context, string, string) error
	Delete(context.Context, string) error
}

var (
	ErrPlatformImageNotFound = errors.New("Platform image not found")
)
