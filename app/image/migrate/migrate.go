// Copyright 2019 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package migrate

import (
	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/tsuru/app/image"
)

func MigrateExposedPorts() error {
	coll, err := image.ImageCustomDataColl()
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
