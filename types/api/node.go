// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"github.com/tsuru/tsuru/healer"
	"github.com/tsuru/tsuru/iaas"
	"github.com/tsuru/tsuru/provision"
)

type ListNodeResponse struct {
	Nodes    []provision.NodeSpec `json:"nodes"`
	Machines []iaas.Machine       `json:"machines"`
}

type InfoNodeResponse struct {
	Node   provision.NodeSpec    `json:"node"`
	Status healer.NodeStatusData `json:"status"`
	Units  []provision.Unit      `json:"units"`
}
