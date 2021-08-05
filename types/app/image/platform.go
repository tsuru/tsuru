// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package image

import (
	"context"
	"errors"
)

type PlatformImage struct {
	Name     string
	Versions []RegistryVersion
	Count    int
}

type RegistryVersion struct {
	Version int
	Images  []string
}

type ImageRegistry string

type PlatformImageService interface {
	NewVersion(context.Context, string) (int, error)
	NewImage(context.Context, ImageRegistry, string, int) string
	CurrentImage(context.Context, ImageRegistry, string) (string, error)
	AppendImages(context.Context, string, int, []string) error
	DeleteImages(context.Context, string) error
	ListImages(context.Context, string) ([]string, error)
	ListImagesOrDefault(context.Context, string) ([]string, error)
	FindImage(context.Context, ImageRegistry, string, string) (string, error)
}

type PlatformImageStorage interface {
	Upsert(context.Context, string) (*PlatformImage, error)
	FindByName(context.Context, string) (*PlatformImage, error)
	Append(context.Context, string, int, []string) error
	Delete(context.Context, string) error
}

var (
	ErrPlatformImageNotFound = errors.New("Platform image not found")
)
