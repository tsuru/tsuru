// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package integration

import (
	"os"
	"path"
	"strings"

	check "gopkg.in/check.v1"
)

func (s *S) getPlatforms() []string {
	availablePlatforms := []string{
		"tsuru/python",
		"tsuru/go",
		"tsuru/cordova",
		"tsuru/elixir",
		"tsuru/java",
		"tsuru/nodejs",
		"tsuru/php",
		"tsuru/play",
		"tsuru/ruby",
		"tsuru/static",
		"tsuru/perl",
	}
	if _, ok := os.LookupEnv(integrationEnvID + "platforms"); !ok {
		return availablePlatforms
	}
	envPlatforms := s.env.All("platforms")
	selectedPlatforms := make([]string, 0, len(availablePlatforms))
	for _, name := range envPlatforms {
		name = strings.Trim(name, " ")
		for i, platform := range availablePlatforms {
			if name == platform || "tsuru/"+name == platform {
				selectedPlatforms = append(selectedPlatforms, platform)
				availablePlatforms = append(availablePlatforms[:i], availablePlatforms[i+1:]...)
				break
			}
		}
	}
	return selectedPlatforms
}

func (s *S) config(c *check.C) {
	checkKubeconfig(c)
	env := NewEnvironment()
	if !env.Has("enabled") {
		return
	}
	s.env = env
	platforms = s.getPlatforms()
}

func checkKubeconfig(c *check.C) {
	defaultKubeConfig := path.Join(os.Getenv("HOME"), ".kube", "config")
	integrationKubeConfig := os.Getenv("INTEGRATION_KUBECONFIG")
	c.Assert(integrationKubeConfig, check.Not(check.Equals), "", check.Commentf("INTEGRATION_KUBECONFIG must be set to run integration tests"))
	c.Assert(integrationKubeConfig, check.Not(check.Equals), defaultKubeConfig, check.Commentf("INTEGRATION_KUBECONFIG must not be the default kubeconfig path"))
	os.Setenv("KUBECONFIG", integrationKubeConfig)
}
