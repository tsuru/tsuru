// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

// Envs represents the configuration of an environment variable data
// for the remote API
type Envs struct {
	Envs        []Env
	ManagedBy   string `json:"managedBy"`
	NoRestart   bool
	Private     bool
	PruneUnused bool `json:"pruneUnused"`
}

type Env struct {
	Name      string
	Value     string
	Alias     string
	Private   *bool  `json:"private,omitempty"`
	ManagedBy string `json:"-" bson:"managedBy"`
}
