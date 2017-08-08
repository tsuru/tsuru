// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

// Team represents a real world team, a team has one creating user and a name.
type Team struct {
	Name         string `json:"name"`
	CreatingUser string
}
