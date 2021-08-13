// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"context"
	"io/ioutil"
	"strconv"

	"github.com/globalsign/mgo/bson"
	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/builder"
	"github.com/tsuru/tsuru/db"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/registry"
	"github.com/tsuru/tsuru/servicemanager"
	"github.com/tsuru/tsuru/storage"
	appTypes "github.com/tsuru/tsuru/types/app"
	"github.com/tsuru/tsuru/validation"
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
	multiErr := tsuruErrors.NewMultiError()
	if len(imgs) > 0 {
		appendErr := servicemanager.PlatformImage.AppendImages(ctx, opts.Name, opts.Version, imgs)
		if appendErr != nil {
			multiErr.Add(appendErr)
		}
	}
	if err != nil {
		multiErr.Add(err)
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
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = s.FindByName(ctx, opts.Name)
	if err != nil {
		return err
	}
	if opts.Input != nil {
		data, err := ioutil.ReadAll(opts.Input)
		if err != nil {
			return err
		}
		if len(data) == 0 {
			return appTypes.ErrMissingFileContent
		}
		opts.Data = data
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
		var apps []App
		err = conn.Apps().Find(bson.M{"framework": opts.Name}).All(&apps)
		if err != nil {
			return err
		}
		for _, app := range apps {
			app.SetUpdatePlatform(true)
		}
	}
	if opts.Args["disabled"] != "" {
		disableBool, err := strconv.ParseBool(opts.Args["disabled"])
		if err != nil {
			return err
		}
		return s.storage.Update(ctx, appTypes.Platform{Name: opts.Name, Disabled: disableBool})
	}
	return nil
}

// Remove implements Remove method of PlatformService interface
func (s *platformService) Remove(ctx context.Context, name string) error {
	if name == "" {
		return appTypes.ErrPlatformNameMissing
	}
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	apps, _ := conn.Apps().Find(bson.M{"framework": name}).Count()
	if apps > 0 {
		return appTypes.ErrDeletePlatformWithApps
	}
	err = builder.PlatformRemove(ctx, name)
	if err != nil {
		log.Errorf("Failed to remove platform from builder: %s", err)
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
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	var apps []App
	err = conn.Apps().Find(bson.M{"framework": opts.Name}).All(&apps)
	if err != nil {
		return err
	}
	for _, app := range apps {
		app.SetUpdatePlatform(true)
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
