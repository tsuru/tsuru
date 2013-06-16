// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

var inspectOut = `
{
	"State": {
		"Running": false,
		"Pid": 0,
		"ExitCode": 0,
		"StartedAt": "2013-06-13T20:59:31.699407Z",
		"Ghost": false
	},
	"NetworkSettings": {
		"IpAddress": "10.10.10.10",
		"IpPrefixLen": 8,
		"Gateway": "10.65.41.1",
		"PortMapping": {"8888": "34233"}
	}
}`
