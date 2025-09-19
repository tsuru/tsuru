// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package provision

import (
	"fmt"
	"sort"
	"strconv"

	"github.com/tsuru/config"
	tsuruEnvs "github.com/tsuru/tsuru/envs"
	appTypes "github.com/tsuru/tsuru/types/app"
	bindTypes "github.com/tsuru/tsuru/types/bind"
)

func WebProcessDefaultPort() string {
	port, err := config.Get("docker:run-cmd:port")
	if err != nil {
		return "8888"
	}
	return fmt.Sprint(port)
}

// Envs returns a map representing the apps environment variables.
func EnvsForApp(app *appTypes.App) map[string]bindTypes.EnvVar {
	mergedEnvs := make(map[string]bindTypes.EnvVar, len(app.Env)+len(app.ServiceEnvs)+1)
	toInterpolate := make(map[string]string)
	var toInterpolateKeys []string
	for _, e := range app.Env {
		mergedEnvs[e.Name] = e
		if e.Alias != "" {
			toInterpolate[e.Name] = e.Alias
			toInterpolateKeys = append(toInterpolateKeys, e.Name)
		}
	}
	for _, e := range app.ServiceEnvs {
		envVar := e.EnvVar
		envVar.ManagedBy = fmt.Sprintf("%s/%s", e.ServiceName, e.InstanceName)
		mergedEnvs[e.Name] = envVar
	}
	sort.Strings(toInterpolateKeys)
	for _, envName := range toInterpolateKeys {
		tsuruEnvs.Interpolate(mergedEnvs, toInterpolate, envName, toInterpolate[envName])
	}
	mergedEnvs[tsuruEnvs.TsuruServicesEnvVar] = tsuruEnvs.ServiceEnvsFromEnvVars(app.ServiceEnvs)

	mergedEnvs["TSURU_APPNAME"] = bindTypes.EnvVar{
		Name:      "TSURU_APPNAME",
		Value:     app.Name,
		ManagedBy: "tsuru",
		Public:    true,
	}

	mergedEnvs["TSURU_APPDIR"] = bindTypes.EnvVar{
		Name:      "TSURU_APPDIR",
		Value:     appTypes.DefaultAppDir,
		ManagedBy: "tsuru",
		Public:    true,
	}

	return mergedEnvs
}

func EnvsForAppAndVersion(a *appTypes.App, process string, version appTypes.AppVersion) []bindTypes.EnvVar {
	var envs []bindTypes.EnvVar

	for _, envData := range EnvsForApp(a) {
		envs = append(envs, envData)
	}
	sort.Slice(envs, func(i int, j int) bool {
		return envs[i].Name < envs[j].Name
	})
	envs = append(envs, bindTypes.EnvVar{Name: "TSURU_PROCESSNAME", Value: process, Public: true})
	if version != nil {
		envs = append(envs, bindTypes.EnvVar{Name: "TSURU_APPVERSION", Value: strconv.Itoa(version.Version()), Public: true})
	}

	host, _ := config.GetString("host")
	envs = append(envs, bindTypes.EnvVar{Name: "TSURU_HOST", Value: host, Public: true})

	envs = append(envs, DefaultWebPortEnvs()...)

	return envs
}

func DefaultWebPortEnvs() []bindTypes.EnvVar {
	port := WebProcessDefaultPort()
	return []bindTypes.EnvVar{
		{Name: "port", Value: port, Public: true},
		{Name: "PORT", Value: port, Public: true},
	}
}
