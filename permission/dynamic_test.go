// Copyright 2026 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package permission

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	permTypes "github.com/tsuru/tsuru/types/permission"
)

func TestNewDynamicBuildsLineage(t *testing.T) {
	scheme, ok := NewDynamic("service-action.acl.rules.sync")
	require.True(t, ok)
	expectedContexts := []permTypes.ContextType{
		permTypes.CtxGlobal,
		permTypes.CtxServiceInstance,
		permTypes.CtxService,
		permTypes.CtxTeam,
	}
	for _, fullName := range []string{
		"service-action.acl.rules.sync",
		"service-action.acl.rules",
		"service-action.acl",
	} {
		require.NotNil(t, scheme, "expected scheme for %q", fullName)
		assert.Equal(t, fullName, scheme.FullName())
		assert.Equal(t, expectedContexts, scheme.AllowedContexts())
		scheme = scheme.Parent
	}
	assert.Nil(t, scheme, "expected lineage to end at the root")
}

func TestNewDynamicRejectsNonDynamicNames(t *testing.T) {
	for _, name := range []string{"", "app.deploy", "service-action"} {
		scheme, ok := NewDynamic(name)
		assert.False(t, ok, "expected %q to be rejected", name)
		assert.Nil(t, scheme)
	}
}

func TestCheckDynamic(t *testing.T) {
	tests := []struct {
		name     string
		granted  []permTypes.Permission
		request  string
		contexts []permTypes.PermissionContext
		want     bool
	}{
		{
			name: "exact match",
			granted: []permTypes.Permission{{
				Scheme:  mustNewDynamic(t, "service-action.acl.rules.sync"),
				Context: permTypes.PermissionContext{CtxType: permTypes.CtxTeam, Value: "team-1"},
			}},
			request:  "service-action.acl.rules.sync",
			contexts: []permTypes.PermissionContext{{CtxType: permTypes.CtxTeam, Value: "team-1"}},
			want:     true,
		},
		{
			name: "ancestor match",
			granted: []permTypes.Permission{{
				Scheme:  mustNewDynamic(t, "service-action.acl.rules"),
				Context: permTypes.PermissionContext{CtxType: permTypes.CtxServiceInstance, Value: "instance-1"},
			}},
			request:  "service-action.acl.rules.sync",
			contexts: []permTypes.PermissionContext{{CtxType: permTypes.CtxServiceInstance, Value: "instance-1"}},
			want:     true,
		},
		{
			name: "global context",
			granted: []permTypes.Permission{{
				Scheme:  mustNewDynamic(t, "service-action.acl.rules"),
				Context: permTypes.PermissionContext{CtxType: permTypes.CtxGlobal},
			}},
			request: "service-action.acl.rules.read",
			want:    true,
		},
		{
			name: "reject string prefix false positive",
			granted: []permTypes.Permission{{
				Scheme:  mustNewDynamic(t, "service-action.acl.rules"),
				Context: permTypes.PermissionContext{CtxType: permTypes.CtxTeam, Value: "team-1"},
			}},
			request:  "service-action.acl.rulex.sync",
			contexts: []permTypes.PermissionContext{{CtxType: permTypes.CtxTeam, Value: "team-1"}},
			want:     false,
		},
		{
			name: "reject context type mismatch",
			granted: []permTypes.Permission{{
				Scheme:  mustNewDynamic(t, "service-action.acl.rules.sync"),
				Context: permTypes.PermissionContext{CtxType: permTypes.CtxService, Value: "svc-1"},
			}},
			request:  "service-action.acl.rules.sync",
			contexts: []permTypes.PermissionContext{{CtxType: permTypes.CtxTeam, Value: "team-1"}},
			want:     false,
		},
		{
			name: "reject context value mismatch",
			granted: []permTypes.Permission{{
				Scheme:  mustNewDynamic(t, "service-action.acl.rules.sync"),
				Context: permTypes.PermissionContext{CtxType: permTypes.CtxTeam, Value: "team-2"},
			}},
			request:  "service-action.acl.rules.sync",
			contexts: []permTypes.PermissionContext{{CtxType: permTypes.CtxTeam, Value: "team-1"}},
			want:     false,
		},
		{
			name: "reject non-dynamic requested name",
			granted: []permTypes.Permission{{
				Scheme:  mustNewDynamic(t, "service-action.acl.rules"),
				Context: permTypes.PermissionContext{CtxType: permTypes.CtxGlobal},
			}},
			request: "app.deploy",
			want:    false,
		},
		{
			name: "ignore nil scheme",
			granted: []permTypes.Permission{{
				Scheme:  nil,
				Context: permTypes.PermissionContext{CtxType: permTypes.CtxGlobal},
			}},
			request: "service-action.acl.rules.read",
			want:    false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CheckDynamic(tt.granted, tt.request, tt.contexts...)
			assert.Equal(t, tt.want, got)
		})
	}
}

func mustNewDynamic(t *testing.T, name string) *permTypes.PermissionScheme {
	t.Helper()
	scheme, ok := NewDynamic(name)
	require.True(t, ok, "expected %q to build a dynamic scheme", name)
	return scheme
}
