// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package version

import (
	"sort"
	"time"

	appTypes "github.com/tsuru/tsuru/types/app"
	check "gopkg.in/check.v1"
)

func (s *S) TestAppVersionService(c *check.C) {
	svc, err := AppVersionService()
	c.Assert(err, check.IsNil)
	_, ok := svc.(*appVersionService)
	c.Assert(ok, check.Equals, true)
}

func (s *S) TestNewAppVersion(c *check.C) {
	svc, err := AppVersionService()
	c.Assert(err, check.IsNil)

	version, err := svc.NewAppVersion(appTypes.NewVersionArgs{
		App:            &appTypes.MockApp{Name: "myapp"},
		EventID:        "myevtid",
		CustomBuildTag: "mybuildtag",
		Description:    "mydesc",
	})
	c.Assert(err, check.IsNil)
	vi := version.VersionInfo()
	c.Assert(vi.CreatedAt.IsZero(), check.Equals, false)
	c.Assert(vi.UpdatedAt.IsZero(), check.Equals, false)
	vi.CreatedAt = time.Time{}
	vi.UpdatedAt = time.Time{}
	c.Assert(vi, check.DeepEquals, appTypes.AppVersionInfo{
		Version:        1,
		Description:    "mydesc",
		CustomBuildTag: "mybuildtag",
		EventID:        "myevtid",
	})

	version, err = svc.NewAppVersion(appTypes.NewVersionArgs{
		App: &appTypes.MockApp{Name: "myapp"},
	})
	c.Assert(err, check.IsNil)
	vi = version.VersionInfo()
	c.Assert(vi.CreatedAt.IsZero(), check.Equals, false)
	c.Assert(vi.UpdatedAt.IsZero(), check.Equals, false)
	vi.CreatedAt = time.Time{}
	vi.UpdatedAt = time.Time{}
	c.Assert(vi, check.DeepEquals, appTypes.AppVersionInfo{
		Version: 2,
	})
}

func (s *S) TestAppVersionService_LatestSuccessfulVersion(c *check.C) {
	app := &appTypes.MockApp{Name: "myapp"}
	svc, err := AppVersionService()
	c.Assert(err, check.IsNil)

	_, err = svc.LatestSuccessfulVersion(app)
	c.Assert(err, check.Equals, appTypes.ErrNoVersionsAvailable)

	version, err := svc.NewAppVersion(appTypes.NewVersionArgs{App: app})
	c.Assert(err, check.IsNil)
	_, err = svc.LatestSuccessfulVersion(app)
	c.Assert(err, check.Equals, appTypes.ErrNoVersionsAvailable)

	err = version.CommitBaseImage()
	c.Assert(err, check.IsNil)
	_, err = svc.LatestSuccessfulVersion(app)
	c.Assert(err, check.Equals, appTypes.ErrNoVersionsAvailable)

	err = version.CommitSuccessful()
	c.Assert(err, check.IsNil)
	version, err = svc.LatestSuccessfulVersion(app)
	c.Assert(err, check.IsNil)
	c.Assert(version.Version(), check.Equals, 1)

	newVersion, err := svc.NewAppVersion(appTypes.NewVersionArgs{App: app})
	c.Assert(err, check.IsNil)
	version, err = svc.LatestSuccessfulVersion(app)
	c.Assert(err, check.IsNil)
	c.Assert(version.Version(), check.Equals, 1)

	err = newVersion.CommitSuccessful()
	c.Assert(err, check.IsNil)
	version, err = svc.LatestSuccessfulVersion(app)
	c.Assert(err, check.IsNil)
	c.Assert(version.Version(), check.Equals, 2)
}

