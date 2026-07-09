// Copyright 2026 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package integration

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"strings"

	check "gopkg.in/check.v1"
)

type serviceManifestPayload struct {
	Enabled       bool                       `json:"enabled"`
	StrictActions bool                       `json:"strict_actions"`
	Operations    []serviceManifestOperation `json:"operations"`
}

type serviceManifestOperation struct {
	Method string `json:"method"`
	Path   string `json:"path"`
	Action string `json:"action"`
}

func serviceGeneralFlow() ExecFlow {
	flow := ExecFlow{
		requires: []string{"servicename"},
	}

	flow.forward = func(c *check.C, env *Environment) {
		baseManifestPath := readManifestPayload(c, path.Join("fixtures", "service", "manifest-general.json"))
		var expectedManifest serviceManifestPayload
		err := json.Unmarshal(baseManifestPath, &expectedManifest)
		c.Assert(err, check.IsNil)

		res := T("service", "list", "-j").Run(env)
		c.Assert(res, ResultOk)
		c.Assert(res, ResultMatches, Expected{Stdout: `(?s).*` + env.Get("servicename") + `.*`})

		res = T("service", "info", "{{.servicename}}").Run(env)
		c.Assert(res, ResultOk)

		res = T("token", "show").Run(env)
		c.Assert(res, ResultOk)
		token := parseAPIToken(c, res.Stdout.String())
		env.Set("apitoken", token)

		manifestURL := "{{.targetaddr}}/1.31/services/{{.servicename}}/manifest"

		res = NewCommand("curl", "-sS", "-X", "GET", manifestURL, "-H", "'Authorization: Bearer {{.apitoken}}'").Run(env)
		c.Assert(res, ResultOk)

		var before any
		err = json.Unmarshal([]byte(res.Stdout.String()), &before)
		c.Assert(err, check.IsNil)

		res = NewCommand("curl", "-sS", "-X", "PUT", manifestURL,
			"-H", "'Authorization: Bearer {{.apitoken}}'",
			"-H", "'Content-Type: application/json'",
			"-d", fmt.Sprintf("'%s'", string(baseManifestPath))).Run(env)
		c.Assert(res, ResultOk)

		res = NewCommand("curl", "-sS", "-X", "GET", manifestURL, "-H", "'Authorization: Bearer {{.apitoken}}'").Run(env)
		c.Assert(res, ResultOk)

		var after serviceManifestPayload
		err = json.Unmarshal([]byte(res.Stdout.String()), &after)
		c.Assert(err, check.IsNil)
		c.Assert(after.Enabled, check.Equals, expectedManifest.Enabled)
		c.Assert(after.StrictActions, check.Equals, expectedManifest.StrictActions)
		c.Assert(after.Operations, check.HasLen, len(expectedManifest.Operations))
	}

	flow.backward = func(c *check.C, env *Environment) {
		token := env.Get("apitoken")
		if token == "" {
			res := T("token", "show").Run(env)
			c.Check(res, ResultOk)
			token = parseAPIToken(c, res.Stdout.String())
			env.Set("apitoken", token)
		}

		res := NewCommand("curl", "-sS", "-X", "PUT", "{{.targetaddr}}/1.31/services/{{.servicename}}/manifest",
			"-H", "'Authorization: Bearer {{.apitoken}}'",
			"-H", "'Content-Type: application/json'",
			"-d", `{"enabled":false,"strict_actions":true,"operations":[]}`).Run(env)
		c.Check(res, ResultOk)
	}

	return flow
}

func readManifestPayload(c *check.C, relativePath string) []byte {
	cwd, err := os.Getwd()
	c.Assert(err, check.IsNil)
	manifestPath := path.Join(cwd, relativePath)
	payload, err := os.ReadFile(manifestPath)
	c.Assert(err, check.IsNil)
	return payload
}

func parseAPIToken(c *check.C, rawTokenOutput string) string {
	trimmed := strings.TrimSpace(rawTokenOutput)
	c.Assert(trimmed, check.Not(check.Equals), "")

	if strings.HasPrefix(trimmed, "{") {
		var tokenResponse struct {
			Token string `json:"token"`
		}
		if err := json.Unmarshal([]byte(trimmed), &tokenResponse); err == nil {
			trimmed = tokenResponse.Token
		}
	}

	for line := range strings.SplitSeq(trimmed, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(strings.ToLower(line), "api key:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				trimmed = strings.TrimSpace(parts[1])
				c.Assert(trimmed, check.Not(check.Equals), "")
				return trimmed
			}
		}
	}

	if strings.Contains(trimmed, "\n") {
		parts := strings.Fields(trimmed)
		trimmed = parts[len(parts)-1]
	}
	if strings.Contains(trimmed, " ") {
		parts := strings.Fields(trimmed)
		trimmed = parts[len(parts)-1]
	}

	trimmed = strings.TrimPrefix(trimmed, "token:")
	trimmed = strings.TrimPrefix(trimmed, "Token:")
	trimmed = strings.TrimSpace(trimmed)

	c.Assert(trimmed, check.Not(check.Equals), "")
	return trimmed
}
