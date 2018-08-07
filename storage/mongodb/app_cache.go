// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import "github.com/tsuru/tsuru/types/cache"

func appCacheStorage() cache.CacheStorage {
	return &cacheStorage{
		collection: "cache",
	}
}
