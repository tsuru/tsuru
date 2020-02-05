// Copyright 2019 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package migrate

import (
	"fmt"

	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/storage"
)

func MigrateExposedPorts() error {
	coll, err := imageCustomDataColl()
	if err != nil {
		return err
	}
	defer coll.Close()

	var img struct {
		Name         string `bson:"_id"`
		ExposedPort  string
		ExposedPorts []string
	}
	iter := coll.Find(nil).Iter()
	for iter.Next(&img) {
		if len(img.ExposedPorts) == 0 && len(img.ExposedPort) != 0 {
			err = coll.UpdateId(img.Name, bson.M{"$set": bson.M{"exposedports": []string{img.ExposedPort}}})
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func imageCustomDataColl() (*storage.Collection, error) {
	const defaultCollection = "docker"
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	name, err := config.GetString("docker:collection")
	if err != nil {
		name = defaultCollection
	}
	return conn.Collection(fmt.Sprintf("%s_image_custom_data", name)), nil
}
