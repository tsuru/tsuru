// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package integration

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path"
	"regexp"
	"strings"
	"testing"

	appTypes "github.com/tsuru/tsuru/types/app"
	check "gopkg.in/check.v1"
)

var dnsValidRegex = regexp.MustCompile(`(?i)[^a-z0-9.-]`)

func Test(t *testing.T) {
	check.TestingT(t)
}

type S struct {
	tmpDir string
	env    *Environment
}

var _ = check.Suite(&S{})

func (s *S) SetUpSuite(c *check.C) {
	var err error
	checkKubeconfig(c)
	s.tmpDir, err = os.MkdirTemp("", "tsuru-integration")
	c.Assert(err, check.IsNil)
	log.Printf("Using INTEGRATION HOME: %v", s.tmpDir)
	err = os.Setenv("HOME", s.tmpDir)
	c.Assert(err, check.IsNil)
	err = os.Setenv("TSURU_DISABLE_COLORS", "1")
	c.Assert(err, check.IsNil)
}

func checkKubeconfig(c *check.C) {
	defaultKubeConfig := path.Join(os.Getenv("HOME"), ".kube", "config")
	integrationKubeConfig := os.Getenv("INTEGRATION_KUBECONFIG")
	if integrationKubeConfig == "" {
		c.Skip("INTEGRATION_KUBECONFIG is not set, skipping kubernetes integration tests")
	}
	c.Assert(integrationKubeConfig, check.Not(check.Equals), "", check.Commentf("INTEGRATION_KUBECONFIG must be set to run integration tests"))
	c.Assert(integrationKubeConfig, check.Not(check.Equals), defaultKubeConfig, check.Commentf("INTEGRATION_KUBECONFIG must not be the default kubeconfig path"))
	os.Setenv("KUBECONFIG", integrationKubeConfig)
}

func (s *S) TearDownSuite(c *check.C) {
	err := os.RemoveAll(s.tmpDir)
	c.Assert(err, check.IsNil)
}

type resultTable struct {
	raw    string
	rows   [][]string
	header []string
}

func (r *resultTable) parse() {
	lines := strings.Split(r.raw, "\n")
	for _, line := range lines {
		if len(line) == 0 || line[0] != '|' {
			continue
		}
		parts := strings.Split(strings.Trim(line, "|"), "|")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		if r.header == nil {
			r.header = append(r.header, parts...)
			continue
		}
		if len(parts) != len(r.header) {
			continue
		}
		if parts[0] == "" {
			for i := range r.rows[len(r.rows)-1] {
				if parts[i] == "" {
					continue
				}
				r.rows[len(r.rows)-1][i] += "\n" + parts[i]
			}
		} else {
			r.rows = append(r.rows, parts)
		}
	}
}

func slugifyName(name string) string {
	return strings.ToLower(dnsValidRegex.ReplaceAllString(name, "-"))
}

func getAppInfo(c *check.C, appName string, env *Environment) *appTypes.AppInfo {
	res := T("app", "info", "-a", appName, "--json").Run(env)
	c.Assert(res, ResultOk)

	appInfo := new(appTypes.AppInfo)
	err := json.NewDecoder(&res.Stdout).Decode(appInfo)
	c.Assert(err, check.IsNil)
	return appInfo
}

func checkAppReady(c *check.C, appName string, env *Environment) (*appTypes.AppInfo, bool) {
	appInfo := getAppInfo(c, appName, env)

	for _, unit := range appInfo.Units {
		if unit.Ready == nil || !(*unit.Ready) {
			fmt.Printf("DEBUG: unit not ready yet: %s\n", unit.Name)
			return nil, false
		}
	}
	return appInfo, true
}

func checkReadyUnits(c *check.C, appName string, env *Environment, amount int) (*appTypes.AppInfo, bool) {
	appInfo, ok := checkAppReady(c, appName, env)
	if !ok {
		return nil, false
	}
	count := 0
	for _, unit := range appInfo.Units {
		if unit.Ready != nil && *unit.Ready {
			count++
		}
	}
	return appInfo, count == amount
}

func checkAppExternallyAddressable(c *check.C, appName string, env *Environment) (*appTypes.AppInfo, bool) {
	appInfo, ok := checkAppReady(c, appName, env)
	if !ok {
		return nil, false
	}
	c.Assert(appInfo.Routers, check.HasLen, 1)
	c.Assert(appInfo.Routers[0].Addresses, check.HasLen, 1)
	if len(appInfo.Routers[0].Addresses[0]) == 0 {
		fmt.Printf("DEBUG: app not externally addressable yet: %s\n", appName)
		return nil, false
	}
	return appInfo, true
}

func checkAppStopped(c *check.C, appName string, env *Environment) (*appTypes.AppInfo, bool) {
	res := T("app", "info", "-a", appName, "--json").Run(env)
	c.Assert(res, ResultOk)

	appInfo := new(appTypes.AppInfo)
	err := json.NewDecoder(&res.Stdout).Decode(appInfo)
	c.Assert(err, check.IsNil)

	return appInfo, len(appInfo.Units) == 0
}
