// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package image

import (
	"github.com/tsuru/config"
	imageTypes "github.com/tsuru/tsuru/types/app/image"
	"gopkg.in/check.v1"
)

func (s *S) TestPlatformNewImage(c *check.C) {
	platformName := "myplatform"
	var count int
	service := &platformImageService{
		storage: &imageTypes.MockPlatformImageStorage{
			OnUpsert: func(n string) (*imageTypes.PlatformImage, error) {
				c.Assert(n, check.Equals, platformName)
				count++
				return &imageTypes.PlatformImage{Name: n, Count: count}, nil
			},
		},
	}
	img1, err := service.NewImage("myplatform")
	c.Assert(err, check.IsNil)
	c.Assert(img1, check.Equals, "tsuru/myplatform:v1")
	img2, err := service.NewImage("myplatform")
	c.Assert(err, check.IsNil)
	c.Assert(img2, check.Equals, "tsuru/myplatform:v2")
	img3, err := service.NewImage("myplatform")
	c.Assert(err, check.IsNil)
	c.Assert(img3, check.Equals, "tsuru/myplatform:v3")
}

func (s *S) TestPlatformNewImageWithRegistry(c *check.C) {
	config.Set("docker:registry", "localhost:3030")
	defer config.Unset("docker:registry")
	platformName := "myplatform"
	var count int
	service := &platformImageService{
		storage: &imageTypes.MockPlatformImageStorage{
			OnUpsert: func(n string) (*imageTypes.PlatformImage, error) {
				c.Assert(n, check.Equals, platformName)
				count++
				return &imageTypes.PlatformImage{Name: n, Count: count}, nil
			},
		},
	}
	img1, err := service.NewImage("myplatform")
	c.Assert(err, check.IsNil)
	c.Assert(img1, check.Equals, "localhost:3030/tsuru/myplatform:v1")
	img2, err := service.NewImage("myplatform")
	c.Assert(err, check.IsNil)
	c.Assert(img2, check.Equals, "localhost:3030/tsuru/myplatform:v2")
	img3, err := service.NewImage("myplatform")
	c.Assert(err, check.IsNil)
	c.Assert(img3, check.Equals, "localhost:3030/tsuru/myplatform:v3")
}

func (s *S) TestPlatformCurrentImage(c *check.C) {
	platformName := "myplatform"
	storage := &imageTypes.MockPlatformImageStorage{}
	service := &platformImageService{
		storage: storage,
	}

	storage.OnFindByName = func(n string) (*imageTypes.PlatformImage, error) {
		c.Assert(n, check.Equals, platformName)
		return &imageTypes.PlatformImage{Name: n, Images: []string{"tsuru/" + platformName + ":v1"}}, nil
	}
	img, err := service.CurrentImage(platformName)
	c.Assert(err, check.IsNil)
	c.Assert(img, check.Equals, "tsuru/myplatform:v1")

	storage.OnFindByName = func(n string) (*imageTypes.PlatformImage, error) {
		return &imageTypes.PlatformImage{Name: n, Images: []string{
			"tsuru/" + platformName + ":v1",
			"tsuru/" + platformName + ":v2",
		}}, nil
	}
	img, err = service.CurrentImage(platformName)
	c.Assert(err, check.IsNil)
	c.Assert(img, check.Equals, "tsuru/myplatform:v2")

	storage.OnFindByName = func(n string) (*imageTypes.PlatformImage, error) {
		return &imageTypes.PlatformImage{Name: n, Images: []string{
			"tsuru/" + platformName + ":v1",
			"tsuru/" + platformName + ":v2",
			"tsuru/" + platformName + ":v3",
		}}, nil
	}
	img, err = service.CurrentImage(platformName)
	c.Assert(err, check.IsNil)
	c.Assert(img, check.Equals, "tsuru/myplatform:v3")

	storage.OnFindByName = func(n string) (*imageTypes.PlatformImage, error) {
		return nil, imageTypes.ErrPlatformImageNotFound
	}
	img, err = service.CurrentImage(platformName)
	c.Assert(err, check.IsNil)
	c.Assert(img, check.Equals, "tsuru/myplatform:latest")
}

func (s *S) TestPlatformListImages(c *check.C) {
	platformName := "myplatform"
	storage := &imageTypes.MockPlatformImageStorage{}
	service := &platformImageService{
		storage: storage,
	}
	storage.OnFindByName = func(n string) (*imageTypes.PlatformImage, error) {
		c.Assert(n, check.Equals, platformName)
		return &imageTypes.PlatformImage{Name: n, Images: []string{
			"tsuru/" + platformName + ":v1",
			"tsuru/" + platformName + ":v2",
		}}, nil
	}
	images, err := service.ListImages(platformName)
	c.Assert(err, check.IsNil)
	c.Assert(images, check.DeepEquals, []string{"tsuru/myplatform:v1", "tsuru/myplatform:v2"})

	storage.OnFindByName = func(n string) (*imageTypes.PlatformImage, error) {
		c.Assert(n, check.Equals, platformName)
		return nil, imageTypes.ErrPlatformImageNotFound
	}
	images, err = service.ListImages(platformName)
	c.Assert(err, check.Equals, imageTypes.ErrPlatformImageNotFound)
	c.Assert(images, check.IsNil)
}

func (s *S) TestPlatformListImagesOrDefault(c *check.C) {
	platformName := "myplatform"
	storage := &imageTypes.MockPlatformImageStorage{}
	service := &platformImageService{
		storage: storage,
	}
	storage.OnFindByName = func(n string) (*imageTypes.PlatformImage, error) {
		c.Assert(n, check.Equals, platformName)
		return &imageTypes.PlatformImage{Name: n, Images: []string{
			"tsuru/" + platformName + ":v1",
			"tsuru/" + platformName + ":v2",
		}}, nil
	}
	images, err := service.ListImagesOrDefault(platformName)
	c.Assert(err, check.IsNil)
	c.Assert(images, check.DeepEquals, []string{"tsuru/myplatform:v1", "tsuru/myplatform:v2"})

	storage.OnFindByName = func(n string) (*imageTypes.PlatformImage, error) {
		c.Assert(n, check.Equals, platformName)
		return nil, imageTypes.ErrPlatformImageNotFound
	}
	images, err = service.ListImagesOrDefault(platformName)
	c.Assert(err, check.IsNil)
	c.Assert(images, check.DeepEquals, []string{"tsuru/myplatform:latest"})
}

func (s *S) TestPlatformDeleteImages(c *check.C) {
	platformName := "myplatform"
	storage := &imageTypes.MockPlatformImageStorage{}
	service := &platformImageService{
		storage: storage,
	}
	storage.OnDelete = func(n string) error {
		c.Assert(n, check.Equals, platformName)
		return nil
	}
	err := service.DeleteImages(platformName)
	c.Assert(err, check.IsNil)

	storage.OnDelete = func(n string) error {
		c.Assert(n, check.Equals, platformName)
		return imageTypes.ErrPlatformImageNotFound
	}
	err = service.DeleteImages(platformName)
	c.Assert(err, check.IsNil)
}

func (s *S) TestPlatformAppendImage(c *check.C) {
	platformName := "myplatform"
	imageName := "tsuru/myplatform:v1"
	storage := &imageTypes.MockPlatformImageStorage{}
	service := &platformImageService{
		storage: storage,
	}
	storage.OnAppend = func(n, image string) error {
		c.Assert(n, check.Equals, platformName)
		c.Assert(image, check.Equals, imageName)
		return nil
	}
	err := service.AppendImage(platformName, imageName)
	c.Assert(err, check.IsNil)
}
