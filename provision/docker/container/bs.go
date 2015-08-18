// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package container

import "github.com/tsuru/config"

func BsSysLogPort() int {
	bsPort, _ := config.GetInt("docker:bs:syslog-port")
	if bsPort == 0 {
		bsPort = 1514
	}
	return bsPort
}
