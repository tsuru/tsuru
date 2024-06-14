// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	"github.com/tsuru/tsuru/types/quota"
	mongoBSON "go.mongodb.org/mongo-driver/bson"
)

func appQuotaStorage() quota.QuotaStorage {
	return &quotaStorage{
		collection: "apps",
		query: func(name string) mongoBSON.M {
			return mongoBSON.M{"name": name}
		},
	}
}
