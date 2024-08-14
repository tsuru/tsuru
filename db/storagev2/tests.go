// Copyright 2024 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storagev2

import (
	"context"
	"errors"
	"strings"
	"testing"

	mongoBSON "go.mongodb.org/mongo-driver/bson"
)

func ClearAllCollections(toKeep []string) error {
	if !testing.Testing() {
		return errors.New("ClearAllCollections should only be used in tests")
	}

	ctx := context.Background()
	db, err := database()
	if err != nil {
		return err
	}

	collections, err := db.ListCollectionNames(ctx, mongoBSON.M{})
	if err != nil {
		return err
	}
nextColl:
	for _, collection := range collections {
		if strings.Contains(collection, "system.") {
			continue
		}
		for i := range toKeep {
			if collection == toKeep[i] {
				continue nextColl
			}
		}

		_, err = db.Collection(collection).DeleteMany(ctx, mongoBSON.M{})

		if err != nil {
			return err
		}
	}
	return nil
}
