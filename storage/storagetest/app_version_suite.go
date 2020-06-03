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
	versions, err := s.AppVersionStorage.AppVersions(app)
	c.Assert(err, check.IsNil)
	err = s.AppVersionStorage.UpdateVersion("myapp", vi1)
	c.Assert(err, check.IsNil)
	updatedVersions, err := s.AppVersionStorage.AppVersions(app)
	c.Assert(err, check.IsNil)
	c.Assert(updatedVersions.UpdatedHash, check.Not(check.Equals), versions.UpdatedHash)
	c.Assert(updatedVersions.Versions[vi1.Version].Disabled, check.Equals, true)
	c.Assert(updatedVersions.Versions[vi2.Version].Disabled, check.Equals, false)
	c.Assert(updatedVersions.UpdatedAt.Unix(), check.Equals, vi2.UpdatedAt.Unix())
}

func (s *AppVersionSuite) TestAppVersionStorage_UpdateVersionSuccess(c *check.C) {
	err := s.AppVersionStorage.UpdateVersionSuccess("myapp", &appTypes.AppVersionInfo{DeploySuccessful: true})
	c.Assert(err, check.Equals, appTypes.ErrNoVersionsAvailable)

	app := &appTypes.MockApp{Name: "myapp"}
	vi1, err := s.AppVersionStorage.NewAppVersion(appTypes.NewVersionArgs{App: app})
	c.Assert(err, check.IsNil)
	vi2, err := s.AppVersionStorage.NewAppVersion(appTypes.NewVersionArgs{App: app})
	c.Assert(err, check.IsNil)
	versions, err := s.AppVersionStorage.AppVersions(app)
	c.Assert(err, check.IsNil)
	vi1.DeploySuccessful = true
	err = s.AppVersionStorage.UpdateVersionSuccess("myapp", vi1)
	c.Assert(err, check.IsNil)
	updatedVersions, err := s.AppVersionStorage.AppVersions(app)
	c.Assert(err, check.IsNil)
	c.Assert(updatedVersions.UpdatedHash, check.Not(check.Equals), versions.UpdatedHash)
	c.Assert(updatedVersions.LastSuccessfulVersion, check.Equals, vi1.Version)
	c.Assert(updatedVersions.Versions[vi1.Version].DeploySuccessful, check.Equals, true)
	c.Assert(updatedVersions.Versions[vi2.Version].DeploySuccessful, check.Equals, false)
	c.Assert(updatedVersions.UpdatedAt.Unix(), check.Equals, vi2.UpdatedAt.Unix())
}

