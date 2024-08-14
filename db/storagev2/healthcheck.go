package storagev2

import (
	"context"

	"github.com/tsuru/tsuru/hc"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

// Copyright 2024 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

func init() {
	hc.AddChecker("MongoDB", healthCheck)
}

func healthCheck(ctx context.Context) error {
	appsCollection, err := AppsCollection()
	if err != nil {
		return err
	}

	return appsCollection.Database().Client().Ping(ctx, readpref.Primary())
}
