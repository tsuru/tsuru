// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
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
func (s *platformService) Create(opts appTypes.PlatformOptions) error {
	p := appTypes.Platform{Name: opts.Name}
	if err := s.validate(p); err != nil {
		return err
	}
	err := s.storage.Insert(p)
	if err != nil {
		return err
	}
	opts.ImageName, err = servicemanager.PlatformImage.NewImage(opts.Name)
	if err != nil {
		return err
	}
	err = builder.PlatformAdd(opts)
	if err != nil {
		if imgErr := servicemanager.PlatformImage.DeleteImages(opts.Name); imgErr != nil {
			log.Errorf("unable to remove platform images: %s", imgErr)
		}
		dbErr := s.storage.Delete(p)
		if dbErr != nil {
			return tsuruErrors.NewMultiError(
				errors.Wrapf(dbErr, "unable to rollback platform add"),
				errors.Wrapf(err, "original platform add error"),
			)
		}
		return err
	}
	return servicemanager.PlatformImage.AppendImage(opts.Name, opts.ImageName)
}

// List implements List method of PlatformService interface
func (s *platformService) List(enabledOnly bool) ([]appTypes.Platform, error) {
	if enabledOnly {
		return s.storage.FindEnabled()
	}
	return s.storage.FindAll()
}

// FindByName implements FindByName method of PlatformService interface
func (s *platformService) FindByName(name string) (*appTypes.Platform, error) {
	p, err := s.storage.FindByName(name)
	if err != nil {
		return nil, appTypes.ErrInvalidPlatform
	}
	return p, nil
}

// Update implements Update method of PlatformService interface
func (s *platformService) Update(opts appTypes.PlatformOptions) error {
	if opts.Name == "" {
		return appTypes.ErrPlatformNameMissing
	}
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = s.FindByName(opts.Name)
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
		opts.ImageName, err = servicemanager.PlatformImage.NewImage(opts.Name)
		if err != nil {
			return err
		}
		err = builder.PlatformUpdate(opts)
		if err != nil {
			return err
		}
		err = servicemanager.PlatformImage.AppendImage(opts.Name, opts.ImageName)
		if err != nil {
			return err
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
		return s.storage.Update(appTypes.Platform{Name: opts.Name, Disabled: disableBool})
	}
	return nil
}

// Remove implements Remove method of PlatformService interface
func (s *platformService) Remove(name string) error {
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
	err = builder.PlatformRemove(name)
	if err != nil {
		log.Errorf("Failed to remove platform from builder: %s", err)
	}
	images, err := servicemanager.PlatformImage.ListImagesOrDefault(name)
	if err == nil {
		for _, img := range images {
			if regErr := registry.RemoveImage(img); regErr != nil {
				log.Errorf("Failed to remove platform image from registry: %s", regErr)
			}
		}
	} else {
		log.Errorf("Failed to retrieve platform images from storage: %s", err)
	}
	err = servicemanager.PlatformImage.DeleteImages(name)
	if err != nil {
		log.Errorf("Failed to remove platform images from storage: %s", err)
	}
	return s.storage.Delete(appTypes.Platform{Name: name})
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