func (s *AppVersionSuite) TestAppVersionStorage_NewAppVersion(c *check.C) {
	app := &appTypes.MockApp{Name: "myapp"}
	vi, err := s.AppVersionStorage.NewAppVersion(appTypes.NewVersionArgs{
		App:            app,
		EventID:        "myevtid",
		CustomBuildTag: "mybuildtag",
		Description:    "mydesc",
	})
	c.Assert(err, check.IsNil)
	c.Assert(vi.CreatedAt.IsZero(), check.Equals, false)
	c.Assert(vi.UpdatedAt.IsZero(), check.Equals, false)
	versions, err := s.AppVersionStorage.AppVersions(app)
	c.Assert(err, check.IsNil)
	c.Assert(versions.UpdatedAt.Unix(), check.Equals, vi.UpdatedAt.Unix())
	c.Assert(versions.UpdatedHash, check.Not(check.Equals), "")
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
	c.Assert(versions.AppName, check.Equals, "myapp")
	c.Assert(versions.Count, check.Equals, 2)
	c.Assert(versions.Versions, check.DeepEquals, map[int]appTypes.AppVersionInfo{
		1: {Version: 1, CustomData: map[string]interface{}{}, Processes: map[string][]string{}, ExposedPorts: []string{}},
		2: {Version: 2, CustomData: map[string]interface{}{}, Processes: map[string][]string{}, ExposedPorts: []string{}},
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
	c.Assert(allVersions, check.HasLen, 2)
	c.Assert(allVersions[0].AppName, check.Equals, "myapp1")
	c.Assert(allVersions[0].Count, check.Equals, 1)
	c.Assert(allVersions[0].UpdatedAt.Unix() <= time.Now().UTC().Unix(), check.Equals, true)
	c.Assert(allVersions[0].Versions, check.DeepEquals, map[int]appTypes.AppVersionInfo{
		1: {Version: 1, CustomData: map[string]interface{}{}, Processes: map[string][]string{}, ExposedPorts: []string{}},
	})

	c.Assert(allVersions[1].AppName, check.Equals, "myapp2")
	c.Assert(allVersions[1].Count, check.Equals, 1)
	c.Assert(allVersions[1].UpdatedAt.Unix() <= time.Now().UTC().Unix(), check.Equals, true)
	c.Assert(allVersions[1].Versions, check.DeepEquals, map[int]appTypes.AppVersionInfo{
		1: {Version: 1, CustomData: map[string]interface{}{}, Processes: map[string][]string{}, ExposedPorts: []string{}},
	})
}

func (s *AppVersionSuite) TestAppVersionStorage_DeleteVersionIDs(c *check.C) {
	app := &appTypes.MockApp{Name: "myapp"}

	err := s.AppVersionStorage.DeleteVersionIDs(app.Name, []int{1})
	c.Assert(err, check.Equals, appTypes.ErrNoVersionsAvailable)

	_, err = s.AppVersionStorage.NewAppVersion(appTypes.NewVersionArgs{App: app})
	c.Assert(err, check.IsNil)
	_, err = s.AppVersionStorage.NewAppVersion(appTypes.NewVersionArgs{App: app})
	c.Assert(err, check.IsNil)
	oldVersions, err := s.AppVersionStorage.AppVersions(app)
	c.Assert(err, check.IsNil)
	err = s.AppVersionStorage.DeleteVersionIDs(app.Name, []int{9})
	c.Assert(err, check.IsNil)

	err = s.AppVersionStorage.DeleteVersionIDs(app.Name, []int{1})
	c.Assert(err, check.IsNil)
	versions, err := s.AppVersionStorage.AppVersions(app)
	c.Assert(err, check.IsNil)
	for k, v := range versions.Versions {
		v.CreatedAt = time.Time{}
		v.UpdatedAt = time.Time{}
		versions.Versions[k] = v
	}
	c.Assert(versions.AppName, check.DeepEquals, "myapp")
	c.Assert(versions.UpdatedHash, check.Not(check.Equals), oldVersions.UpdatedHash)
	c.Assert(versions.Count, check.DeepEquals, 2)
	c.Assert(versions.LastSuccessfulVersion, check.DeepEquals, 0)
	c.Assert(versions.UpdatedAt.IsZero(), check.Equals, false)
	c.Assert(versions.Versions, check.DeepEquals, map[int]appTypes.AppVersionInfo{
		2: {Version: 2, CustomData: map[string]interface{}{}, Processes: map[string][]string{}, ExposedPorts: []string{}},
	})
}

func (s *AppVersionSuite) TestAppVersionStorage_ConcurrencyDeletes(c *check.C) {
	app := &appTypes.MockApp{Name: "myapp-concurrent"}

	_, err := s.AppVersionStorage.NewAppVersion(appTypes.NewVersionArgs{App: app})
	c.Assert(err, check.IsNil)
	_, err = s.AppVersionStorage.NewAppVersion(appTypes.NewVersionArgs{App: app})
	c.Assert(err, check.IsNil)
	oldVersions, err := s.AppVersionStorage.AppVersions(app)
	c.Assert(err, check.IsNil)

	err = s.AppVersionStorage.DeleteVersionIDs(app.Name, []int{2}, &appTypes.AppVersionWriteOptions{
		PreviousUpdatedHash: oldVersions.UpdatedHash,
	})
	c.Assert(err, check.IsNil)

	err = s.AppVersionStorage.DeleteVersionIDs(app.Name, []int{1}, &appTypes.AppVersionWriteOptions{
		PreviousUpdatedHash: oldVersions.UpdatedHash,
	})
	c.Check(err, check.Equals, appTypes.ErrTransactionCancelledByChange)
	versions, err := s.AppVersionStorage.AppVersions(app)
	c.Assert(err, check.IsNil)
	c.Check(versions.AppName, check.DeepEquals, "myapp-concurrent")
	c.Check(versions.Count, check.DeepEquals, 2)
	c.Check(versions.LastSuccessfulVersion, check.DeepEquals, 0)
	c.Check(versions.UpdatedAt.IsZero(), check.Equals, false)

	versionIDs := []int{}
	for versionID := range versions.Versions {
		versionIDs = append(versionIDs, versionID)
	}

	c.Check(versionIDs, check.DeepEquals, []int{1})
}

func (s *AppVersionSuite) TestAppVersionStorage_ConcurrencyMarkToRemoval(c *check.C) {
	app := &appTypes.MockApp{Name: "myapp-concurrent"}

	_, err := s.AppVersionStorage.NewAppVersion(appTypes.NewVersionArgs{App: app})
	c.Assert(err, check.IsNil)
	_, err = s.AppVersionStorage.NewAppVersion(appTypes.NewVersionArgs{App: app})
	c.Assert(err, check.IsNil)
	oldVersions, err := s.AppVersionStorage.AppVersions(app)
	c.Assert(err, check.IsNil)

	err = s.AppVersionStorage.MarkVersionsToRemoval(app.Name, []int{2}, &appTypes.AppVersionWriteOptions{
		PreviousUpdatedHash: oldVersions.UpdatedHash,
	})
	c.Assert(err, check.IsNil)

	err = s.AppVersionStorage.MarkVersionsToRemoval(app.Name, []int{1}, &appTypes.AppVersionWriteOptions{
		PreviousUpdatedHash: oldVersions.UpdatedHash,
	})
	c.Check(err, check.Equals, appTypes.ErrTransactionCancelledByChange)
	versions, err := s.AppVersionStorage.AppVersions(app)
	c.Assert(err, check.IsNil)
	c.Check(versions.AppName, check.DeepEquals, "myapp-concurrent")
	c.Check(versions.Count, check.DeepEquals, 2)
	c.Check(versions.LastSuccessfulVersion, check.DeepEquals, 0)
	c.Check(versions.UpdatedAt.IsZero(), check.Equals, false)

	versionIDs := []int{}
	for versionID, v := range versions.Versions {
		if !v.MarkedToRemoval {
			versionIDs = append(versionIDs, versionID)
		}
	}

	c.Check(versionIDs, check.DeepEquals, []int{1})
}

// TODO: UpdateVersion, UpdateVersionSuccess, DeleteVersions
