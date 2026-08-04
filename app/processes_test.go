// Copyright 2026 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"testing"

	"github.com/stretchr/testify/require"
	appTypes "github.com/tsuru/tsuru/types/app"
	provTypes "github.com/tsuru/tsuru/types/provision"
)

func TestMergeProcesses(t *testing.T) {
	healthcheck := &provTypes.TsuruYamlHealthcheck{Path: "/health"}
	startupcheck := &provTypes.TsuruYamlStartupcheck{Command: []string{"check-startup"}}
	providerProcesses := []appTypes.Process{
		{Name: "worker", Startupcheck: startupcheck},
		{Name: "web", Healthcheck: healthcheck},
	}
	appProcesses := []appTypes.Process{
		{Name: "scheduler", Plan: "small"},
		{Name: "web", Plan: "large", Metadata: appTypes.Metadata{
			Labels: []appTypes.MetadataItem{{Name: "team", Value: "core"}},
		}},
	}

	result := mergeProcesses(providerProcesses, appProcesses)

	require.Equal(t, []appTypes.Process{
		{Name: "scheduler", Plan: "small"},
		{
			Name:        "web",
			Plan:        "large",
			Metadata:    appTypes.Metadata{Labels: []appTypes.MetadataItem{{Name: "team", Value: "core"}}},
			Healthcheck: healthcheck,
		},
		{Name: "worker", Startupcheck: startupcheck},
	}, result)
}

func TestMergeProcessesWithProviderErrorResult(t *testing.T) {
	appProcesses := []appTypes.Process{{Name: "web", Plan: "large"}}

	result := mergeProcesses(nil, appProcesses)

	require.Equal(t, appProcesses, result)
}
