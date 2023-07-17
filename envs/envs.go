// Copyright 2023 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package envs

import (
	"encoding/json"

	bindTypes "github.com/tsuru/tsuru/types/bind"
)

const TsuruServicesEnvVar = "TSURU_SERVICES"

func Interpolate(mergedEnvs map[string]bindTypes.EnvVar, toInterpolate map[string]string, envName, varName string) {
	delete(toInterpolate, envName)
	if toInterpolate[varName] != "" {
		Interpolate(mergedEnvs, toInterpolate, envName, toInterpolate[varName])
		return
	}
	if _, isSet := mergedEnvs[varName]; !isSet {
		return
	}
	env := mergedEnvs[envName]
	env.Value = mergedEnvs[varName].Value
	mergedEnvs[envName] = env
}

func ServiceEnvsFromEnvVars(vars []bindTypes.ServiceEnvVar) bindTypes.EnvVar {
	type serviceInstanceEnvs struct {
		InstanceName string            `json:"instance_name"`
		Envs         map[string]string `json:"envs"`
	}
	result := map[string][]serviceInstanceEnvs{}
	for _, v := range vars {
		found := false
		for i, instanceList := range result[v.ServiceName] {
			if instanceList.InstanceName == v.InstanceName {
				result[v.ServiceName][i].Envs[v.Name] = v.Value
				found = true
				break
			}
		}
		if !found {
			result[v.ServiceName] = append(result[v.ServiceName], serviceInstanceEnvs{
				InstanceName: v.InstanceName,
				Envs:         map[string]string{v.Name: v.Value},
			})
		}
	}
	jsonVal, _ := json.Marshal(result)
	return bindTypes.EnvVar{
		Name:   TsuruServicesEnvVar,
		Value:  string(jsonVal),
		Public: false,
	}
}
