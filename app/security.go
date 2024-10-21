// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	appTypes "github.com/tsuru/tsuru/types/app"
	bindTypes "github.com/tsuru/tsuru/types/bind"
)

var SuppressedEnv = "*** (private variable)"

func SuppressSensitiveEnvs(a *appTypes.App) {
	newEnv := map[string]bindTypes.EnvVar{}
	for key, env := range a.Env {
		if !env.Public {
			env.Value = SuppressedEnv
		}
		newEnv[key] = env
	}
	a.Env = newEnv

	for i, serviceEnv := range a.ServiceEnvs {
		if !serviceEnv.EnvVar.Public {
			a.ServiceEnvs[i].Value = SuppressedEnv
		}
	}
}
