// Copyright 2022 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	"github.com/tsuru/tsuru/types/quota"
	mongoBSON "go.mongodb.org/mongo-driver/bson"
)

func teamQuotaStorage() quota.QuotaStorage {
	return &quotaStorage{
		collection: "teams",
		query: func(name string) mongoBSON.M {
			return mongoBSON.M{"_id": name}
		},
	}
}
