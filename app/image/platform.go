// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package image

import (
	"fmt"
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

func (s *platformImageService) NewImage(platformName string) (string, error) {
	p, err := s.storage.Upsert(platformName)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s/%s:v%d", basicImageName("tsuru"), platformName, p.Count), nil
}

func (s *platformImageService) CurrentImage(platformName string) (string, error) {
	img, err := s.storage.FindByName(platformName)
	if err != nil {
		log.Errorf("Couldn't find images for platform %q, fallback to default image name. Error: %s", platformName, err)
		return platformBasicImageName(platformName), nil
	}
	if len(img.Images) == 0 && img.Count > 0 {
		log.Errorf("Couldn't find valid images for platform %q", platformName)
		return platformBasicImageName(platformName), nil
	}
	if len(img.Images) == 0 {
		return "", ErrNoImagesAvailable
	}
	return img.Images[len(img.Images)-1], nil
}

func (s *platformImageService) AppendImage(platformName, imageID string) error {
	return s.storage.Append(platformName, imageID)
}

func (s *platformImageService) DeleteImages(platformName string) error {
	err := s.storage.Delete(platformName)
	if err != nil && err != imageTypes.ErrPlatformImageNotFound {
		return err
	}
	return nil
}

func (s *platformImageService) ListImages(platformName string) ([]string, error) {
	img, err := s.storage.FindByName(platformName)
	if err != nil {
		return nil, err
	}
	return img.Images, nil
}

// PlatformListImagesOrDefault returns basicImageName when platform is empty
// for backwards compatibility
func (s *platformImageService) ListImagesOrDefault(platformName string) ([]string, error) {
	imgs, err := s.ListImages(platformName)
	if err != nil && err == imageTypes.ErrPlatformImageNotFound {
		return []string{platformBasicImageName(platformName)}, nil
	}
	return imgs, err
}

func (s *platformImageService) FindImage(platformName, image string) (string, error) {
	imgs, err := s.ListImages(platformName)
	if err != nil {
		return "", err
	}
	for _, img := range imgs {
		if strings.HasSuffix(img, image) {
			return img, nil
		}
	}
	return "", imageTypes.ErrPlatformImageNotFound
}

func platformBasicImageName(platformName string) string {
	return fmt.Sprintf("%s/%s:latest", basicImageName("tsuru"), platformName)
}
