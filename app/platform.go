// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"context"
	"strconv"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/builder"
	"github.com/tsuru/tsuru/db/storagev2"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/registry"
	"github.com/tsuru/tsuru/servicemanager"
	"github.com/tsuru/tsuru/storage"
	appTypes "github.com/tsuru/tsuru/types/app"
	"github.com/tsuru/tsuru/validation"
	mongoBSON "go.mongodb.org/mongo-driver/bson"
)

var _ appTypes.PlatformService = &platformService{}

type platformService struct {
	storage appTypes.PlatformStorage
}

func PlatformService() (appTypes.PlatformService, error) {
	dbDriver, err := storage.GetCurrentDbDriver()
	if err != nil {
		dbDriver, err = storage.GetDefaultDbDriver()
		if err != nil {
			return nil, err
		}
	}
	return &platformService{
		storage: dbDriver.PlatformStorage,
	}, nil
}

// Create implements Create method of PlatformService interface
func (s *platformService) Create(ctx context.Context, opts appTypes.PlatformOptions) error {
	p := appTypes.Platform{Name: opts.Name}
	if err := s.validate(p); err != nil {
		return err
	}

	err := s.storage.Insert(ctx, p)
	if err != nil {
		return err
	}

	opts.Version, err = servicemanager.PlatformImage.NewVersion(ctx, opts.Name)
	if err != nil {
		return err
	}

	imgs, err := builder.PlatformBuild(ctx, opts)
	if err != nil {
		return err
	}

	multiErr := tsuruErrors.NewMultiError()
	if len(imgs) > 0 {
		appendErr := servicemanager.PlatformImage.AppendImages(ctx, opts.Name, opts.Version, imgs)
		if appendErr != nil {
			multiErr.Add(appendErr)
		}
	}

	// TODO: rewrite the below code using actions pipeline.
	if multiErr.Len() > 0 {
		if imgErr := servicemanager.PlatformImage.DeleteImages(ctx, opts.Name); imgErr != nil {
			multiErr.Add(imgErr)
			log.Errorf("unable to remove platform images: %s", imgErr)
		}

		dbErr := s.storage.Delete(ctx, p)
		if dbErr != nil {
			multiErr.Add(dbErr)
			return tsuruErrors.NewMultiError(
				errors.Wrapf(dbErr, "unable to rollback platform add"),
				errors.Wrapf(err, "original platform add error"),
			)
		}
	}

	return multiErr.ToError()
}

// List implements List method of PlatformService interface
func (s *platformService) List(ctx context.Context, enabledOnly bool) ([]appTypes.Platform, error) {
	if enabledOnly {
		return s.storage.FindEnabled(ctx)
	}
	return s.storage.FindAll(ctx)
}

// FindByName implements FindByName method of PlatformService interface
func (s *platformService) FindByName(ctx context.Context, name string) (*appTypes.Platform, error) {
	p, err := s.storage.FindByName(ctx, name)
	if err != nil {
		return nil, appTypes.ErrInvalidPlatform
	}
	return p, nil
}

