// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package permission

import (
	"strings"
)

type recorder struct {
	permissionScheme
	children []*recorder
}

type recorderCB func(r *recorder)

func (r *recorder) add(perms ...string) *recorder {
	for _, name := range perms {
		r.children = append(r.children, &recorder{permissionScheme: permissionScheme{name: name}})
	}
	return r
}

func (r *recorder) addWithContextCB(perm string, contextTypes []contextType, cb recorderCB) *recorder {
	newParent := &recorder{permissionScheme: permissionScheme{name: perm, contexts: contextTypes}}
	r.children = append(r.children, newParent)
	if cb != nil {
		cb(newParent)
	}
	return r
}

func (r *recorder) addWithContext(perm string, contextTypes []contextType) *recorder {
	return r.addWithContextCB(perm, contextTypes, nil)
}

func (r *recorder) addCB(perm string, cb recorderCB) *recorder {
	return r.addWithContextCB(perm, nil, cb)
}

func (r *recorder) Permissions() PermissionSchemeList {
	var ret []*permissionScheme
	stack := []*recorder{r}
	for len(stack) > 0 {
		last := len(stack) - 1
		el := stack[last]
		stack = stack[:last]
		ret = append(ret, &el.permissionScheme)
		for i := len(el.children) - 1; i >= 0; i-- {
			child := el.children[i]
			child.parent = &el.permissionScheme
			stack = append(stack, child)
		}
	}
	return ret
}

func (r *recorder) get(name string) *permissionScheme {
	if name == "" {
		return &r.permissionScheme
	}
	parts := strings.Split(name, ".")
	children := r.children
	var parent *permissionScheme
	for len(children) > 0 && len(parts) > 0 {
		var currentElement *permissionScheme
		for _, child := range children {
			if child.name == parts[0] {
				child.permissionScheme.parent = parent
				currentElement = &child.permissionScheme
				parts = parts[1:]
				children = child.children
				break
			}
		}
		parent = currentElement
		if parent == nil {
			break
		}
	}
	if parent == nil {
		panic("unregistered permission: " + name)
	}
	return parent
}
