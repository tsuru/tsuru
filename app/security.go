// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"github.com/tsuru/tsuru/app/bind"
)

const suppressedEnv = "****"

func (a *App) SuppressSensitiveEnvs() {
	newEnv := map[string]bind.EnvVar{}
	for key, env := range a.Env {
		if !env.Public {
			env.Value = suppressedEnv
		}
		newEnv[key] = env
	}
	a.Env = newEnv

	for i, serviceEnv := range a.ServiceEnvs {
		if !serviceEnv.EnvVar.Public {
			a.ServiceEnvs[i].Value = suppressedEnv
		}
	}
}
