// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dockercommon

import (
	"time"

	"github.com/tsuru/config"
	provTypes "github.com/tsuru/tsuru/types/provision"
)

const (
	defaultUsername = "ubuntu"
	defaultUserID   = 1000
)

func UserForContainer() (username string, uid *int64) {
	userid, err := config.GetInt("docker:uid")
	if err == nil {
		if userid >= 0 {
			userid64 := int64(userid)
			uid = &userid64
		}
	} else {
		defUID := int64(defaultUserID)
		uid = &defUID
	}
	username, err = config.GetString("docker:user")
	if err != nil {
		username, err = config.GetString("docker:ssh:user")
		if err != nil {
			username = defaultUsername
		}
	}
	return username, uid
}

func DeployHealthcheckTimeout(startupcheck *provTypes.TsuruYamlStartupcheck, healthcheck *provTypes.TsuruYamlHealthcheck) time.Duration {
	const defaultWaitSeconds = 120

	minWaitSeconds, _ := config.GetInt("docker:healthcheck:max-time")
	if minWaitSeconds <= 0 {
		minWaitSeconds = defaultWaitSeconds
	}

	var waitTime int
	if startupcheck != nil {
		waitTime = startupcheck.DeployTimeoutSeconds
	} else if healthcheck != nil {
		waitTime = healthcheck.DeployTimeoutSeconds
	}

	waitTime = max(waitTime, minWaitSeconds)

	return time.Duration(waitTime) * time.Second
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
