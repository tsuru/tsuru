// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/storage"
)

const (
	uniqueDocumentID         = "kubernetes"
	kubernetesCollectionName = "kubernetesnodes"
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
	return conn.Collection(kubernetesCollectionName), nil
}
