// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"github.com/globocom/tsuru/db"
)

type Platform struct {
	Name string `bson:"_id"`
}

// Platforms returns the list of available platforms.
func Platforms() ([]Platform, error) {
	var platforms []Platform
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	err = conn.Platforms().Find(nil).All(&platforms)
	return platforms, err
}
