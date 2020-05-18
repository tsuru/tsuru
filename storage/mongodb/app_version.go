// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/globalsign/mgo"
	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app/image"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/storage"
	appTypes "github.com/tsuru/tsuru/types/app"
)

const (
	defaultLegacyCollectionPrefix = "docker"

	collectionName = "app_versions"
)

type legacyImageMetadata struct {
	Name            string `bson:"_id"`
	CustomData      map[string]interface{}
	LegacyProcesses map[string]string   `bson:"processes"`
	Processes       map[string][]string `bson:"processes_list"`
	ExposedPorts    []string
	DisableRollback bool
	Reason          string
}

type appVersionStorage struct{}

func (s *appVersionStorage) collection() (*storage.Collection, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	coll := conn.Collection(collectionName)
	err = coll.EnsureIndex(mgo.Index{
		Key:    []string{"appname"},
		Unique: true,
	})
	return coll, err
}

func (s *appVersionStorage) legacyBuilderImagesCollection() (*storage.Collection, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	return conn.Collection("builder_app_image"), nil
}

func (s *appVersionStorage) legacyImagesCollection() (*storage.Collection, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	name, err := config.GetString("docker:collection")
	if err != nil {
		name = defaultLegacyCollectionPrefix
	}
	return conn.Collection(fmt.Sprintf("%s_app_image", name)), nil
}

func (s *appVersionStorage) legacyCustomDataCollection() (*storage.Collection, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	name, err := config.GetString("docker:collection")
	if err != nil {
		name = defaultLegacyCollectionPrefix
	}
	return conn.Collection(fmt.Sprintf("%s_image_custom_data", name)), nil
}

type appImages struct {
	AppName string `bson:"_id"`
	Images  []string
	Count   int
}

func (s *appVersionStorage) legacyImagesData(appName string) (appImages, error) {
	coll, err := s.legacyImagesCollection()
	if err != nil {
		return appImages{}, err
	}
	defer coll.Close()
	var imageData appImages
	err = coll.Find(bson.M{"_id": appName}).One(&imageData)
	return imageData, err
}

func (s *appVersionStorage) legacyBuildImages(appName string) ([]string, error) {
	coll, err := s.legacyBuilderImagesCollection()
	if err != nil {
		return nil, err
	}
	defer coll.Close()
	var imageData appImages
	err = coll.Find(bson.M{"_id": appName}).One(&imageData)
	return imageData.Images, err
}

func (s *appVersionStorage) UpdateVersion(appName string, vi *appTypes.AppVersionInfo) error {
	now := time.Now().UTC()
	vi.UpdatedAt = now
	return s.baseUpdate(appName, bson.M{
		"$set": bson.M{
			fmt.Sprintf("versions.%d", vi.Version): vi,
			"updatedat":                            now,
		},
	})
}

func (s *appVersionStorage) UpdateVersionSuccess(appName string, vi *appTypes.AppVersionInfo) error {
	now := time.Now().UTC()
	vi.UpdatedAt = now
	return s.baseUpdate(appName, bson.M{
		"$set": bson.M{
			"lastsuccessfulversion":                vi.Version,
			"updatedat":                            now,
			fmt.Sprintf("versions.%d", vi.Version): vi,
		},
	})
}

func (s *appVersionStorage) baseUpdate(appName string, updateQuery bson.M) error {
	coll, err := s.collection()
	if err != nil {
		return err
	}
	defer coll.Close()
	err = coll.Update(bson.M{"appname": appName}, updateQuery)
	if err == mgo.ErrNotFound {
		return appTypes.ErrNoVersionsAvailable
	}
	return err
}

