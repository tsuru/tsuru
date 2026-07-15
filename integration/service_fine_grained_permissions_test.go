// Copyright 2026 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package integration

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	appTypes "github.com/tsuru/tsuru/types/app"
	check "gopkg.in/check.v1"
)

func serviceFineGrainedPermissionsFlow() ExecFlow {
	flow := ExecFlow{
		requires: []string{"servicename", "bindnames", "team", "adminuser"},
	}

	flow.forward = func(c *check.C, env *Environment) {
		targetAddr := env.Get("targetaddr")
		servicename := env.Get("servicename")
		bindname := env.Get("bindnames")
		adminUser := env.Get("adminuser")
		teamName := env.Get("team")
		roleName := fmt.Sprintf("integration-fg-%s-%s", servicename, time.Now().Format("150405.999999999"))
		roleName = strings.ReplaceAll(roleName, ".", "-")
		env.Set("fgRole", roleName)

		manifestPayload := readManifestPayload(c, path.Join("fixtures", "service", "manifest-fine-grained.json"))
		var manifest serviceManifestPayload
		err := json.Unmarshal(manifestPayload, &manifest)
		c.Assert(err, check.IsNil)
		c.Assert(len(manifest.Operations), check.Not(check.Equals), 0)
		var permissionName string
		for _, op := range manifest.Operations {
			if strings.EqualFold(op.Method, "POST") && strings.Contains(op.Path, "sync") && op.Action != "" {
				permissionName = fmt.Sprintf("service-action.%s.%s", servicename, op.Action)
				break
			}
		}
		if permissionName == "" {
			permissionName = fmt.Sprintf("service-action.%s.%s", servicename, manifest.Operations[0].Action)
		}
		c.Assert(permissionName, check.Not(check.Equals), "")
		env.Set("fgPermission", permissionName)

		res := T("token", "show").Run(env)
		c.Assert(res, ResultOk)
		token := parseAPIToken(c, res.Stdout.String())
		env.Set("apitoken", token)
		authHeader := fmt.Sprintf("'Authorization: Bearer %s'", token)

		manifestURL := fmt.Sprintf("%s/1.31/services/%s/manifest", targetAddr, servicename)
		res = NewCommand("curl", "-sS", "-X", "PUT", manifestURL,
			"-H", authHeader,
			"-H", "'Content-Type: application/json'",
			"-d", fmt.Sprintf("'%s'", string(manifestPayload))).Run(env)
		c.Assert(res, ResultOk)

		res = NewCommand("curl", "-sS", "-X", "GET", manifestURL,
			"-H", authHeader).Run(env)
		c.Assert(res, ResultOk)
		var readManifest serviceManifestPayload
		err = json.Unmarshal([]byte(res.Stdout.String()), &readManifest)
		c.Assert(err, check.IsNil)
		c.Assert(readManifest.Enabled, check.Equals, manifest.Enabled)
		c.Assert(readManifest.StrictActions, check.Equals, manifest.StrictActions)
		c.Assert(readManifest.Operations, check.Not(check.Equals), nil)

		res = T("permission", "list").Run(env)
		c.Assert(res, ResultOk)
		c.Assert(strings.Contains(res.Stdout.String(), permissionName), check.Equals, true)

		res = T("role", "add", roleName, "team").Run(env)
		c.Assert(res, ResultOk)

		res = T("role", "assign", roleName, adminUser, teamName).Run(env)
		c.Assert(res, ResultOk)

		res = T("role", "permission", "add", roleName, permissionName).Run(env)
		c.Assert(res, ResultOk)

		res = NewCommand("curl", "-sS", "-X", "GET", fmt.Sprintf("%s/1.0/roles/%s", targetAddr, roleName),
			"-H", authHeader).Run(env)
		c.Assert(res, ResultOk)
		var roleInfo struct {
			DynamicSchemeNames []string `json:"dynamic_scheme_names"`
		}
		err = json.Unmarshal([]byte(res.Stdout.String()), &roleInfo)
		c.Assert(err, check.IsNil)
		c.Assert(roleInfo.DynamicSchemeNames, check.DeepEquals, []string{permissionName})

		proxyStatus := func(proxyPath string) string {
			urlToCall := fmt.Sprintf("'%s/1.0/services/%s/proxy/%s?callback=%s'", targetAddr, servicename, bindname, url.PathEscape(proxyPath))
			res = NewCommand("curl", "-sS", "-o", "/dev/null", "-w", "'%{http_code}'",
				"-X", "POST",
				urlToCall,
				"-H", authHeader).Run(env)
			c.Assert(res, ResultOk)
			return strings.TrimSpace(res.Stdout.String())
		}

		c.Assert(proxyStatus("/resources/"+bindname+"/rules/123"), check.Equals, "403")

		c.Assert(proxyStatus("/resources/"+bindname+"/rules/123/sync"), check.Not(check.Equals), "403")
	}

	flow.backward = func(c *check.C, env *Environment) {
		targetAddr := env.Get("targetaddr")
		roleName := env.Get("fgRole")
		if roleName == "" {
			return
		}
		servicename := env.Get("servicename")
		permissionName := env.Get("fgPermission")

		if env.Get("apitoken") == "" {
			res := T("token", "show").Run(env)
			c.Check(res, ResultOk)
			env.Set("apitoken", parseAPIToken(c, res.Stdout.String()))
		}
		authHeader := fmt.Sprintf("'Authorization: Bearer %s'", env.Get("apitoken"))

		if permissionName != "" {
			res := T("role", "permission", "remove", roleName, permissionName).Run(env)
			c.Check(res, ResultOk)
		}

		adminUser := env.Get("adminuser")
		if adminUser != "" {
			res := T("role", "dissociate", roleName, adminUser, env.Get("team")).Run(env)
			c.Check(res, ResultOk)
		}

		res := T("role", "remove", "-y", roleName).Run(env)
		c.Check(res, ResultOk)

		resetPayload := `{"enabled":false,"strict_actions":true,"operations":[]}`
		if servicename != "" {
			res := NewCommand("curl", "-sS", "-X", "PUT",
				fmt.Sprintf("%s/1.31/services/%s/manifest", targetAddr, servicename),
				"-H", authHeader,
				"-H", "'Content-Type: application/json'",
				"-d", fmt.Sprintf("'%s'", resetPayload)).Run(env)
			c.Check(res, ResultOk)
		}
	}

	return flow
}

