// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package permission

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
)

var ErrUnauthorized = errors.New("You don't have permission to do this action")

type permissionScheme struct {
	name     string
	parent   *permissionScheme
	contexts []contextType
}

type PermissionSchemeList []*permissionScheme

type Context struct {
	CtxType contextType
	Value   interface{}
}

type contextType string

var (
	CtxGlobal          = contextType("global")
	CtxApp             = contextType("app")
	CtxTeam            = contextType("team")
	CtxPool            = contextType("pool")
	CtxIaaS            = contextType("iaas")
	CtxServiceInstance = contextType("service-instance")

	allTypes = []contextType{
		CtxGlobal, CtxApp, CtxTeam, CtxPool, CtxIaaS, CtxServiceInstance,
	}
)

func parseContext(ctx string) (contextType, error) {
	for _, t := range allTypes {
		if string(t) == ctx {
			return t, nil
		}
	}
	return "", fmt.Errorf("invalid context type %q", ctx)
}

func (l PermissionSchemeList) Len() int           { return len(l) }
func (l PermissionSchemeList) Swap(i, j int)      { l[i], l[j] = l[j], l[i] }
func (l PermissionSchemeList) Less(i, j int) bool { return l[i].FullName() < l[j].FullName() }

func (s *permissionScheme) nameParts() []string {
	parent := s
	var parts []string
	for parent != nil {
		if parent.name != "" {
			parts = append(parts, parent.name)
		}
		parent = parent.parent
	}
	return parts
}

func (s *permissionScheme) isParent(other *permissionScheme) bool {
	root := other
	myPointer := reflect.ValueOf(s).Pointer()
	for root != nil {
		if reflect.ValueOf(root).Pointer() == myPointer {
			return true
		}
		root = root.parent
	}
	return false
}

func (s *permissionScheme) FullName() string {
	parts := s.nameParts()
	var str string
	for i := len(parts) - 1; i >= 0; i-- {
		str += parts[i]
		if i != 0 {
			str += "."
		}
	}
	return str
}

func (s *permissionScheme) Identifier() string {
	parts := s.nameParts()
	var str string
	for i := len(parts) - 1; i >= 0; i-- {
		str += strings.Replace(strings.Title(parts[i]), "-", "", -1)
	}
	if str == "" {
		return "All"
	}
	return str
}

func (s *permissionScheme) AllowedContexts() []contextType {
	if s.contexts != nil {
		return s.contexts
	}
	parent := s
	for parent != nil {
		if parent.contexts != nil {
			return parent.contexts
		}
		parent = parent.parent
	}
	return nil
}

type Permission struct {
	Scheme  *permissionScheme
	Context Context
}

type Token interface {
	Permissions() ([]Permission, error)
}

func Check(token Token, schemeString string, contexts ...Context) bool {
	perms, err := token.Permissions()
	if err != nil {
		return false
	}
	scheme := PermissionRegistry.get(schemeString)
	for _, perm := range perms {
		if perm.Scheme.isParent(scheme) {
			if perm.Context.CtxType == CtxGlobal {
				return true
			}
			for _, ctx := range contexts {
				if ctx.CtxType == perm.Context.CtxType {
					if reflect.DeepEqual(ctx.Value, perm.Context.Value) {
						return true
					}
				}
			}
		}
	}
	return false
}
