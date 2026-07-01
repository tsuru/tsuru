// Copyright 2026 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package permission

import (
	"fmt"
	"sort"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	permTypes "github.com/tsuru/tsuru/types/permission"
)

func resetDynamicRegistryForTest() {
	DynamicPermissionRegistry = &dynamicRegistry{
		schemeByActionName: make(map[string]*permTypes.PermissionScheme),
	}
}

func TestRegisterDynamicCreatesLineage(t *testing.T) {
	resetDynamicRegistryForTest()
	err := RegisterDynamic("service-action.acl.rules.sync", []permTypes.ContextType{permTypes.CtxServiceInstance, permTypes.CtxTeam})
	require.NoError(t, err)
	tests := []struct {
		name   string
		parent string
	}{
		{name: "service-action", parent: ""},
		{name: "service-action.acl", parent: "service-action"},
		{name: "service-action.acl.rules", parent: "service-action.acl"},
		{name: "service-action.acl.rules.sync", parent: "service-action.acl.rules"},
	}
	for _, tt := range tests {
		scheme, ok := LookupDynamic(tt.name)
		require.True(t, ok, "expected %q to be registered", tt.name)
		assert.Equal(t, tt.name, scheme.FullName())
		if tt.parent == "" {
			assert.Nil(t, scheme.Parent, "expected %q parent to be nil", tt.name)
		} else {
			require.NotNil(t, scheme.Parent, "expected %q parent to be set", tt.name)
			assert.Equal(t, tt.parent, scheme.Parent.FullName())
		}
	}
	leaf, _ := LookupDynamic("service-action.acl.rules.sync")
	expectedContexts := []permTypes.ContextType{permTypes.CtxGlobal, permTypes.CtxServiceInstance, permTypes.CtxTeam}
	assert.Equal(t, expectedContexts, leaf.AllowedContexts())
	ancestor, _ := LookupDynamic("service-action.acl.rules")
	assert.Equal(t, expectedContexts, ancestor.AllowedContexts())
}

func TestRegisterDynamicIsIdempotent(t *testing.T) {
	resetDynamicRegistryForTest()
	name := "service-action.acl.rules.sync"
	err := RegisterDynamic(name, []permTypes.ContextType{permTypes.CtxTeam})
	require.NoError(t, err)
	err = RegisterDynamic(name, []permTypes.ContextType{permTypes.CtxTeam})
	require.NoError(t, err)
	list := ListDynamic()
	assert.Len(t, list, 4)
	leaf, ok := LookupDynamic(name)
	require.True(t, ok, "expected %q lookup to succeed", name)
	expectedContexts := []permTypes.ContextType{permTypes.CtxGlobal, permTypes.CtxTeam}
	assert.Equal(t, expectedContexts, leaf.AllowedContexts())
}

func TestUnregisterDynamicIsIdempotent(t *testing.T) {
	resetDynamicRegistryForTest()
	name := "service-action.acl.rules.sync"
	require.NoError(t, RegisterDynamic(name, []permTypes.ContextType{permTypes.CtxTeam}))
	require.NoError(t, UnregisterDynamic(name))
	if _, ok := LookupDynamic(name); ok {
		t.Fatalf("expected %q to be removed", name)
	}
	_, ok := LookupDynamic("service-action.acl.rules")
	assert.True(t, ok, "expected ancestor scheme to remain available")
	require.NoError(t, UnregisterDynamic(name))
}

func TestListDynamicIsSorted(t *testing.T) {
	resetDynamicRegistryForTest()
	for _, name := range []string{
		"service-action.beta.rules.sync",
		"service-action.acl.rules.read",
		"service-action.acl.rules.sync",
	} {
		require.NoError(t, RegisterDynamic(name, []permTypes.ContextType{permTypes.CtxTeam}))
	}
	got := make([]string, 0, len(ListDynamic()))
	for _, scheme := range ListDynamic() {
		got = append(got, scheme.FullName())
	}
	want := append([]string(nil), got...)
	sort.Strings(want)
	assert.Equal(t, fmt.Sprint(want), fmt.Sprint(got))
}

