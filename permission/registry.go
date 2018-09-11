// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package permission

import (
	"strings"

	"github.com/pkg/errors"
	permTypes "github.com/tsuru/tsuru/types/permission"
)

type registry struct {
	PermissionScheme
	children []*registry
}

func (r *registry) add(names ...string) *registry {
	for _, name := range names {
		r.addWithCtx(name, nil)
	}
	return r
}

func (r *registry) addWithCtx(name string, contextTypes []permTypes.ContextType) *registry {
	parts := strings.Split(name, ".")
	parent := r
	for i, part := range parts {
		subR := parent.getSubRegistry(part)
		if subR == nil {
			subR = &registry{PermissionScheme: PermissionScheme{name: part}}
			parent.children = append(parent.children, subR)
		}
		if i == len(parts)-1 {
			subR.PermissionScheme.contexts = contextTypes
		}
		parent = subR
	}
	return r
}

func (r *registry) getSubRegistry(name string) *registry {
	if name == "" {
		return r
	}
	parts := strings.Split(name, ".")
	children := r.children
	parent := r
	for len(parts) > 0 {
		var currentElement *registry
		for _, child := range children {
			if child.name == parts[0] {
				if parent != nil {
					child.PermissionScheme.parent = &parent.PermissionScheme
				}
				currentElement = child
				parts = parts[1:]
				children = child.children
				break
			}
		}
		parent = currentElement
		if parent == nil {
			return nil
		}
	}
	return parent
}

func (r *registry) PermissionsWithContextType(ctxType permTypes.ContextType) PermissionSchemeList {
	perms := r.Permissions()
	var ret []*PermissionScheme
	for _, p := range perms {
		for _, ctx := range p.AllowedContexts() {
			if ctx == ctxType {
				ret = append(ret, p)
				break
			}
		}
	}
	return ret
}

func (r *registry) Permissions() PermissionSchemeList {
	var ret []*PermissionScheme
	stack := []*registry{r}
	for len(stack) > 0 {
		last := len(stack) - 1
		el := stack[last]
		stack = stack[:last]
		ret = append(ret, &el.PermissionScheme)
		for i := len(el.children) - 1; i >= 0; i-- {
			child := el.children[i]
			child.parent = &el.PermissionScheme
			stack = append(stack, child)
		}
	}
	return ret
}

func SafeGet(name string) (*PermissionScheme, error) {
	subR := PermissionRegistry.getSubRegistry(name)
	if subR == nil {
		return nil, errors.New("unregistered permission")
	}
	return &subR.PermissionScheme, nil
}

func (r *registry) get(name string) *PermissionScheme {
	subR := r.getSubRegistry(name)
	if subR == nil {
		panic("unregistered permission: " + name)
	}
	return &subR.PermissionScheme
}
