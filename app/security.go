// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import appTypes "github.com/tsuru/tsuru/types/app"

var SuppressedEnv = "*** (private variable)"

func (a *App) SuppressSensitiveEnvs() {
	newEnv := map[string]appTypes.EnvVar{}
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