func TestCheckDynamic(t *testing.T) {
	resetDynamicRegistryForTest()
	for _, name := range []string{
		"service-action.acl.rules.sync",
		"service-action.acl.rules.read",
		"service-action.acl.rulex.sync",
	} {
		require.NoError(t, RegisterDynamic(name, []permTypes.ContextType{permTypes.CtxServiceInstance, permTypes.CtxTeam}))
	}
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
				Scheme:  mustLookupDynamic(t, "service-action.acl.rules.sync"),
				Context: permTypes.PermissionContext{CtxType: permTypes.CtxTeam, Value: "team-1"},
			}},
			request:  "service-action.acl.rules.sync",
			contexts: []permTypes.PermissionContext{{CtxType: permTypes.CtxTeam, Value: "team-1"}},
			want:     true,
		},
		{
			name: "ancestor match",
			granted: []permTypes.Permission{{
				Scheme:  mustLookupDynamic(t, "service-action.acl.rules"),
				Context: permTypes.PermissionContext{CtxType: permTypes.CtxServiceInstance, Value: "instance-1"},
			}},
			request:  "service-action.acl.rules.sync",
			contexts: []permTypes.PermissionContext{{CtxType: permTypes.CtxServiceInstance, Value: "instance-1"}},
			want:     true,
		},
		{
			name: "global context",
			granted: []permTypes.Permission{{
				Scheme:  mustLookupDynamic(t, "service-action.acl.rules"),
				Context: permTypes.PermissionContext{CtxType: permTypes.CtxGlobal},
			}},
			request: "service-action.acl.rules.read",
			want:    true,
		},
		{
			name: "reject string prefix false positive",
			granted: []permTypes.Permission{{
				Scheme:  mustLookupDynamic(t, "service-action.acl.rules"),
				Context: permTypes.PermissionContext{CtxType: permTypes.CtxTeam, Value: "team-1"},
			}},
			request:  "service-action.acl.rulex.sync",
			contexts: []permTypes.PermissionContext{{CtxType: permTypes.CtxTeam, Value: "team-1"}},
			want:     false,
		},
		{
			name: "reject context type mismatch",
			granted: []permTypes.Permission{{
				Scheme:  mustLookupDynamic(t, "service-action.acl.rules.sync"),
				Context: permTypes.PermissionContext{CtxType: permTypes.CtxService, Value: "svc-1"},
			}},
			request:  "service-action.acl.rules.sync",
			contexts: []permTypes.PermissionContext{{CtxType: permTypes.CtxTeam, Value: "team-1"}},
			want:     false,
		},
		{
			name: "reject context value mismatch",
			granted: []permTypes.Permission{{
				Scheme:  mustLookupDynamic(t, "service-action.acl.rules.sync"),
				Context: permTypes.PermissionContext{CtxType: permTypes.CtxTeam, Value: "team-2"},
			}},
			request:  "service-action.acl.rules.sync",
			contexts: []permTypes.PermissionContext{{CtxType: permTypes.CtxTeam, Value: "team-1"}},
			want:     false,
		},
		{
			name: "reject unknown requested scheme",
			granted: []permTypes.Permission{{
				Scheme:  mustLookupDynamic(t, "service-action.acl.rules"),
				Context: permTypes.PermissionContext{CtxType: permTypes.CtxGlobal},
			}},
			request: "service-action.acl.rules.delete",
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

func TestDynamicRegistryConcurrentAccess(t *testing.T) {
	resetDynamicRegistryForTest()
	const workers = 24
	const perWorker = 20
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < perWorker; j++ {
				name := fmt.Sprintf("service-action.svc-%d.action-%d.read", i, j)
				assert.NoError(t, RegisterDynamic(name, []permTypes.ContextType{permTypes.CtxTeam}), "register failed for %q", name)
				scheme, ok := LookupDynamic(name)
				if !assert.True(t, ok, "lookup failed for %q", name) {
					return
				}
				granted := []permTypes.Permission{{
					Scheme:  scheme,
					Context: permTypes.PermissionContext{CtxType: permTypes.CtxTeam, Value: "team-1"},
				}}
				assert.True(t, CheckDynamic(granted, name, permTypes.PermissionContext{CtxType: permTypes.CtxTeam, Value: "team-1"}), "check failed for %q", name)
				_ = ListDynamic()
			}
		}(i)
	}
	wg.Wait()
	if t.Failed() {
		return
	}
	want := 1 + workers + (workers * perWorker * 2)
	assert.Len(t, ListDynamic(), want)
}

func mustLookupDynamic(t *testing.T, name string) *permTypes.PermissionScheme {
	t.Helper()
	scheme, ok := LookupDynamic(name)
	require.True(t, ok, "expected %q lookup to succeed", name)
	return scheme
}
