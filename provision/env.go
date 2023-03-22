// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package provision

import (
	"fmt"
	"sort"
	"strconv"

	"github.com/tsuru/config"
	appTypes "github.com/tsuru/tsuru/types/app"
)

func WebProcessDefaultPort() string {
	port, err := config.Get("docker:run-cmd:port")
	if err != nil {
		return "8888"
	}
	return fmt.Sprint(port)
}

func EnvsForApp(a App, process string, isDeploy bool, version appTypes.AppVersion) []appTypes.EnvVar {
	var envs []appTypes.EnvVar
	if !isDeploy {
		for _, envData := range a.Envs() {
			envs = append(envs, envData)
		}
		sort.Slice(envs, func(i int, j int) bool {
			return envs[i].Name < envs[j].Name
		})
		envs = append(envs, appTypes.EnvVar{Name: "TSURU_PROCESSNAME", Value: process})
		if version != nil {
			envs = append(envs, appTypes.EnvVar{Name: "TSURU_APPVERSION", Value: strconv.Itoa(version.Version())})
		}
	}
	host, _ := config.GetString("host")
	envs = append(envs, appTypes.EnvVar{Name: "TSURU_HOST", Value: host})
	if !isDeploy {
		envs = append(envs, DefaultWebPortEnvs()...)
	}
	return envs
}

func DefaultWebPortEnvs() []appTypes.EnvVar {
	port := WebProcessDefaultPort()
	return []appTypes.EnvVar{
		{Name: "port", Value: port},
		{Name: "PORT", Value: port},
	}
}
