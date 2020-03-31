// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package version

import (
	"strconv"
	"strings"

	"github.com/tsuru/tsuru/storage"
	appTypes "github.com/tsuru/tsuru/types/app"
)

type appVersionService struct {
	storage appTypes.AppVersionStorage
}

func AppVersionService() (appTypes.AppVersionService, error) {
	dbDriver, err := storage.GetCurrentDbDriver()
	if err != nil {
		dbDriver, err = storage.GetDefaultDbDriver()
		if err != nil {
			return nil, err
		}
	}
	return &appVersionService{
		storage: dbDriver.AppVersionStorage,
	}, nil
}

func (s *appVersionService) NewAppVersion(args appTypes.NewVersionArgs) (appTypes.AppVersion, error) {
	versionInfo, err := s.storage.NewAppVersion(args)
	if err != nil {
		return nil, err
	}
	return &appVersionImpl{
		storage:     s.storage,
		app:         args.App,
		versionInfo: versionInfo,
	}, nil
}

func (s *appVersionService) LatestSuccessfulVersion(app appTypes.App) (appTypes.AppVersion, error) {
	versions, err := s.storage.AppVersions(app)
	if err != nil {
		return nil, err
	}
	if versions.LastSuccessfulVersion > 0 {
		version, ok := versions.Versions[versions.LastSuccessfulVersion]
		if !ok {
			return nil, appTypes.ErrNoVersionsAvailable
		}
		return &appVersionImpl{
			app:         app,
			storage:     s.storage,
			versionInfo: &version,
		}, nil
	}
	return nil, appTypes.ErrNoVersionsAvailable
}

func (s *appVersionService) VersionByPendingImage(app appTypes.App, imageID string) (appTypes.AppVersion, error) {
	versions, err := s.storage.AppVersions(app)
	if err != nil {
		return nil, err
	}
	for _, v := range versions.Versions {
		vi := &appVersionImpl{
			app:         app,
			storage:     s.storage,
			versionInfo: &v,
		}
		if vi.BaseImageName() == imageID {
			return vi, nil
		}
	}
	return nil, appTypes.ErrNoVersionsAvailable
}

func (s *appVersionService) VersionByImageOrVersion(app appTypes.App, imageOrVersion string) (appTypes.AppVersion, error) {
	versions, err := s.storage.AppVersions(app)
	if err != nil {
		return nil, err
	}
	for _, v := range versions.Versions {
		if v.DeploySuccessful &&
			v.DeployImage == imageOrVersion ||
			strconv.Itoa(v.Version) == imageOrVersion ||
			strings.HasSuffix(v.DeployImage, imageOrVersion) {
			return &appVersionImpl{
				app:         app,
				storage:     s.storage,
				versionInfo: &v,
			}, nil
		}
	}
	return nil, appTypes.ErrInvalidVersion{Version: imageOrVersion}
}

func (s *appVersionService) AppVersions(app appTypes.App) (appTypes.AppVersions, error) {
	return s.storage.AppVersions(app)
}

func (s *appVersionService) DeleteVersions(appName string) error {
	return s.storage.DeleteVersions(appName)
}

func (s *appVersionService) AllAppVersions() ([]appTypes.AppVersions, error) {
	return s.storage.AllAppVersions()
}

func (s *appVersionService) DeleteVersion(appName string, version int) error {
	return s.storage.DeleteVersion(appName, version)
}

func (s *appVersionService) AppVersionFromInfo(app appTypes.App, info appTypes.AppVersionInfo) appTypes.AppVersion {
	return &appVersionImpl{
		app:         app,
		storage:     s.storage,
		versionInfo: &info,
	}
}
