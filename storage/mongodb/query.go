// Copyright 2026 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	"github.com/tsuru/tsuru/types/query"
	mongoBSON "go.mongodb.org/mongo-driver/bson"
)

// translateQuery converts a backend-neutral query.Query into a MongoDB filter.
func translateQuery(q query.Query) mongoBSON.M {
	m := mongoBSON.M{}
	for field, val := range q.Eq {
		m[field] = val
	}
	for field, vals := range q.In {
		m[field] = mongoBSON.M{"$in": vals}
	}
	for field, pattern := range q.Regex {
		m[field] = mongoBSON.M{"$regex": pattern}
	}
	for field, vals := range q.All {
		m[field] = mongoBSON.M{"$all": vals}
	}
	if len(q.Or) > 0 {
		or := make([]mongoBSON.M, len(q.Or))
		for i, sub := range q.Or {
			or[i] = translateQuery(sub)
		}
		m["$or"] = or
	}
	return m
}