func (s *appVersionStorage) NewAppVersion(args appTypes.NewVersionArgs) (*appTypes.AppVersionInfo, error) {
	appVersions, err := s.AppVersions(args.App)
	if err != nil && err != appTypes.ErrNoVersionsAvailable {
		return nil, err
	}
	currentCount := appVersions.Count + 1

	now := time.Now().UTC()
	appVersionInfo := appTypes.AppVersionInfo{
		Description:    args.Description,
		Version:        currentCount,
		EventID:        args.EventID,
		CustomBuildTag: args.CustomBuildTag,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	coll, err := s.collection()
	if err != nil {
		return nil, err
	}
	defer coll.Close()
	_, err = coll.Upsert(bson.M{"appname": args.App.GetName()}, bson.M{
		"$set": bson.M{
			"count":     appVersionInfo.Version,
			"updatedat": time.Now().UTC(),
			fmt.Sprintf("versions.%d", appVersionInfo.Version): appVersionInfo,
		},
	})
	if err != nil {
		return nil, err
	}
	return &appVersionInfo, nil
}

func (s *appVersionStorage) DeleteVersions(appName string) error {
	coll, err := s.collection()
	if err != nil {
		return err
	}
	defer coll.Close()
	err = coll.Remove(bson.M{"appname": appName})
	if err == mgo.ErrNotFound {
		return nil
	}
	return err
}

func (s *appVersionStorage) AllAppVersions() ([]appTypes.AppVersions, error) {
	coll, err := s.collection()
	if err != nil {
		return nil, err
	}
	defer coll.Close()
	var allAppVersions []appTypes.AppVersions
	err = coll.Find(nil).All(&allAppVersions)
	if err != nil {
		return nil, err
	}
	return allAppVersions, nil
}

func (s *appVersionStorage) AppVersions(app appTypes.App) (appTypes.AppVersions, error) {
	coll, err := s.collection()
	if err != nil {
		return appTypes.AppVersions{}, err
	}
	defer coll.Close()
	var appVersions appTypes.AppVersions
	err = coll.Find(bson.M{"appname": app.GetName()}).One(&appVersions)
	if err == mgo.ErrNotFound {
		err = s.importLegacyVersions(app)
		if err == nil {
			err = coll.Find(bson.M{"appname": app.GetName()}).One(&appVersions)
		}
	}
	if err == mgo.ErrNotFound {
		return appVersions, appTypes.ErrNoVersionsAvailable
	}
	return appVersions, err
}

func (s *appVersionStorage) DeleteVersion(appName string, version int) error {
	err := s.baseUpdate(appName, bson.M{
		"$unset": bson.M{fmt.Sprintf("versions.%d", version): ""},
		"$set":   bson.M{"updatedat": time.Now().UTC()},
	})
	if err == mgo.ErrNotFound {
		return appTypes.ErrNoVersionsAvailable
	}
	return err
}

func (s *appVersionStorage) importLegacyVersions(app appTypes.App) error {
	imgData, err := s.legacyImagesData(app.GetName())
	if err != nil {
		return err
	}
	customDataColl, err := s.legacyCustomDataCollection()
	if err != nil {
		return err
	}
	defer customDataColl.Close()
	now := time.Now().UTC()
	versions := map[int]appTypes.AppVersionInfo{}
	var lastSuccessfulVersion int
	for _, imageID := range imgData.Images {
		var version int
		version, err = versionNumberFromLegacyImage(imageID)
		if err != nil {
			return err
		}
		var data legacyImageMetadata
		err = customDataColl.FindId(imageID).One(&data)
		if err != nil {
			if err == mgo.ErrNotFound {
				continue
			}
			return err
		}
		if len(data.Processes) == 0 {
			data.Processes = make(map[string][]string, len(data.LegacyProcesses))
			for k, v := range data.LegacyProcesses {
				data.Processes[k] = []string{v}
			}
		}
		vi := appTypes.AppVersionInfo{
			Version:          version,
			DeployImage:      imageID,
			Processes:        data.Processes,
			ExposedPorts:     data.ExposedPorts,
			CustomData:       data.CustomData,
			Disabled:         data.DisableRollback,
			DisabledReason:   data.Reason,
			DeploySuccessful: true,
			CreatedAt:        now,
			UpdatedAt:        now,
		}
		repo, tag := image.SplitImageName(vi.DeployImage)
		vi.BuildImage = fmt.Sprintf("%s:%s-builder", repo, tag)
		versions[version] = vi
		lastSuccessfulVersion = version
	}

	buildImgs, err := s.legacyBuildImages(app.GetName())
	if err != nil && err != mgo.ErrNotFound {
		return err
	}
	for _, imageID := range buildImgs {
		if strings.HasSuffix(imageID, "-builder") {
			continue
		}
		_, tag := image.SplitImageName(imageID)
		imgData.Count++
		version := imgData.Count
		vi := appTypes.AppVersionInfo{
			Version:        version,
			BuildImage:     imageID,
			CustomBuildTag: tag,
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		versions[version] = vi
	}

	coll, err := s.collection()
	if err != nil {
		return err
	}
	defer coll.Close()
	return coll.Insert(appTypes.AppVersions{
		AppName:               app.GetName(),
		Count:                 imgData.Count,
		Versions:              versions,
		LastSuccessfulVersion: lastSuccessfulVersion,
	})
}

func versionNumberFromLegacyImage(imageID string) (int, error) {
	_, tag := image.SplitImageName(imageID)
	return strconv.Atoi(strings.TrimPrefix(tag, "v"))
}