func (s *S) TestAppVersionService_AppVersions(c *check.C) {
	app := &appTypes.MockApp{Name: "myapp"}
	svc, err := AppVersionService()
	c.Assert(err, check.IsNil)

	_, err = svc.AppVersions(app)
	c.Assert(err, check.Equals, appTypes.ErrNoVersionsAvailable)

	_, err = svc.NewAppVersion(appTypes.NewVersionArgs{App: app})
	c.Assert(err, check.IsNil)
	_, err = svc.NewAppVersion(appTypes.NewVersionArgs{App: app})
	c.Assert(err, check.IsNil)
	versions, err := svc.AppVersions(app)
	c.Assert(err, check.IsNil)
	for k, v := range versions.Versions {
		v.CreatedAt = time.Time{}
		v.UpdatedAt = time.Time{}
		versions.Versions[k] = v
	}
	c.Assert(versions, check.DeepEquals, appTypes.AppVersions{
		AppName:               "myapp",
		Count:                 2,
		LastSuccessfulVersion: 0,
		Versions: map[int]appTypes.AppVersionInfo{
			1: {Version: 1, CustomData: map[string]interface{}{}, Processes: map[string][]string{}, ExposedPorts: []string{}},
			2: {Version: 2, CustomData: map[string]interface{}{}, Processes: map[string][]string{}, ExposedPorts: []string{}},
		},
	})
}

func (s *S) TestAppVersionService_DeleteVersions(c *check.C) {
	app := &appTypes.MockApp{Name: "myapp"}
	svc, err := AppVersionService()
	c.Assert(err, check.IsNil)

	_, err = svc.AppVersions(app)
	c.Assert(err, check.Equals, appTypes.ErrNoVersionsAvailable)
	err = svc.DeleteVersions(app.Name)
	c.Assert(err, check.IsNil)

	_, err = svc.NewAppVersion(appTypes.NewVersionArgs{App: app})
	c.Assert(err, check.IsNil)
	err = svc.DeleteVersions(app.Name)
	c.Assert(err, check.IsNil)
	_, err = svc.AppVersions(app)
	c.Assert(err, check.Equals, appTypes.ErrNoVersionsAvailable)
}

func (s *S) TestAppVersionService_AllAppVersions(c *check.C) {
	svc, err := AppVersionService()
	c.Assert(err, check.IsNil)
	allVersions, err := svc.AllAppVersions()
	c.Assert(err, check.IsNil)
	c.Assert(allVersions, check.HasLen, 0)
	app1 := &appTypes.MockApp{Name: "myapp1"}
	app2 := &appTypes.MockApp{Name: "myapp2"}
	_, err = svc.NewAppVersion(appTypes.NewVersionArgs{App: app1})
	c.Assert(err, check.IsNil)
	_, err = svc.NewAppVersion(appTypes.NewVersionArgs{App: app2})
	c.Assert(err, check.IsNil)
	allVersions, err = svc.AllAppVersions()
	c.Assert(err, check.IsNil)
	sort.Slice(allVersions, func(i, j int) bool {
		return allVersions[i].AppName < allVersions[j].AppName
	})
	for i := range allVersions {
		for k, v := range allVersions[i].Versions {
			v.CreatedAt = time.Time{}
			v.UpdatedAt = time.Time{}
			allVersions[i].Versions[k] = v
		}
	}
	c.Assert(allVersions, check.DeepEquals, []appTypes.AppVersions{
		{
			AppName:               "myapp1",
			Count:                 1,
			LastSuccessfulVersion: 0,
			Versions: map[int]appTypes.AppVersionInfo{
				1: {Version: 1, CustomData: map[string]interface{}{}, Processes: map[string][]string{}, ExposedPorts: []string{}},
			},
		},
		{
			AppName:               "myapp2",
			Count:                 1,
			LastSuccessfulVersion: 0,
			Versions: map[int]appTypes.AppVersionInfo{
				1: {Version: 1, CustomData: map[string]interface{}{}, Processes: map[string][]string{}, ExposedPorts: []string{}},
			},
		},
	})
}

