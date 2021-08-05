// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package image

import (
	"context"

	"github.com/tsuru/config"
	imageTypes "github.com/tsuru/tsuru/types/app/image"
	check "gopkg.in/check.v1"
)

type S struct{}

var _ = check.Suite(&S{})

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
	img1 := service.NewImage(context.TODO(), "", "myplatform", 1)
	c.Assert(img1, check.Equals, "tsuru/myplatform:v1")
	img2 := service.NewImage(context.TODO(), "", "myplatform", 2)
	c.Assert(img2, check.Equals, "tsuru/myplatform:v2")
	img3 := service.NewImage(context.TODO(), imageTypes.ImageRegistry("reg1.com/tsuru"), "myplatform", 3)
	c.Assert(img3, check.Equals, "reg1.com/tsuru/myplatform:v3")
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
	img1 := service.NewImage(context.TODO(), "", "myplatform", 1)
	c.Assert(img1, check.Equals, "localhost:3030/tsuru/myplatform:v1")
	img2 := service.NewImage(context.TODO(), imageTypes.ImageRegistry("reg1.com/tsuru"), "myplatform", 2)
	c.Assert(img2, check.Equals, "reg1.com/tsuru/myplatform:v2")
}

func (s *S) TestPlatformCurrentImage(c *check.C) {
	platformName := "myplatform"
	storage := &imageTypes.MockPlatformImageStorage{}
	service := &platformImageService{
		storage: storage,
	}

	storage.OnFindByName = func(n string) (*imageTypes.PlatformImage, error) {
		c.Assert(n, check.Equals, platformName)
		return &imageTypes.PlatformImage{
			Name: n,
			Versions: []imageTypes.RegistryVersion{
				{
					Version: 1,
					Images:  []string{"tsuru/" + platformName + ":v1", "reg1.com/tsuru/" + platformName + ":v1"},
				},
			},
		}, nil
	}
	img, err := service.CurrentImage(context.TODO(), "", platformName)
	c.Assert(err, check.IsNil)
	c.Assert(img, check.Equals, "tsuru/myplatform:v1")
	img, err = service.CurrentImage(context.TODO(), "reg1.com", platformName)
	c.Assert(err, check.IsNil)
	c.Assert(img, check.Equals, "reg1.com/tsuru/myplatform:v1")
	img, err = service.CurrentImage(context.TODO(), "reg-invalid.com", platformName)
	c.Assert(err, check.IsNil)
	c.Assert(img, check.Equals, "tsuru/myplatform:v1")

	storage.OnFindByName = func(n string) (*imageTypes.PlatformImage, error) {
		c.Assert(n, check.Equals, platformName)
		return &imageTypes.PlatformImage{
			Name: n,
			Versions: []imageTypes.RegistryVersion{
				{
					Version: 1,
					Images:  []string{"tsuru/" + platformName + ":v1"},
				},
				{
					Version: 2,
					Images:  []string{"tsuru/" + platformName + ":v2"},
				},
			},
		}, nil
	}
	img, err = service.CurrentImage(context.TODO(), "", platformName)
	c.Assert(err, check.IsNil)
	c.Assert(img, check.Equals, "tsuru/myplatform:v2")

	storage.OnFindByName = func(n string) (*imageTypes.PlatformImage, error) {
		return nil, imageTypes.ErrPlatformImageNotFound
	}
	img, err = service.CurrentImage(context.TODO(), "", platformName)
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
		return &imageTypes.PlatformImage{
			Name: n,
			Versions: []imageTypes.RegistryVersion{
				{
					Version: 1,
					Images:  []string{"tsuru/" + platformName + ":v1", "reg1.com/tsuru/" + platformName + ":v1"},
				},
				{
					Version: 2,
					Images:  []string{"tsuru/" + platformName + ":v2"},
				},
			},
		}, nil
	}
	images, err := service.ListImages(context.TODO(), platformName)
	c.Assert(err, check.IsNil)
	c.Assert(images, check.DeepEquals, []string{"tsuru/myplatform:v1", "reg1.com/tsuru/myplatform:v1", "tsuru/myplatform:v2"})

	storage.OnFindByName = func(n string) (*imageTypes.PlatformImage, error) {
		c.Assert(n, check.Equals, platformName)
		return nil, imageTypes.ErrPlatformImageNotFound
	}
	images, err = service.ListImages(context.TODO(), platformName)
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
		return &imageTypes.PlatformImage{
			Name: n,
			Versions: []imageTypes.RegistryVersion{
				{
					Version: 1,
					Images:  []string{"tsuru/" + platformName + ":v1", "reg1.com/tsuru/" + platformName + ":v1"},
				},
				{
					Version: 2,
					Images:  []string{"tsuru/" + platformName + ":v2"},
				},
			},
		}, nil
	}
	images, err := service.ListImagesOrDefault(context.TODO(), platformName)
	c.Assert(err, check.IsNil)
	c.Assert(images, check.DeepEquals, []string{"tsuru/myplatform:v1", "reg1.com/tsuru/myplatform:v1", "tsuru/myplatform:v2"})

	storage.OnFindByName = func(n string) (*imageTypes.PlatformImage, error) {
		c.Assert(n, check.Equals, platformName)
		return nil, imageTypes.ErrPlatformImageNotFound
	}
	images, err = service.ListImagesOrDefault(context.TODO(), platformName)
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
	err := service.DeleteImages(context.TODO(), platformName)
	c.Assert(err, check.IsNil)

	storage.OnDelete = func(n string) error {
		c.Assert(n, check.Equals, platformName)
		return imageTypes.ErrPlatformImageNotFound
	}
	err = service.DeleteImages(context.TODO(), platformName)
	c.Assert(err, check.IsNil)
}

