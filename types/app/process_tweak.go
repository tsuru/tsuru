// Copyright 2023 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

type Process struct {
	Name     string   `json:"name"` // name of process, it is like a merge key
	Plan     string   `json:"plan,omitempty"`
	Metadata Metadata `json:"metadata"`
}

func (p *Process) Empty() bool {
	return p.Plan == "" && p.Metadata.Empty()
}
