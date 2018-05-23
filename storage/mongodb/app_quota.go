// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/tsuru/types/quota"
)

func appQuotaStorage() quota.QuotaStorage {
	return &quotaStorage{
		collection: "apps",
		query: func(name string) bson.M {
			return bson.M{"name": name}
		},
	}
}
