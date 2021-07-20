// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package version

import (
	"context"
	"strconv"
	"strings"

	"github.com/tsuru/tsuru/storage"
	appTypes "github.com/tsuru/tsuru/types/app"
)

type appVersionService struct {
	storage appTypes.AppVersionStorage
}

var _ appTypes.AppVersionService = (*appVersionService)(nil)

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

func (s *appVersionService) NewAppVersion(ctx context.Context, args appTypes.NewVersionArgs) (appTypes.AppVersion, error) {
	versionInfo, err := s.storage.NewAppVersion(ctx, args)
	if err != nil {
		return nil, err
	}
	return &appVersionImpl{
		storage:     s.storage,
		app:         args.App,
		versionInfo: versionInfo,
	}, nil
}

func (s *appVersionService) LatestSuccessfulVersion(ctx context.Context, app appTypes.App) (appTypes.AppVersion, error) {
	versions, err := s.storage.AppVersions(ctx, app)
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

func (s *appVersionService) VersionByPendingImage(ctx context.Context, app appTypes.App, imageID string) (appTypes.AppVersion, error) {
	versions, err := s.storage.AppVersions(ctx, app)
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

func (s *appVersionService) VersionByImageOrVersion(ctx context.Context, app appTypes.App, imageOrVersion string) (appTypes.AppVersion, error) {
	versions, err := s.storage.AppVersions(ctx, app)
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

func (s *appVersionService) AppVersions(ctx context.Context, app appTypes.App) (appTypes.AppVersions, error) {
	return s.storage.AppVersions(ctx, app)
}

func (s *appVersionService) DeleteVersions(ctx context.Context, appName string, opts ...*appTypes.AppVersionWriteOptions) error {
	return s.storage.DeleteVersions(ctx, appName, opts...)
}

func (s *appVersionService) AllAppVersions(ctx context.Context, appNamesFilter ...string) ([]appTypes.AppVersions, error) {
	return s.storage.AllAppVersions(ctx, appNamesFilter...)
}

func (s *appVersionService) DeleteVersionIDs(ctx context.Context, appName string, versions []int, opts ...*appTypes.AppVersionWriteOptions) error {
	return s.storage.DeleteVersionIDs(ctx, appName, versions, opts...)
}

func (s *appVersionService) MarkToRemoval(ctx context.Context, appName string, opts ...*appTypes.AppVersionWriteOptions) error {
	return s.storage.MarkToRemoval(ctx, appName, opts...)
}

func (s *appVersionService) MarkVersionsToRemoval(ctx context.Context, appName string, versions []int, opts ...*appTypes.AppVersionWriteOptions) error {
	return s.storage.MarkVersionsToRemoval(ctx, appName, versions, opts...)
}

func (s *appVersionService) AppVersionFromInfo(ctx context.Context, app appTypes.App, info appTypes.AppVersionInfo) appTypes.AppVersion {
	return &appVersionImpl{
		ctx:         ctx,
		app:         app,
		storage:     s.storage,
		versionInfo: &info,
	}
}