func serviceFineGrainedPermissionsSharedActionsFlow() ExecFlow {
	flow := ExecFlow{
		requires: []string{"poolnames", "installedplatforms", "serviceimage", "team", "adminuser"},
	}

	flow.forward = func(c *check.C, env *Environment) {
		targetAddr := env.Get("targetaddr")
		suffix := time.Now().Format("150405")
		firstService := fmt.Sprintf("fg-shared-a-%s", suffix)
		secondService := fmt.Sprintf("fg-shared-b-%s", suffix)

		env.Set("fgSharedFirstService", firstService)
		env.Set("fgSharedSecondService", secondService)

		roleName := fmt.Sprintf("integration-fg-shared-%s", suffix)
		roleName = strings.ReplaceAll(roleName, ".", "-")
		env.Set("fgSharedRole", roleName)

		manifestPayload := readManifestPayload(c, path.Join("fixtures", "service", "manifest-fine-grained.json"))
		var manifest serviceManifestPayload
		err := json.Unmarshal(manifestPayload, &manifest)
		c.Assert(err, check.IsNil)
		c.Assert(len(manifest.Operations), check.Not(check.Equals), 0)

		var action string
		for _, op := range manifest.Operations {
			if strings.EqualFold(op.Method, "POST") && strings.Contains(op.Path, "sync") && op.Action != "" {
				action = op.Action
				break
			}
		}
		if action == "" {
			action = manifest.Operations[0].Action
		}
		c.Assert(action, check.Not(check.Equals), "")

		firstPermission := fmt.Sprintf("service-action.%s.%s", firstService, action)
		secondPermission := fmt.Sprintf("service-action.%s.%s", secondService, action)
		env.Set("fgSharedFirstPermission", firstPermission)
		env.Set("fgSharedSecondPermission", secondPermission)

		res := T("token", "show").Run(env)
		c.Assert(res, ResultOk)
		token := parseAPIToken(c, res.Stdout.String())
		env.Set("apitoken", token)
		authHeader := fmt.Sprintf("'Authorization: Bearer %s'", token)

		poolName := env.Get("pool")
		if poolName == "" {
			pools := env.All("poolnames")
			c.Assert(len(pools) > 0, check.Equals, true)
			poolName = pools[0]
		}
		env.Set("fgSharedFirstApp", createServiceFromManifestTemplate(c, env, firstService, poolName))
		env.Set("fgSharedSecondApp", createServiceFromManifestTemplate(c, env, secondService, poolName))

		for _, svcName := range []string{firstService, secondService} {
			manifestURL := fmt.Sprintf("%s/1.31/services/%s/manifest", targetAddr, svcName)
			res = NewCommand("curl", "-sS", "-X", "PUT", manifestURL,
				"-H", authHeader,
				"-H", "'Content-Type: application/json'",
				"-d", fmt.Sprintf("'%s'", string(manifestPayload))).Run(env)
			c.Assert(res, ResultOk)
		}

		res = T("permission", "list").Run(env)
		c.Assert(res, ResultOk)
		c.Assert(strings.Contains(res.Stdout.String(), firstPermission), check.Equals, true)
		c.Assert(strings.Contains(res.Stdout.String(), secondPermission), check.Equals, true)

		res = T("role", "add", roleName, "team").Run(env)
		c.Assert(res, ResultOk)

		res = T("role", "assign", roleName, env.Get("adminuser"), env.Get("team")).Run(env)
		c.Assert(res, ResultOk)

		res = T("role", "permission", "add", roleName, firstPermission).Run(env)
		c.Assert(res, ResultOk)
		res = T("role", "permission", "add", roleName, secondPermission).Run(env)
		c.Assert(res, ResultOk)

		res = NewCommand("curl", "-sS", "-X", "GET", fmt.Sprintf("%s/1.0/roles/%s", targetAddr, roleName),
			"-H", authHeader).Run(env)
		c.Assert(res, ResultOk)
		var roleInfo struct {
			DynamicSchemeNames []string `json:"dynamic_scheme_names"`
		}
		err = json.Unmarshal([]byte(res.Stdout.String()), &roleInfo)
		c.Assert(err, check.IsNil)
		sort.Strings(roleInfo.DynamicSchemeNames)
		expectedPermissions := []string{firstPermission, secondPermission}
		sort.Strings(expectedPermissions)
		c.Assert(roleInfo.DynamicSchemeNames, check.DeepEquals, expectedPermissions)
	}

	flow.backward = func(c *check.C, env *Environment) {
		roleName := env.Get("fgSharedRole")
		perms := []string{
			env.Get("fgSharedFirstPermission"),
			env.Get("fgSharedSecondPermission"),
		}

		if env.Get("apitoken") == "" {
			res := T("token", "show").Run(env)
			c.Check(res, ResultOk)
			env.Set("apitoken", parseAPIToken(c, res.Stdout.String()))
		}

		for _, permissionName := range perms {
			if permissionName == "" {
				continue
			}
			res := T("role", "permission", "remove", roleName, permissionName).Run(env)
			c.Check(res, ResultOk)
		}

		adminUser := env.Get("adminuser")
		if roleName != "" && adminUser != "" {
			res := T("role", "dissociate", roleName, adminUser, env.Get("team")).Run(env)
			c.Check(res, ResultOk)
		}

		if roleName != "" {
			res := T("role", "remove", "-y", roleName).Run(env)
			c.Check(res, ResultOk)
		}

		if env.Get("fgSharedFirstService") != "" {
			res := T("service", "destroy", env.Get("fgSharedFirstService"), "-y").Run(env)
			c.Check(res, ResultOk)
		}
		if env.Get("fgSharedSecondService") != "" {
			res := T("service", "destroy", env.Get("fgSharedSecondService"), "-y").Run(env)
			c.Check(res, ResultOk)
		}
		if env.Get("fgSharedFirstApp") != "" {
			res := T("app", "remove", "-y", "-a", env.Get("fgSharedFirstApp")).Run(env)
			c.Check(res, ResultOk)
		}
		if env.Get("fgSharedSecondApp") != "" {
			res := T("app", "remove", "-y", "-a", env.Get("fgSharedSecondApp")).Run(env)
			c.Check(res, ResultOk)
		}
	}

	return flow
}

