// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	"time"

	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/tsuru/storage/storagetest"
	appTypes "github.com/tsuru/tsuru/types/app"
	check "gopkg.in/check.v1"
)

var _ = check.Suite(&storagetest.AppVersionSuite{
	AppVersionStorage: &appVersionStorage{},
	SuiteHooks:        &mongodbBaseTest{name: "appversion"},
})

type appVersionSuite struct {
	storagetest.SuiteHooks
}

var _ = check.Suite(&appVersionSuite{
	SuiteHooks: &mongodbBaseTest{name: "appversion-internal"},
})

func (s *appVersionSuite) TestLegacyImport(c *check.C) {
	storage := &appVersionStorage{}

	coll, err := storage.legacyImagesCollection()
	c.Assert(err, check.IsNil)
	defer coll.Close()
	imgs := []string{
		"myregistry.com/tsuru/app-myapp:v6",
		"myregistry.com/tsuru/app-myapp:v8",
		"myregistry.com/tsuru/app-myapp:v9",
		"myregistry.com/tsuru/app-myapp:v10",
		"myregistry.com/tsuru/app-myapp:v11",
		"myregistry.com/tsuru/app-myapp:v12",
		"myregistry.com/tsuru/app-myapp:v13",
		"myregistry.com/tsuru/app-myapp:v15",
		"myregistry.com/tsuru/app-myapp:v16",
		"myregistry.com/tsuru/app-myapp:v17",
	}
	err = coll.Insert(bson.M{"_id": "myapp", "count": 18, "images": imgs})
	c.Assert(err, check.IsNil)

	builderColl, err := storage.legacyBuilderImagesCollection()
	c.Assert(err, check.IsNil)
	defer builderColl.Close()
	builderImgs := []string{
		"myregistry.com/tsuru/app-myapp:v6-builder",
		"myregistry.com/tsuru/app-myapp:v8-builder",
		"myregistry.com/tsuru/app-myapp:v9-builder",
		"myregistry.com/tsuru/app-myapp:v10-builder",
		"myregistry.com/tsuru/app-myapp:mycustomtag",
		"myregistry.com/tsuru/app-myapp:v11-builder",
		"myregistry.com/tsuru/app-myapp:v12-builder",
		"myregistry.com/tsuru/app-myapp:v13-builder",
		"myregistry.com/tsuru/app-myapp:v15-builder",
		"myregistry.com/tsuru/app-myapp:v16-builder",
		"myregistry.com/tsuru/app-myapp:v17-builder",
	}
	err = builderColl.Insert(bson.M{"_id": "myapp", "images": builderImgs})
	c.Assert(err, check.IsNil)

	dataColl, err := storage.legacyCustomDataCollection()
	c.Assert(err, check.IsNil)
	defer dataColl.Close()
	for i := range imgs[:5] {
		data := bson.M{
			"_id": imgs[i],
			"customdata": bson.M{
				"healthcheck": bson.M{"path": "/"},
				"hooks":       nil,
			},
			"processes_list":  bson.M{"web": []string{"python myapp.py"}},
			"exposedports":    []string{"8888/tcp"},
			"disablerollback": false,
			"reason":          "",
		}
		err = dataColl.Insert(data)
		c.Assert(err, check.IsNil)
	}

	for i := range imgs[5:] {
		data := bson.M{
			"_id":             imgs[i+5],
			"processes":       bson.M{"web": "python myapp2.py"},
			"exposedports":    []string{"8888/tcp"},
			"disablerollback": false,
			"reason":          "",
		}
		err = dataColl.Insert(data)
		c.Assert(err, check.IsNil)
	}

	versions, err := storage.AppVersions(&appTypes.MockApp{Name: "myapp"})
	c.Assert(err, check.IsNil)
	for k, v := range versions.Versions {
		c.Assert(v.CreatedAt.IsZero(), check.Equals, false)
		c.Assert(v.UpdatedAt.IsZero(), check.Equals, false)
		v.CreatedAt = time.Time{}
		v.UpdatedAt = time.Time{}
		versions.Versions[k] = v
	}
	c.Assert(versions, check.DeepEquals, appTypes.AppVersions{
		AppName:               "myapp",
		Count:                 19,
		LastSuccessfulVersion: 17,
		Versions: map[int]appTypes.AppVersionInfo{
			6: {
				Version:          6,
				BuildImage:       "myregistry.com/tsuru/app-myapp:v6-builder",
				DeployImage:      "myregistry.com/tsuru/app-myapp:v6",
				DeploySuccessful: true,
				ExposedPorts:     []string{"8888/tcp"},
				CustomData: map[string]interface{}{
					"hooks": nil,
					"healthcheck": map[string]interface{}{
						"path": "/",
					},
				},
				Processes: map[string][]string{
					"web": {"python myapp.py"},
				},
			},
			8: {
				Version:          8,
				BuildImage:       "myregistry.com/tsuru/app-myapp:v8-builder",
				DeployImage:      "myregistry.com/tsuru/app-myapp:v8",
				DeploySuccessful: true,
				ExposedPorts:     []string{"8888/tcp"},
				CustomData: map[string]interface{}{
					"hooks": nil,
					"healthcheck": map[string]interface{}{
						"path": "/",
					},
				},
				Processes: map[string][]string{
					"web": {"python myapp.py"},
				},
			},
			9: {
				Version:          9,
				BuildImage:       "myregistry.com/tsuru/app-myapp:v9-builder",
				DeployImage:      "myregistry.com/tsuru/app-myapp:v9",
				DeploySuccessful: true,
				ExposedPorts:     []string{"8888/tcp"},
				CustomData: map[string]interface{}{
					"hooks": nil,
					"healthcheck": map[string]interface{}{
						"path": "/",
					},
				},
				Processes: map[string][]string{
					"web": {"python myapp.py"},
				},
			},
			10: {
				Version:          10,
				BuildImage:       "myregistry.com/tsuru/app-myapp:v10-builder",
				DeployImage:      "myregistry.com/tsuru/app-myapp:v10",
				DeploySuccessful: true,
				ExposedPorts:     []string{"8888/tcp"},
				CustomData: map[string]interface{}{
					"hooks": nil,
					"healthcheck": map[string]interface{}{
						"path": "/",
					},
				},
				Processes: map[string][]string{
					"web": {"python myapp.py"},
				},
			},
			11: {
				Version:          11,
				BuildImage:       "myregistry.com/tsuru/app-myapp:v11-builder",
				DeployImage:      "myregistry.com/tsuru/app-myapp:v11",
				DeploySuccessful: true,
				ExposedPorts:     []string{"8888/tcp"},
				CustomData: map[string]interface{}{
					"hooks": nil,
					"healthcheck": map[string]interface{}{
						"path": "/",
					},
				},
				Processes: map[string][]string{
					"web": {"python myapp.py"},
				},
			},
			12: {
				Version:          12,
				BuildImage:       "myregistry.com/tsuru/app-myapp:v12-builder",
				DeployImage:      "myregistry.com/tsuru/app-myapp:v12",
				DeploySuccessful: true,
				ExposedPorts:     []string{"8888/tcp"},
				CustomData:       map[string]interface{}{},
				Processes: map[string][]string{
					"web": {"python myapp2.py"},
				},
			},
			13: {
				Version:          13,
				BuildImage:       "myregistry.com/tsuru/app-myapp:v13-builder",
				DeployImage:      "myregistry.com/tsuru/app-myapp:v13",
				DeploySuccessful: true,
				ExposedPorts:     []string{"8888/tcp"},
				CustomData:       map[string]interface{}{},
				Processes: map[string][]string{
					"web": {"python myapp2.py"},
				},
			},
			15: {
				Version:          15,
				BuildImage:       "myregistry.com/tsuru/app-myapp:v15-builder",
				DeployImage:      "myregistry.com/tsuru/app-myapp:v15",
				DeploySuccessful: true,
				ExposedPorts:     []string{"8888/tcp"},
				CustomData:       map[string]interface{}{},
				Processes: map[string][]string{
					"web": {"python myapp2.py"},
				},
			},
			16: {
				Version:          16,
				BuildImage:       "myregistry.com/tsuru/app-myapp:v16-builder",
				DeployImage:      "myregistry.com/tsuru/app-myapp:v16",
				DeploySuccessful: true,
				ExposedPorts:     []string{"8888/tcp"},
				CustomData:       map[string]interface{}{},
				Processes: map[string][]string{
					"web": {"python myapp2.py"},
				},
			},
			17: {
				Version:          17,
				BuildImage:       "myregistry.com/tsuru/app-myapp:v17-builder",
				DeployImage:      "myregistry.com/tsuru/app-myapp:v17",
				DeploySuccessful: true,
				ExposedPorts:     []string{"8888/tcp"},
				CustomData:       map[string]interface{}{},
				Processes: map[string][]string{
					"web": {"python myapp2.py"},
				},
			},
			18: {
				Version:          18,
				BuildImage:       "myregistry.com/tsuru/app-myapp:mycustomtag",
				DeployImage:      "",
				CustomData:       map[string]interface{}{},
				Processes:        map[string][]string{},
				ExposedPorts:     []string{},
				CustomBuildTag:   "mycustomtag",
				DeploySuccessful: false,
			},
		},
	})
}
