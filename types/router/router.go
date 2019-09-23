// Copyright 2019 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package router

type HealthcheckData struct {
	Path    string
	Status  int
	Body    string
	TCPOnly bool
}
