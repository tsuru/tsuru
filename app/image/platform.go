// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package image

import (
	"context"
	"fmt"
	"strconv"
	"strings"

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

func (s *platformImageService) NewImage(ctx context.Context, reg imageTypes.ImageRegistry, platformName string, version int) string {
	return fmt.Sprintf("%s/%s:v%d", basicImageName(reg, "tsuru"), platformName, version)
}

func (s *platformImageService) CurrentImage(ctx context.Context, reg imageTypes.ImageRegistry, platformName string) (string, error) {
	img, err := s.storage.FindByName(ctx, platformName)
	if err != nil {
		log.Errorf("Couldn't find images for platform %q, fallback to default image name. Error: %s", platformName, err)
		return platformBasicImageName(reg, platformName), nil
	}
	if len(img.Versions) == 0 && img.Count > 0 {
		log.Errorf("Couldn't find valid images for platform %q", platformName)
		return platformBasicImageName(reg, platformName), nil
	}
	if len(img.Versions) == 0 {
		return "", imageTypes.ErrPlatformImageNotFound
	}
	latestVersion := img.Versions[len(img.Versions)-1]
	if len(latestVersion.Images) == 0 {
		return "", imageTypes.ErrPlatformImageNotFound
	}
	for _, img := range latestVersion.Images {
		if strings.HasPrefix(img, string(reg)) {
			return img, nil
		}
	}
	return latestVersion.Images[0], nil
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
	imgs, err := s.ListImages(ctx, platformName)
	if err != nil && err == imageTypes.ErrPlatformImageNotFound {
		return []string{platformBasicImageName("", platformName)}, nil
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
	if foundVersion == nil || len(foundVersion.Images) == 0 {
		return "", imageTypes.ErrPlatformImageNotFound
	}

	for _, img := range foundVersion.Images {
		if strings.HasPrefix(img, string(reg)) {
			return img, nil
		}
	}
	return foundVersion.Images[0], nil
}

func platformBasicImageName(reg imageTypes.ImageRegistry, platformName string) string {
	return fmt.Sprintf("%s/%s:latest", basicImageName(reg, "tsuru"), platformName)
}
