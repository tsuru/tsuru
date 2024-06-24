// Copyright 2024 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storagev2

import (
	"context"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	DefaultDatabaseURL  = "mongodb://127.0.0.1:27017"
	DefaultDatabaseName = "tsuru"
)

var (
	client          atomic.Pointer[mongo.Client]
	databaseNamePtr atomic.Pointer[string]
)

func Reset() {
	client.Store(nil)
	databaseNamePtr.Store(nil)
}

func connect() (*mongo.Client, *string, error) {
	var uri string

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	uri, databaseName := dbConfig()

	connectedClient, err := mongo.Connect(
		ctx,
		options.Client().
			ApplyURI(uri).
			SetAppName("tsurud"),
	)
	if err != nil {
		return nil, nil, err
	}

	swapped := client.CompareAndSwap(nil, connectedClient)
	databaseNamePtr.Store(&databaseName)

	if swapped {
		err = EnsureIndexesCreated(connectedClient.Database(databaseName))

		if err != nil {
			return nil, nil, errors.Wrap(err, "failed to create indexes")
		}
	}

	return connectedClient, &databaseName, nil
}

func dbConfig() (string, string) {
	uri, _ := config.GetString("database:url")
	if uri == "" {
		uri = DefaultDatabaseURL
	}

	if !strings.HasPrefix(uri, "mongodb://") {
		uri = "mongodb://" + uri
	}

	uriParsed, _ := url.Parse(uri)

	if uriParsed.Path == "" {
		uriParsed.Path = "/"
	}

	dbname, _ := config.GetString("database:name")
	if dbname == "" {
		dbname = DefaultDatabaseName
	}

	return uriParsed.String(), dbname
}

func Collection(name string) (*mongo.Collection, error) {
	connectedClient := client.Load()
	databaseName := databaseNamePtr.Load()

	if connectedClient == nil || databaseName == nil {
		var err error
		connectedClient, databaseName, err = connect()
		if err != nil {
			return nil, err
		}
	}
	return connectedClient.Database(*databaseName).Collection(name, options.Collection()), nil
}
