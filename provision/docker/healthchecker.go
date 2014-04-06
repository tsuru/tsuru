// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import "github.com/tsuru/tsuru/provision"

// isReachable returns true if the web application deploy in the
// unit is accessible via 0.0.0.0:PORT.
func IsReachable(unit provision.AppUnit) (bool, error) {
	return false, nil
}