func (s *S) TestAppVersionService_DeleteVersion(c *check.C) {
	app := &appTypes.MockApp{Name: "myapp"}
	svc, err := AppVersionService()
	c.Assert(err, check.IsNil)

	err = svc.DeleteVersion(app.Name, 1)
	c.Assert(err, check.Equals, appTypes.ErrNoVersionsAvailable)

	_, err = svc.NewAppVersion(appTypes.NewVersionArgs{App: app})
	c.Assert(err, check.IsNil)
	_, err = svc.NewAppVersion(appTypes.NewVersionArgs{App: app})
	c.Assert(err, check.IsNil)
	err = svc.DeleteVersion(app.Name, 9)
	c.Assert(err, check.IsNil)

	err = svc.DeleteVersion(app.Name, 1)
	c.Assert(err, check.IsNil)
	versions, err := svc.AppVersions(app)
	c.Assert(err, check.IsNil)
	for k, v := range versions.Versions {
		v.CreatedAt = time.Time{}
		v.UpdatedAt = time.Time{}
		versions.Versions[k] = v
	}
	c.Assert(versions, check.DeepEquals, appTypes.AppVersions{
		AppName:               "myapp",
		Count:                 2,
		LastSuccessfulVersion: 0,
		Versions: map[int]appTypes.AppVersionInfo{
			2: {Version: 2, CustomData: map[string]interface{}{}, Processes: map[string][]string{}, ExposedPorts: []string{}},
		},
	})
}

func (s *S) TestAppVersionService_VersionByPendingImage(c *check.C) {
	app := &appTypes.MockApp{Name: "myapp"}
	svc, err := AppVersionService()
	c.Assert(err, check.IsNil)

	_, err = svc.VersionByPendingImage(app, "something/invalid")
	c.Assert(err, check.Equals, appTypes.ErrNoVersionsAvailable)

	_, err = svc.NewAppVersion(appTypes.NewVersionArgs{App: app})
	c.Assert(err, check.IsNil)
	_, err = svc.VersionByPendingImage(app, "something/invalid")
	c.Assert(err, check.Equals, appTypes.ErrNoVersionsAvailable)

	version, err := svc.VersionByPendingImage(app, "tsuru/app-myapp:v1")
	c.Assert(err, check.IsNil)
	c.Assert(version.Version(), check.Equals, 1)
}

func (s *S) TestAppVersionService_VersionByImageOrVersion(c *check.C) {
	app := &appTypes.MockApp{Name: "myapp"}
	svc, err := AppVersionService()
	c.Assert(err, check.IsNil)

	_, err = svc.VersionByImageOrVersion(app, "invalid")
	c.Assert(err, check.Equals, appTypes.ErrNoVersionsAvailable)

	newVersion, err := svc.NewAppVersion(appTypes.NewVersionArgs{App: app})
	c.Assert(err, check.IsNil)
	_, err = svc.VersionByImageOrVersion(app, "invalid")
	c.Assert(err, check.Equals, appTypes.ErrInvalidVersion{
		Version: "invalid",
	})

	_, err = svc.VersionByImageOrVersion(app, "tsuru/app-myapp:v1")
	c.Assert(err, check.Equals, appTypes.ErrInvalidVersion{
		Version: "tsuru/app-myapp:v1",
	})

	err = newVersion.CommitBaseImage()
	c.Assert(err, check.IsNil)
	err = newVersion.CommitSuccessful()
	c.Assert(err, check.IsNil)
	version, err := svc.VersionByImageOrVersion(app, "tsuru/app-myapp:v1")
	c.Assert(err, check.IsNil)
	c.Assert(version.Version(), check.Equals, 1)

	version, err = svc.VersionByImageOrVersion(app, "1")
	c.Assert(err, check.IsNil)
	c.Assert(version.Version(), check.Equals, 1)

	version, err = svc.VersionByImageOrVersion(app, "v1")
	c.Assert(err, check.IsNil)
	c.Assert(version.Version(), check.Equals, 1)
}
