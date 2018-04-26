// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package provision

import (
	"fmt"
	"sort"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app/bind"
)

func WebProcessDefaultPort() string {
	port, err := config.Get("docker:run-cmd:port")
	if err != nil {
		return "8888"
	}
	return fmt.Sprint(port)
}

func EnvsForApp(a App, process string, isDeploy bool) []bind.EnvVar {
	var envs []bind.EnvVar
	if !isDeploy {
		for _, envData := range a.Envs() {
			envs = append(envs, envData)
		}
		sort.Slice(envs, func(i int, j int) bool {
			return envs[i].Name < envs[j].Name
		})
		envs = append(envs, bind.EnvVar{Name: "TSURU_PROCESSNAME", Value: process})
	}
	host, _ := config.GetString("host")
	envs = append(envs, bind.EnvVar{Name: "TSURU_HOST", Value: host})
	if !isDeploy {
		port := WebProcessDefaultPort()
		envs = append(envs, []bind.EnvVar{
			{Name: "port", Value: port},
			{Name: "PORT", Value: port},
		}...)
	}
	return envs
}