// Update implements Update method of PlatformService interface
func (s *platformService) Update(ctx context.Context, opts appTypes.PlatformOptions) error {
	if opts.Name == "" {
		return appTypes.ErrPlatformNameMissing
	}

	_, err := s.FindByName(ctx, opts.Name)
	if err != nil {
		return err
	}

	if disabled := opts.Args["disabled"]; disabled == "" && len(opts.Data) == 0 {
		return errors.New("either disabled or dockerfile must be provided")
	}

	if len(opts.Data) > 0 {
		opts.Version, err = servicemanager.PlatformImage.NewVersion(ctx, opts.Name)
		if err != nil {
			return err
		}

		imgs, err := builder.PlatformBuild(ctx, opts)
		if err != nil {
			return err
		}

		multiErr := tsuruErrors.NewMultiError()
		if len(imgs) > 0 {
			err = servicemanager.PlatformImage.AppendImages(ctx, opts.Name, opts.Version, imgs)
			if err != nil {
				multiErr.Add(err)
			}
		}

		if multiErr.Len() > 0 {
			return multiErr.ToError()
		}

		var apps []*appTypes.App

		appsCollection, err := storagev2.AppsCollection()
		if err != nil {
			return err
		}

		cursor, err := appsCollection.Find(ctx, mongoBSON.M{"framework": opts.Name})
		if err != nil {
			return err
		}
		err = cursor.All(ctx, &apps)
		if err != nil {
			return err
		}

		for _, app := range apps {
			SetUpdatePlatform(ctx, app, true)
		}
	}

	if disabledStr := opts.Args["disabled"]; disabledStr != "" {
		disabled, _ := strconv.ParseBool(disabledStr)
		return s.storage.Update(ctx, appTypes.Platform{Name: opts.Name, Disabled: disabled})
	}

	return nil
}

// Remove implements Remove method of PlatformService interface
func (s *platformService) Remove(ctx context.Context, name string) error {
	if name == "" {
		return appTypes.ErrPlatformNameMissing
	}
	appsCollection, err := storagev2.AppsCollection()
	if err != nil {
		return err
	}
	apps, err := appsCollection.CountDocuments(ctx, mongoBSON.M{"framework": name})
	if err != nil {
		return err
	}
	if apps > 0 {
		return appTypes.ErrDeletePlatformWithApps
	}
	images, err := servicemanager.PlatformImage.ListImagesOrDefault(ctx, name)
	if err == nil {
		for _, img := range images {
			if regErr := registry.RemoveImage(ctx, img); regErr != nil {
				log.Errorf("Failed to remove platform image from registry: %s", regErr)
			}
		}
	} else {
		log.Errorf("Failed to retrieve platform images from storage: %s", err)
	}
	err = servicemanager.PlatformImage.DeleteImages(ctx, name)
	if err != nil {
		log.Errorf("Failed to remove platform images from storage: %s", err)
	}
	return s.storage.Delete(ctx, appTypes.Platform{Name: name})
}

// Rollback implements Rollback method of PlatformService interface
func (s *platformService) Rollback(ctx context.Context, opts appTypes.PlatformOptions) error {
	if opts.Name == "" {
		return appTypes.ErrPlatformNameMissing
	}
	if opts.RollbackVersion == 0 {
		return appTypes.ErrPlatformImageMissing
	}
	_, err := s.FindByName(ctx, opts.Name)
	if err != nil {
		return err
	}
	opts.Version, err = servicemanager.PlatformImage.NewVersion(ctx, opts.Name)
	if err != nil {
		return err
	}
	imgs, err := builder.PlatformBuild(ctx, opts)
	multiErr := tsuruErrors.NewMultiError()
	if len(imgs) > 0 {
		appendErr := servicemanager.PlatformImage.AppendImages(ctx, opts.Name, opts.Version, imgs)
		if appendErr != nil {
			multiErr.Add(appendErr)
		}
	}
	if err != nil {
		multiErr.Add(err)
	}
	if multiErr.Len() > 0 {
		return multiErr.ToError()
	}
	appsCollection, err := storagev2.AppsCollection()
	if err != nil {
		return err
	}
	var apps []*appTypes.App
	cursor, err := appsCollection.Find(ctx, mongoBSON.M{"framework": opts.Name})
	if err != nil {
		return err
	}

	err = cursor.All(ctx, &apps)
	if err != nil {
		return err
	}
	for _, app := range apps {
		SetUpdatePlatform(ctx, app, true)
	}
	return nil
}

func (s *platformService) validate(p appTypes.Platform) error {
	if p.Name == "" {
		return appTypes.ErrPlatformNameMissing
	}
	if !validation.ValidateName(p.Name) {
		return appTypes.ErrInvalidPlatformName
	}
	return nil
}
