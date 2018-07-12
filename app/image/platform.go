// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package image

import (
	"fmt"

	"github.com/globalsign/mgo"
	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/storage"
	"github.com/tsuru/tsuru/log"
)

type platformImages struct {
	PlatformName string `bson:"_id"`
	Images       []string
	Count        int
}

func platformImagesColl() (*storage.Collection, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	name, err := config.GetString("docker:collection")
	if err != nil {
		name = defaultCollection
	}
	return conn.Collection(fmt.Sprintf("%s_platform_image", name)), nil
}

func PlatformNewImage(platformName string) (string, error) {
	coll, err := platformImagesColl()
	if err != nil {
		return "", err
	}
	defer coll.Close()
	var imgs platformImages
	dbChange := mgo.Change{
		Update:    bson.M{"$inc": bson.M{"count": 1}},
		ReturnNew: true,
		Upsert:    true,
	}
	_, err = coll.FindId(platformName).Apply(dbChange, &imgs)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s/%s:v%d", basicImageName("tsuru"), platformName, imgs.Count), nil
}

func PlatformCurrentImage(platformName string) (string, error) {
	coll, err := platformImagesColl()
	if err != nil {
		return "", err
	}
	defer coll.Close()
	var imgs platformImages
	err = coll.FindId(platformName).One(&imgs)
	if err != nil {
		log.Errorf("Couldn't find images for platform %q, fallback to default image name. Error: %s", platformName, err)
		return platformBasicImageName(platformName), nil
	}
	if len(imgs.Images) == 0 && imgs.Count > 0 {
		log.Errorf("Couldn't find valid images for platform %q", platformName)
		return platformBasicImageName(platformName), nil
	}
	if len(imgs.Images) == 0 {
		return "", ErrNoImagesAvailable
	}
	return imgs.Images[len(imgs.Images)-1], nil
}

func PlatformAppendImage(platformName, imageID string) error {
	coll, err := platformImagesColl()
	if err != nil {
		return err
	}
	defer coll.Close()
	bulk := coll.Bulk()
	bulk.Upsert(bson.M{"_id": platformName}, bson.M{"$pull": bson.M{"images": imageID}})
	bulk.Upsert(bson.M{"_id": platformName}, bson.M{"$push": bson.M{"images": imageID}})
	_, err = bulk.Run()
	return err
}

func PlatformDeleteImages(platformName string) error {
	coll, err := platformImagesColl()
	if err != nil {
		return err
	}
	defer coll.Close()
	err = coll.RemoveId(platformName)
	if err != nil && err != mgo.ErrNotFound {
		return err
	}
	return nil
}

func PlatformListImages(platformName string) ([]string, error) {
	coll, err := platformImagesColl()
	if err != nil {
		return nil, err
	}
	defer coll.Close()
	var img platformImages
	err = coll.FindId(platformName).One(&img)
	if err != nil {
		return nil, err
	}
	return img.Images, nil
}

// PlatformListImagesOrDefault returns basicImageName when platform is empty
// for backwards compatibility
func PlatformListImagesOrDefault(platformName string) ([]string, error) {
	imgs, err := PlatformListImages(platformName)
	if err != nil && err == mgo.ErrNotFound {
		return []string{platformBasicImageName(platformName)}, nil
	}
	return imgs, err
}

func platformBasicImageName(platformName string) string {
	return fmt.Sprintf("%s/%s:latest", basicImageName("tsuru"), platformName)
}