func (s *S) TestPlatformAppendImage(c *check.C) {
	platformName := "myplatform"
	imageName := "tsuru/myplatform:v1"
	storage := &imageTypes.MockPlatformImageStorage{}
	service := &platformImageService{
		storage: storage,
	}
	storage.OnAppend = func(platform string, version int, images []string) error {
		c.Assert(platform, check.Equals, platformName)
		c.Assert(images, check.DeepEquals, []string{imageName})
		return nil
	}
	err := service.AppendImages(context.TODO(), platformName, 1, []string{imageName})
	c.Assert(err, check.IsNil)
}

func (s *S) TestPlatformFindImage(c *check.C) {
	platformName := "myplatform"
	imageName := "tsuru/myplatform:v1"
	storage := &imageTypes.MockPlatformImageStorage{}
	service := &platformImageService{
		storage: storage,
	}
	storage.OnFindByName = func(n string) (*imageTypes.PlatformImage, error) {
		c.Assert(n, check.Equals, platformName)
		return &imageTypes.PlatformImage{
			Name: n,
			Versions: []imageTypes.RegistryVersion{
				{
					Version: 1,
					Images:  []string{"tsuru/" + platformName + ":v1", "reg1.com/tsuru/" + platformName + ":v1"},
				},
				{
					Version: 2,
					Images:  []string{"tsuru/" + platformName + ":v2"},
				},
			},
		}, nil
	}
	image, err := service.FindImage(context.TODO(), "", platformName, imageName)
	c.Assert(err, check.IsNil)
	c.Assert(image, check.Equals, imageName)

	image, err = service.FindImage(context.TODO(), "", platformName, ":v1")
	c.Assert(err, check.IsNil)
	c.Assert(image, check.Equals, imageName)

	image, err = service.FindImage(context.TODO(), "", platformName, "v2")
	c.Assert(err, check.IsNil)
	c.Assert(image, check.Equals, "tsuru/"+platformName+":v2")

	storage.OnFindByName = func(n string) (*imageTypes.PlatformImage, error) {
		c.Assert(n, check.Equals, platformName)
		return &imageTypes.PlatformImage{
			Name: n,
			Versions: []imageTypes.RegistryVersion{
				{
					Version: 2,
					Images:  []string{"tsuru/" + platformName + ":v2"},
				},
			},
		}, nil
	}
	image, err = service.FindImage(context.TODO(), "", platformName, imageName)
	c.Assert(err, check.Equals, imageTypes.ErrPlatformImageNotFound)
	c.Assert(image, check.Equals, "")

	storage.OnFindByName = func(n string) (*imageTypes.PlatformImage, error) {
		c.Assert(n, check.Equals, platformName)
		return nil, imageTypes.ErrPlatformImageNotFound
	}
	image, err = service.FindImage(context.TODO(), "", platformName, imageName)
	c.Assert(err, check.NotNil)
	c.Assert(image, check.Equals, "")
}
