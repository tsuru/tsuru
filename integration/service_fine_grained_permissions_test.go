// Copyright 2026 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package integration

import (
	"encoding/json"
	"fmt"
	"net/url"
	"path"
	"strings"
	"time"

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

		grantPermissionURL := fmt.Sprintf("%s/1.31/roles/%s/dynamic-permissions", targetAddr, roleName)
		res = NewCommand("curl", "-sS", "-X", "POST", grantPermissionURL,
			"-H", authHeader,
			"-H", "'Content-Type: application/x-www-form-urlencoded'",
			"-d", fmt.Sprintf("'permission=%s'", permissionName)).Run(env)
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
			res := NewCommand("curl", "-sS", "-X", "DELETE",
				fmt.Sprintf("%s/1.31/roles/%s/dynamic-permissions/%s", targetAddr, roleName, permissionName),
				"-H", authHeader).Run(env)
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
