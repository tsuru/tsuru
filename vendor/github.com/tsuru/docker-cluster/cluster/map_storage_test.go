// Copyright 2014 docker-cluster authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cluster_test

import (
	"testing"

	"github.com/tsuru/docker-cluster/cluster"
	storageTesting "github.com/tsuru/docker-cluster/storage/testing"
)

func TestMapStorageStorage(t *testing.T) {
	storageTesting.RunTestsForStorage(&cluster.MapStorage{}, t)
}
