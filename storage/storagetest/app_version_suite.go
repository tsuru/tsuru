package storagetest

import (
	"sort"
	"time"

	appTypes "github.com/tsuru/tsuru/types/app"
	check "gopkg.in/check.v1"
)

type AppVersionSuite struct {
	SuiteHooks
	AppVersionStorage appTypes.AppVersionStorage
}

func (s *AppVersionSuite) TestAppVersionStorage_UpdateVersion(c *check.C) {
	err := s.AppVersionStorage.UpdateVersion("myapp", &appTypes.AppVersionInfo{Disabled: true})
	c.Assert(err, check.Equals, appTypes.ErrNoVersionsAvailable)

	app := &appTypes.MockApp{Name: "myapp"}
	vi1, err := s.AppVersionStorage.NewAppVersion(appTypes.NewVersionArgs{App: app})
	c.Assert(err, check.IsNil)
	vi2, err := s.AppVersionStorage.NewAppVersion(appTypes.NewVersionArgs{App: app})
	c.Assert(err, check.IsNil)
	vi1.Disabled = true
	err = s.AppVersionStorage.UpdateVersion("myapp", vi1)
	c.Assert(err, check.IsNil)
	versions, err := s.AppVersionStorage.AppVersions(app)
	c.Assert(err, check.IsNil)
	c.Assert(versions.Versions[vi1.Version].Disabled, check.Equals, true)
	c.Assert(versions.Versions[vi2.Version].Disabled, check.Equals, false)
}

func (s *AppVersionSuite) TestAppVersionStorage_UpdateVersionSuccess(c *check.C) {
	err := s.AppVersionStorage.UpdateVersionSuccess("myapp", &appTypes.AppVersionInfo{DeploySuccessful: true})
	c.Assert(err, check.Equals, appTypes.ErrNoVersionsAvailable)

	app := &appTypes.MockApp{Name: "myapp"}
	vi1, err := s.AppVersionStorage.NewAppVersion(appTypes.NewVersionArgs{App: app})
	c.Assert(err, check.IsNil)
	vi2, err := s.AppVersionStorage.NewAppVersion(appTypes.NewVersionArgs{App: app})
	c.Assert(err, check.IsNil)
	vi1.DeploySuccessful = true
	err = s.AppVersionStorage.UpdateVersionSuccess("myapp", vi1)
	c.Assert(err, check.IsNil)
	versions, err := s.AppVersionStorage.AppVersions(app)
	c.Assert(err, check.IsNil)
	c.Assert(versions.LastSuccessfulVersion, check.Equals, vi1.Version)
	c.Assert(versions.Versions[vi1.Version].DeploySuccessful, check.Equals, true)
	c.Assert(versions.Versions[vi2.Version].DeploySuccessful, check.Equals, false)
}

func (s *AppVersionSuite) TestAppVersionStorage_NewAppVersion(c *check.C) {
	vi, err := s.AppVersionStorage.NewAppVersion(appTypes.NewVersionArgs{
		App:            &appTypes.MockApp{Name: "myapp"},
		EventID:        "myevtid",
		CustomBuildTag: "mybuildtag",
		Description:    "mydesc",
	})
	c.Assert(err, check.IsNil)
	c.Assert(vi.CreatedAt.IsZero(), check.Equals, false)
	c.Assert(vi.UpdatedAt.IsZero(), check.Equals, false)
	vi.CreatedAt = time.Time{}
	vi.UpdatedAt = time.Time{}
	c.Assert(vi, check.DeepEquals, &appTypes.AppVersionInfo{
		Version:        1,
		Description:    "mydesc",
		CustomBuildTag: "mybuildtag",
		EventID:        "myevtid",
	})

	vi, err = s.AppVersionStorage.NewAppVersion(appTypes.NewVersionArgs{
		App: &appTypes.MockApp{Name: "myapp"},
	})
	c.Assert(err, check.IsNil)
	c.Assert(vi.CreatedAt.IsZero(), check.Equals, false)
	c.Assert(vi.UpdatedAt.IsZero(), check.Equals, false)
	vi.CreatedAt = time.Time{}
	vi.UpdatedAt = time.Time{}
	c.Assert(vi, check.DeepEquals, &appTypes.AppVersionInfo{
		Version: 2,
	})
}

func (s *AppVersionSuite) TestAppVersionStorage_AppVersions(c *check.C) {
	app := &appTypes.MockApp{Name: "myapp"}
	_, err := s.AppVersionStorage.AppVersions(app)
	c.Assert(err, check.Equals, appTypes.ErrNoVersionsAvailable)

	_, err = s.AppVersionStorage.NewAppVersion(appTypes.NewVersionArgs{App: app})
	c.Assert(err, check.IsNil)
	_, err = s.AppVersionStorage.NewAppVersion(appTypes.NewVersionArgs{App: app})
	c.Assert(err, check.IsNil)

	versions, err := s.AppVersionStorage.AppVersions(app)
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

func (s *AppVersionSuite) TestAppVersionStorage_DeleteVersions(c *check.C) {
	app := &appTypes.MockApp{Name: "myapp"}

	_, err := s.AppVersionStorage.AppVersions(app)
	c.Assert(err, check.Equals, appTypes.ErrNoVersionsAvailable)
	err = s.AppVersionStorage.DeleteVersions(app.Name)
	c.Assert(err, check.IsNil)

	_, err = s.AppVersionStorage.NewAppVersion(appTypes.NewVersionArgs{App: app})
	c.Assert(err, check.IsNil)
	err = s.AppVersionStorage.DeleteVersions(app.Name)
	c.Assert(err, check.IsNil)
	_, err = s.AppVersionStorage.AppVersions(app)
	c.Assert(err, check.Equals, appTypes.ErrNoVersionsAvailable)
}

func (s *AppVersionSuite) TestAppVersionStorage_AllAppVersions(c *check.C) {
	allVersions, err := s.AppVersionStorage.AllAppVersions()
	c.Assert(err, check.IsNil)
	c.Assert(allVersions, check.HasLen, 0)
	app1 := &appTypes.MockApp{Name: "myapp1"}
	app2 := &appTypes.MockApp{Name: "myapp2"}
	_, err = s.AppVersionStorage.NewAppVersion(appTypes.NewVersionArgs{App: app1})
	c.Assert(err, check.IsNil)
	_, err = s.AppVersionStorage.NewAppVersion(appTypes.NewVersionArgs{App: app2})
	c.Assert(err, check.IsNil)
	allVersions, err = s.AppVersionStorage.AllAppVersions()
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

func (s *AppVersionSuite) TestAppVersionStorage_DeleteVersion(c *check.C) {
	app := &appTypes.MockApp{Name: "myapp"}

	err := s.AppVersionStorage.DeleteVersion(app.Name, 1)
	c.Assert(err, check.Equals, appTypes.ErrNoVersionsAvailable)

	_, err = s.AppVersionStorage.NewAppVersion(appTypes.NewVersionArgs{App: app})
	c.Assert(err, check.IsNil)
	_, err = s.AppVersionStorage.NewAppVersion(appTypes.NewVersionArgs{App: app})
	c.Assert(err, check.IsNil)
	err = s.AppVersionStorage.DeleteVersion(app.Name, 9)
	c.Assert(err, check.IsNil)

	err = s.AppVersionStorage.DeleteVersion(app.Name, 1)
	c.Assert(err, check.IsNil)
	versions, err := s.AppVersionStorage.AppVersions(app)
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
