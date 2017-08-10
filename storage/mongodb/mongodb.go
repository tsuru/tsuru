// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import "github.com/tsuru/tsuru/storage"

func init() {
	mongodbDriver := storage.DbDriver{
		TeamService: &TeamService{},
	}
	storage.RegisterDbDriver("mongodb", mongodbDriver)
}
