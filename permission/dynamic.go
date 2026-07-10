// Copyright 2026 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package permission

import (
	"sort"
	"strings"
	"sync"

	permTypes "github.com/tsuru/tsuru/types/permission"
)

const (
	DynamicPermissionPrefix = "service-action."
)

type dynamicRegistry struct {
	mu                 sync.RWMutex
	schemeByActionName map[string]*permTypes.PermissionScheme
}

var DynamicPermissionRegistry = &dynamicRegistry{
	schemeByActionName: make(map[string]*permTypes.PermissionScheme),
}

// RegisterDynamic registers (idempotently) an action scheme with the given contexts.
// Safe for concurrent use.
func RegisterDynamic(name string, ctxs []permTypes.ContextType) error {
	return DynamicPermissionRegistry.register(name, ctxs)
}

// UnregisterDynamic removes an action scheme (used on explicit manifest changes /
// service deletion). Idempotent.
func UnregisterDynamic(name string) error {
	DynamicPermissionRegistry.unregister(name)
	return nil
}

// LookupDynamic returns the scheme and whether it exists.
func LookupDynamic(name string) (*permTypes.PermissionScheme, bool) {
	return DynamicPermissionRegistry.lookup(name)
}

// ListDynamic returns all dynamic schemes.
func ListDynamic() permTypes.PermissionSchemeList {
	return DynamicPermissionRegistry.list()
}

func IsDynamicPermissionName(name string) bool {
	return strings.HasPrefix(name, DynamicPermissionPrefix)
}

// CheckDynamic returns true if any granted name is an ancestor-or-equal of the
// requested name and the granted permission context matches the request contexts.
func CheckDynamic(granted []permTypes.Permission, requested string, contexts ...permTypes.PermissionContext) bool {
	if requested == "" {
		return false
	}
	if _, ok := LookupDynamic(requested); !ok {
		return false
	}
	for _, perm := range granted {
		if perm.Scheme == nil {
			continue
		}
		if !isDynamicAncestorOrSelf(perm.Scheme.FullName(), requested) {
			continue
		}
		if perm.Context.CtxType == permTypes.CtxGlobal {
			return true
		}
		for _, ctx := range contexts {
			if ctx.CtxType == perm.Context.CtxType && ctx.Value == perm.Context.Value {
				return true
			}
		}
	}
	return false
}

func (r *dynamicRegistry) register(name string, ctxs []permTypes.ContextType) error {
	if name == "" {
		return permTypes.ErrInvalidPermissionName
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	var parent *permTypes.PermissionScheme
	for _, fullName := range schemeLineage(name) {
		scheme, ok := r.schemeByActionName[fullName]
		if !ok {
			scheme = &permTypes.PermissionScheme{
				Name:     schemeSegment(fullName),
				Parent:   parent,
				Contexts: cloneContextTypes(ctxs),
			}
			r.schemeByActionName[fullName] = scheme
		}
		parent = scheme
	}
	return nil
}

func (r *dynamicRegistry) unregister(name string) {
	if name == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.schemeByActionName, name)
}

func (r *dynamicRegistry) lookup(name string) (*permTypes.PermissionScheme, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	scheme, ok := r.schemeByActionName[name]
	return scheme, ok
}

func (r *dynamicRegistry) list() permTypes.PermissionSchemeList {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.schemeByActionName))
	for name := range r.schemeByActionName {
		names = append(names, name)
	}
	sort.Strings(names)
	result := make(permTypes.PermissionSchemeList, 0, len(names))
	for _, name := range names {
		result = append(result, r.schemeByActionName[name])
	}
	return result
}

func schemeLineage(name string) []string {
	parts := strings.Split(name, ".")
	result := make([]string, 0, len(parts))
	for i := range parts {
		result = append(result, strings.Join(parts[:i+1], "."))
	}
	return result
}

func schemeSegment(name string) string {
	lastDot := strings.LastIndex(name, ".")
	if lastDot == -1 {
		return name
	}
	return name[lastDot+1:]
}

func isDynamicAncestorOrSelf(granted string, requested string) bool {
	if granted == "" || requested == "" {
		return false
	}
	if granted == requested {
		return true
	}
	if !strings.HasPrefix(requested, granted) {
		return false
	}
	return len(requested) > len(granted) && requested[len(granted)] == '.'
}

func cloneContextTypes(ctxs []permTypes.ContextType) []permTypes.ContextType {
	if ctxs == nil {
		return nil
	}
	cloned := make([]permTypes.ContextType, len(ctxs))
	copy(cloned, ctxs)
	return cloned
}
