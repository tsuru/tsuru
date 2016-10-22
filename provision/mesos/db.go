// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mesos

import (
	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/storage"
)

const (
	uniqueDocumentID    = "mesos"
	mesosCollectionName = "mesosnodes"
)

type NodeAddrs struct {
	UniqueID  string `bson:"_id"`
	Addresses []string
}

func nodeAddrCollection() (*storage.Collection, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return conn.Collection(mesosCollectionName), nil
}
