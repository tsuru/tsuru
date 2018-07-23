// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package image

import (
	"errors"
)

type PlatformImage struct {
	Name   string
	Images []string
	Count  int
}

type PlatformImageService interface {
	NewImage(string) (string, error)
	CurrentImage(string) (string, error)
	AppendImage(string, string) error
	DeleteImages(string) error
	ListImages(string) ([]string, error)
	ListImagesOrDefault(string) ([]string, error)
}

type PlatformImageStorage interface {
	Upsert(string) (*PlatformImage, error)
	FindByName(string) (*PlatformImage, error)
	Append(string, string) error
	Delete(string) error
}

var (
	ErrPlatformImageNotFound = errors.New("Platform image not found")
)
