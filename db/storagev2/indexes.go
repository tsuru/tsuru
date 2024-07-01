// Copyright 2024 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storagev2

import (
	"context"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	mongoBSON "go.mongodb.org/mongo-driver/bson"
)

type EnsureIndex struct {
	Collection string
	Indexes    []mongo.IndexModel
}

var EnsureIndexes = []EnsureIndex{
	{
		Collection: "events", // Assuming the collection name is "events"
		Indexes: []mongo.IndexModel{
			{
				Keys: mongoBSON.D{{Key: "owner.name", Value: 1}},
			},
			{
				Keys: mongoBSON.D{{Key: "target.value", Value: 1}},
			},
			{
				Keys: mongoBSON.D{{Key: "extratargets.target.value", Value: 1}},
			},
			{
				Keys: mongoBSON.D{{Key: "kind.name", Value: 1}},
			},
			{
				Keys: mongoBSON.D{{Key: "starttime", Value: -1}},
			},
			{
				Keys: mongoBSON.D{{Key: "uniqueid", Value: 1}},
			},
			{
				Keys:    mongoBSON.D{{Key: "running", Value: 1}},
				Options: &options.IndexOptions{},
			},
			{
				Keys: mongoBSON.D{{Key: "allowed.scheme", Value: 1}},
			},
			{
				Keys:    mongoBSON.D{{Key: "target.value", Value: 1}, {Key: "kind.name", Value: 1}, {Key: "starttime", Value: -1}},
				Options: options.Index().SetBackground(true), //nolint
			},
			{
				Keys:    mongoBSON.D{{Key: "target.value", Value: 1}, {Key: "starttime", Value: -1}},
				Options: options.Index().SetBackground(true), //nolint
			},
			{
				Keys:    mongoBSON.D{{Key: "extratargets.target.value", Value: 1}, {Key: "starttime", Value: -1}},
				Options: options.Index().SetBackground(true), //nolint
			},
			{
				Keys:    mongoBSON.D{{Key: "lock", Value: 1}},
				Options: options.Index().SetBackground(true).SetSparse(true).SetUnique(true).SetBackground(true), //nolint
			},
		},
	},
}

func EnsureIndexesCreated(db *mongo.Database) error {

	for _, index := range EnsureIndexes {

		collection := db.Collection(index.Collection) // Replace "events" with the actual name of your collection

		_, err := collection.Indexes().CreateMany(context.TODO(), index.Indexes)
		if err != nil {
			return err
		}
	}

	return nil
}
