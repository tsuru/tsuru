// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package image_test

import (
	"context"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app/image"
	"github.com/tsuru/tsuru/servicemanager"
	appTypes "github.com/tsuru/tsuru/types/app"
	imgTypes "github.com/tsuru/tsuru/types/app/image"
	"gopkg.in/check.v1"
)

func (s *S) TestSplitImageName(c *check.C) {
	tests := []struct {
		image        string
		expectedRepo string
		expectedTag  string
	}{
		{image: "tsuru", expectedRepo: "tsuru", expectedTag: "latest"},
		{image: "tsuru:v1", expectedRepo: "tsuru", expectedTag: "v1"},
		{image: "tsuru/platform", expectedRepo: "tsuru/platform", expectedTag: "latest"},
		{image: "tsuru/platform:v1", expectedRepo: "tsuru/platform", expectedTag: "v1"},
		{image: "registry.com/tsuru/platform:v1", expectedRepo: "registry.com/tsuru/platform", expectedTag: "v1"},
	}
	for i, t := range tests {
		repo, tag := image.SplitImageName(t.image)
		c.Check(repo, check.DeepEquals, t.expectedRepo, check.Commentf("failed test %d", i))
		c.Check(tag, check.DeepEquals, t.expectedTag, check.Commentf("failed test %d", i))
	}
}

func (s *S) TestParseImageParts(c *check.C) {
	tt := []struct {
		imageURI         string
		expectedRegistry string
		expectedImage    string
		expectedTag      string
	}{
		{"tsuru", "", "tsuru", ""},
		{"tsuru:v1", "", "tsuru", "v1"},
		{"tsuru/platform", "", "tsuru/platform", ""},
		{"tsuru/platform:v1", "", "tsuru/platform", "v1"},
		{"registry.com/tsuru/platform:v1", "registry.com", "tsuru/platform", "v1"},

		{"f064bf4", "", "f064bf4", ""},
		{"", "", "", ""},
		{"registry.io/tsuru/app-img:v1", "registry.io", "tsuru/app-img", "v1"},
		{"tsuru/app-img:v1", "", "tsuru/app-img", "v1"},
		{"tsuru/app-img", "", "tsuru/app-img", ""},
		{"f064bf4:v1", "", "f064bf4", "v1"},
		{"registry:5000/app-img:v1", "registry:5000", "app-img", "v1"},
		{"registry.io/app-img:v1", "registry.io", "app-img", "v1"},
		{"localhost/app-img:v1", "localhost", "app-img", "v1"},
		{"registry.io/a/b/c/app-img:v1", "registry.io", "a/b/c/app-img", "v1"},
	}

	for _, t := range tt {
		registry, image, tag := image.ParseImageParts(t.imageURI)
		c.Check(registry, check.Equals, t.expectedRegistry, check.Commentf("Invalid registry for image: %v", t.imageURI))
		c.Check(image, check.Equals, t.expectedImage, check.Commentf("Invalid image for image: %v", t.imageURI))
		c.Check(tag, check.Equals, t.expectedTag, check.Commentf("Invalid tag for image: %v", t.imageURI))
	}
}

func (s *S) TestGetBuildImage(c *check.C) {
	s.mockService.PlatformImage.OnFindImage = func(reg imgTypes.ImageRegistry, name, version string) (string, error) {
		return "tsuru/" + name + ":" + version, nil
	}
	s.mockService.PlatformImage.OnCurrentImage = func(reg imgTypes.ImageRegistry, name string) (string, error) {
		return "tsuru/" + name + ":v1", nil
	}
	tests := []struct {
		name              string
		registry          string
		ns                string
		app               appTypes.App
		successfulVersion bool

		expectedImage string
	}{
		{
			name:          "no deploys",
			app:           appTypes.App{Platform: "python", PlatformVersion: "latest"},
			expectedImage: "tsuru/python:v1",
		},
		{
			name:          "no deploys with platform version",
			app:           appTypes.App{Platform: "python", PlatformVersion: "v9"},
			expectedImage: "tsuru/python:v9",
		},
		{
			name:              "no deploys with version",
			successfulVersion: true,
			app:               appTypes.App{Platform: "python", PlatformVersion: "latest"},
			expectedImage:     "tsuru/python:v1",
		},
		{
			name:              "more deploys with version",
			successfulVersion: true,
			app:               appTypes.App{Platform: "python", Deploys: 1, PlatformVersion: "latest"},
			expectedImage:     "tsuru/app-myapp:v2",
		},
		{
			name:              "more deploys with version with ns",
			successfulVersion: true,
			ns:                "other-tsuru",
			app:               appTypes.App{Platform: "python", Deploys: 1, PlatformVersion: "latest"},
			expectedImage:     "other-tsuru/app-myapp:v3",
		},
		{
			name:              "multiple 10 deploys with version",
			successfulVersion: true,
			app:               appTypes.App{Platform: "python", Deploys: 20, PlatformVersion: "latest"},
			expectedImage:     "tsuru/python:v1",
		},
		{
			name:              "more deploys with registry",
			registry:          "mock.registry.com",
			successfulVersion: true,
			app:               appTypes.App{Platform: "python", Deploys: 1, PlatformVersion: "latest"},
			expectedImage:     "mock.registry.com/tsuru/app-myapp:v5",
		},
	}

	for _, tt := range tests {
		c.Logf("test %v", tt.name)
		config.Set("docker:repository-namespace", tt.ns)
		config.Set("docker:registry", tt.registry)
		tt.app.Name = "myapp"

		s.mockService.App.OnRegistry = func(app *appTypes.App) (imgTypes.ImageRegistry, error) {
			return imgTypes.ImageRegistry(tt.registry), nil
		}

		if tt.successfulVersion {
			version, err := servicemanager.AppVersion.NewAppVersion(context.TODO(), appTypes.NewVersionArgs{
				App: &appTypes.App{Name: "myapp"},
			})
			c.Assert(err, check.IsNil)
			err = version.CommitBaseImage()
			c.Assert(err, check.IsNil)
			err = version.CommitSuccessful()
			c.Assert(err, check.IsNil)
		}
		img, err := image.GetBuildImage(context.TODO(), &tt.app)
		c.Assert(err, check.IsNil)
		c.Check(img, check.Equals, tt.expectedImage)
	}
}
