// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package image

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/storage"
	imageTypes "github.com/tsuru/tsuru/types/app/image"
)

var _ imageTypes.PlatformImageService = &platformImageService{}

type platformImageService struct {
	storage imageTypes.PlatformImageStorage
}

func PlatformImageService() (imageTypes.PlatformImageService, error) {
	dbDriver, err := storage.GetCurrentDbDriver()
	if err != nil {
		dbDriver, err = storage.GetDefaultDbDriver()
		if err != nil {
			return nil, err
		}
	}
	return &platformImageService{
		storage: dbDriver.PlatformImageStorage,
	}, nil
}

func (s *platformImageService) NewVersion(ctx context.Context, platformName string) (int, error) {
	p, err := s.storage.Upsert(ctx, platformName)
	if err != nil {
		return 0, err
	}
	return p.Count, nil
}

func (s *platformImageService) NewImage(ctx context.Context, reg imageTypes.ImageRegistry, platformName string, version int) (string, error) {
	imageName, err := basicImageName(reg, "tsuru")
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s/%s:v%d", imageName, platformName, version), nil
}

func (s *platformImageService) CurrentImage(ctx context.Context, reg imageTypes.ImageRegistry, platformName string) (string, error) {
	img, err := s.storage.FindByName(ctx, platformName)
	if err != nil {
		log.Errorf("Couldn't find images for platform %q, fallback to default image name. Error: %s", platformName, err)
		imageNew, err := platformBasicImageName(reg, platformName)
		if err != nil {
			return "", err
		}
		return imageNew, nil
	}
	if len(img.Versions) == 0 && img.Count > 0 {
		log.Errorf("Couldn't find valid images for platform %q", platformName)
		imageNew, err := platformBasicImageName(reg, platformName)
		if err != nil {
			return "", err
		}
		return imageNew, nil
	}
	if len(img.Versions) == 0 {
		return "", imageTypes.ErrPlatformImageNotFound
	}
	latestVersion := img.Versions[len(img.Versions)-1]
	return findImageByRegistry(reg, latestVersion)
}

func (s *platformImageService) AppendImages(ctx context.Context, platformName string, version int, imageIDs []string) error {
	return s.storage.Append(ctx, platformName, version, imageIDs)
}

func (s *platformImageService) DeleteImages(ctx context.Context, platformName string) error {
	err := s.storage.Delete(ctx, platformName)
	if err != nil && err != imageTypes.ErrPlatformImageNotFound {
		return err
	}
	return nil
}

func (s *platformImageService) ListImages(ctx context.Context, platformName string) ([]string, error) {
	img, err := s.storage.FindByName(ctx, platformName)
	if err != nil {
		return nil, err
	}
	var imgs []string
	for _, version := range img.Versions {
		imgs = append(imgs, version.Images...)
	}
	return imgs, nil
}

// PlatformListImagesOrDefault returns basicImageName when platform is empty
// for backwards compatibility
func (s *platformImageService) ListImagesOrDefault(ctx context.Context, platformName string) ([]string, error) {
	var err error
	imgs, err := s.ListImages(ctx, platformName)
	if err != nil && err == imageTypes.ErrPlatformImageNotFound {
		imageNew, err := platformBasicImageName("", platformName)
		if err != nil {
			return nil, err
		}
		return []string{imageNew}, nil
	}
	return imgs, err
}

func (s *platformImageService) FindImage(ctx context.Context, reg imageTypes.ImageRegistry, platformName, image string) (string, error) {
	imgData, err := s.storage.FindByName(ctx, platformName)
	if err != nil {
		return "", err
	}
	_, img, tag := ParseImageParts(image)
	if tag == "" {
		tag = img
	}

	wantedVersion, _ := strconv.Atoi(strings.TrimPrefix(tag, "v"))
	var foundVersion *imageTypes.RegistryVersion
	for _, version := range imgData.Versions {
		if version.Version == wantedVersion {
			foundVersion = &version
			break
		}
	}
	if foundVersion == nil {
		return "", imageTypes.ErrPlatformImageNotFound
	}

	return findImageByRegistry(reg, *foundVersion)
}

func platformBasicImageName(reg imageTypes.ImageRegistry, platformName string) (string, error) {
	imageNew, err := basicImageName(reg, "tsuru")
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s/%s:latest", imageNew, platformName), nil
}

func findImageByRegistry(reg imageTypes.ImageRegistry, imgVersion imageTypes.RegistryVersion) (string, error) {
	if len(imgVersion.Images) == 0 {
		return "", imageTypes.ErrPlatformImageNotFound
	}
	if reg != "" {
		for _, img := range imgVersion.Images {
			if strings.HasPrefix(img, string(reg)) {
				return img, nil
			}
		}
	}
	defaultReg, _ := config.GetString("docker:registry")
	for _, img := range imgVersion.Images {
		if strings.HasPrefix(img, defaultReg) {
			return img, nil
		}
	}
	return "", errors.Errorf("platform image not found for registry %q in %v", reg, imgVersion.Images)
}
