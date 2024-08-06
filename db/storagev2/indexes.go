// Copyright 2024 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storagev2

import (
	"context"
	"fmt"
	"sync"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/log"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	mongoBSON "go.mongodb.org/mongo-driver/bson"
)

type EnsureIndex struct {
	Collection        string
	GetCollectionName func() string
	Indexes           []mongo.IndexModel
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

	{
		Collection: "webhook",
		Indexes: []mongo.IndexModel{
			{
				Keys:    mongoBSON.D{{Key: "name", Value: 1}},
				Options: options.Index().SetUnique(true),
			},
		},
	},

	{
		Collection: "service_broker",
		Indexes: []mongo.IndexModel{
			{
				Keys:    mongoBSON.D{{Key: "name", Value: 1}},
				Options: options.Index().SetUnique(true),
			},
		},
	},

	{
		GetCollectionName: getOAuthTokensCollectionName,
		Indexes: []mongo.IndexModel{
			{
				Keys: mongoBSON.D{{Key: "token.accesstoken", Value: 1}},
			},
			{
				Keys: mongoBSON.D{{Key: "useremail", Value: 1}},
			},
		},
	},
}

var notifyDefaultAuthToken sync.Once

func getOAuthTokensCollectionName() string {
	name, _ := config.GetString("auth:oauth:collection")
	if name == "" {
		notifyDefaultAuthToken.Do(func() {
			log.Debugf("auth:oauth:collection not found using default value: %s.", name)
		})
		return "oauth_tokens"
	}

	return name
}

func EnsureIndexesCreated(db *mongo.Database) error {
	for i, index := range EnsureIndexes {
		collectionName := index.Collection

		if collectionName == "" && index.GetCollectionName != nil {
			collectionName = index.GetCollectionName()
		}

		if collectionName == "" {
			return fmt.Errorf("CollectionName or GetCollectionName must be defined on index %d", i)
		}

		collection := db.Collection(collectionName)

		_, err := collection.Indexes().CreateMany(context.TODO(), index.Indexes)
		if err != nil {
			return err
		}
	}

	return nil
}
