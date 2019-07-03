// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

// Envs represents the configuration of an environment variable data
// for the remote API
type Envs struct {
	Envs      []struct{ Name, Value, Alias string }
	NoRestart bool
	Private   bool
}