func createServiceFromManifestTemplate(c *check.C, env *Environment, serviceName, pool string) string {
	appName := "integration-fg-" + serviceName

	res := T("app", "create", appName, "{{.installedplatforms}}", "-t", "{{.team}}", "-o", pool).Run(env)
	c.Assert(res, ResultOk)
	res = T("app", "info", "-a", appName).Run(env)
	c.Assert(res, ResultOk)
	res = T("env", "set", "-a", appName, "EVI_ENVIRONS='{\"INTEGRATION_ENV\":\"TRUE\"}'").Run(env)
	c.Assert(res, ResultOk)
	res = T("app", "deploy", "-a", appName, "-i", "{{.serviceimage}}").Run(env)
	c.Assert(res, ResultOk)

	appInfo := new(appTypes.AppInfo)
	ok := retry(5*time.Minute, func() (ready bool) {
		appInfo, ready = checkAppExternallyAddressable(c, appName, env)
		return ready
	})
	c.Assert(ok, check.Equals, true, check.Commentf("app not ready after 5 minutes: %v", res))
	c.Assert(appInfo.InternalAddresses, check.HasLen, 1)

	externalAddress := appInfo.Routers[0].Addresses[0]
	cmd := NewCommand("curl", "-sS", "-o", "/dev/null", "--write-out", "%{http_code}", "http://"+externalAddress)
	ok = retry(15*time.Minute, func() bool {
		res = cmd.Run(env)
		code, _ := strconv.Atoi(res.Stdout.String())
		return code >= 200 && code < 500
	})
	c.Assert(ok, check.Equals, true, check.Commentf("invalid result: %v", res))

	dir, err := os.MkdirTemp("", "service")
	c.Assert(err, check.IsNil)
	currDir, err := os.Getwd()
	c.Assert(err, check.IsNil)
	err = os.Chdir(dir)
	c.Assert(err, check.IsNil)
	defer os.Chdir(currDir)

	res = T("service", "template").Run(env)
	c.Assert(res, ResultOk)
	getServiceAddress := func() string {
		if env.Get("local") == "true" {
			fmt.Println("DEBUG: Using external address for service in local mode")
			return appInfo.Routers[0].Addresses[0]
		}
		fmt.Println("DEBUG: Using internal address for service")
		internalAddress := appInfo.InternalAddresses[0]
		return fmt.Sprintf("%s:%d", internalAddress.Domain, internalAddress.Port)
	}
	serviceAddress := getServiceAddress()
	replaces := map[string]string{
		"team_responsible_to_provide_service": "integration-team",
		"production-endpoint.com":             fmt.Sprintf("http://%s", serviceAddress),
		"servicename":                         serviceName,
	}
	for k, v := range replaces {
		res = NewCommand("sed", "-i", "-e", "'s~"+k+"~"+v+"~'", "manifest.yaml").Run(env)
		c.Assert(res, ResultOk)
	}

	res = T("service", "list").Run(env)
	c.Assert(res, ResultOk)

	res = T("service", "create", "manifest.yaml").Run(env)
	c.Assert(res, ResultOk)

	ok = retry(time.Minute, func() bool {
		res = T("service", "info", serviceName).Run(env)
		return res.ExitCode == 0
	})
	c.Assert(ok, check.Equals, true, check.Commentf("invalid result: %v", res))

	return appName
}
