// Copyright 2014 docker-cluster authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cluster_test

import (
	"testing"

	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/docker-cluster/storage"
	"github.com/tsuru/docker-cluster/storage/mongodb"
)

func TestUpdateNodeDoesNotExist(t *testing.T) {
	mongo, err := mongodb.Mongodb("mongodb://localhost:27017", "test-docker-node-update")
	if err != nil {
		t.Fatal(err)
	}
	clu, err := cluster.New(nil, mongo, "")
	if err != nil {
		t.Fatal(err)
	}
	node := cluster.Node{Address: "http://localhost:4243"}
	err = clu.Register(node)
	defer clu.Unregister("http://localhost:4243")
	nodeUpd := cluster.Node{Address: "http://localhost:4223"}
	nodeUpd.Metadata = map[string]string{"k1": "v1", "k2": "v2"}
	nodeUpd, err = clu.UpdateNode(nodeUpd)
	if err != storage.ErrNoSuchNode {
		t.Error("Expected: No such node in storage, got: ", err)
	}
}
