// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"github.com/tsuru/tsuru/provision"
)

type ListNodeResponse struct {
	Nodes []provision.NodeSpec `json:"nodes"`
}

type InfoNodeResponse struct {
	Node  provision.NodeSpec `json:"node"`
	Units []provision.Unit   `json:"units"`
}
