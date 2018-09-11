// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

type AppRouter struct {
	Name         string            `json:"name"`
	Opts         map[string]string `json:"opts"`
	Address      string            `json:"address" bson:"-"`
	Type         string            `json:"type" bson:"-"`
	Status       string            `json:"status,omitempty" bson:"-"`
	StatusDetail string            `json:"status-detail,omitempty" bson:"-"`
}
