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
		Collection: "events",
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

	{
		Collection: "pool_constraints",
		Indexes: []mongo.IndexModel{
			{
				Keys:    mongoBSON.D{{Key: "poolexpr", Value: 1}, {Key: "field", Value: 1}},
				Options: options.Index().SetUnique(true),
			},
		},
	},

	{
		Collection: "event_blocks",
		Indexes: []mongo.IndexModel{
			{
				Keys: mongoBSON.D{{Key: "ownername", Value: 1}, {Key: "kindname", Value: 1}, {Key: "target", Value: 1}},
			},
			{
				Keys: mongoBSON.D{{Key: "starttime", Value: -1}},
			},
			{
				Keys: mongoBSON.D{{Key: "active", Value: 1}, {Key: "starttime", Value: -1}},
			},
		},
	},

	{
		Collection: "platform_images",
		Indexes: []mongo.IndexModel{
			{
				Keys:    mongoBSON.D{{Key: "name", Value: 1}},
				Options: options.Index().SetUnique(true), //nolint
			},
		},
	},

	{
		Collection: "jobs",
		Indexes: []mongo.IndexModel{
			{
				Keys:    mongoBSON.D{{Key: "name", Value: 1}},
				Options: options.Index().SetUnique(true), //nolint
			},
		},
	},

	{
		Collection: "tokens",
		Indexes: []mongo.IndexModel{
			{
				Keys: mongoBSON.D{{Key: "token", Value: 1}},
			},
		},
	},

	{
		Collection: "users",
		Indexes: []mongo.IndexModel{
			{
				Keys:    mongoBSON.D{{Key: "email", Value: 1}},
				Options: options.Index().SetUnique(true),
			},
			{
				Keys: mongoBSON.D{{Key: "apikey", Value: 1}},
			},
		},
	},

	{
		Collection: "team_tokens",
		Indexes: []mongo.IndexModel{
			{
				Keys:    mongoBSON.D{{Key: "token", Value: 1}},
				Options: options.Index().SetUnique(true),
			},

			{
				Keys:    mongoBSON.D{{Key: "token_id", Value: 1}},
				Options: options.Index().SetUnique(true),
			},
		},
	},

	{
		Collection: "cache",
		Indexes: []mongo.IndexModel{
			{
				Keys:    mongoBSON.D{{Key: "expireat", Value: 1}},
				Options: options.Index().SetExpireAfterSeconds(1),
			},
		},
	},

	{
		Collection: "service_broker_catalog_cache",
		Indexes: []mongo.IndexModel{
			{
				Keys:    mongoBSON.D{{Key: "expireat", Value: 1}},
				Options: options.Index().SetExpireAfterSeconds(1),
			},
		},
	},
}

func EnsureIndexesCreated(db *mongo.Database) error {

	for _, index := range EnsureIndexes {

		collection := db.Collection(index.Collection)

		_, err := collection.Indexes().CreateMany(context.TODO(), index.Indexes)
		if err != nil {
			return err
		}
	}

	return nil
}
