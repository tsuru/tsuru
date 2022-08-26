// Copyright 2022 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/tsuru/types/quota"
)

func teamQuotaStorage() quota.QuotaStorage {
	return &quotaStorage{
		collection: "teams",
		query: func(name string) bson.M {
			return bson.M{"_id": name}
		},
	}
}
