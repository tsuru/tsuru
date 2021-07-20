// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package image

import (
	"context"
	"fmt"
	"strings"

	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/storage"
	imageTypes "github.com/tsuru/tsuru/types/app/image"
)

var _ imageTypes.PlatformImageService = (*platformImageService)(nil)

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

func (s *platformImageService) NewImage(ctx context.Context, platformName string) (string, error) {
	p, err := s.storage.Upsert(ctx, platformName)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s/%s:v%d", basicImageName("tsuru"), platformName, p.Count), nil
}

func (s *platformImageService) CurrentImage(ctx context.Context, platformName string) (string, error) {
	img, err := s.storage.FindByName(ctx, platformName)
	if err != nil {
		log.Errorf("Couldn't find images for platform %q, fallback to default image name. Error: %s", platformName, err)
		return platformBasicImageName(platformName), nil
	}
	if len(img.Images) == 0 && img.Count > 0 {
		log.Errorf("Couldn't find valid images for platform %q", platformName)
		return platformBasicImageName(platformName), nil
	}
	if len(img.Images) == 0 {
		return "", imageTypes.ErrPlatformImageNotFound
	}
	return img.Images[len(img.Images)-1], nil
}

func (s *platformImageService) AppendImage(ctx context.Context, platformName, imageID string) error {
	return s.storage.Append(ctx, platformName, imageID)
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
	return img.Images, nil
}

// PlatformListImagesOrDefault returns basicImageName when platform is empty
// for backwards compatibility
func (s *platformImageService) ListImagesOrDefault(ctx context.Context, platformName string) ([]string, error) {
	imgs, err := s.ListImages(ctx, platformName)
	if err != nil && err == imageTypes.ErrPlatformImageNotFound {
		return []string{platformBasicImageName(platformName)}, nil
	}
	return imgs, err
}

func (s *platformImageService) FindImage(ctx context.Context, platformName, image string) (string, error) {
	imgs, err := s.ListImages(ctx, platformName)
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
