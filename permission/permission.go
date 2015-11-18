// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package permission

import (
	"fmt"
	"net/http"
	"reflect"
	"strings"

	"github.com/tsuru/tsuru/errors"
)

var ErrUnauthorized = &errors.HTTP{Code: http.StatusUnauthorized, Message: "You don't have permission to do this action"}

type permissionScheme struct {
	name     string
	parent   *permissionScheme
	contexts []contextType
}

type PermissionSchemeList []*permissionScheme

type permissionContext struct {
	CtxType contextType
	Value   string
}

func Context(t contextType, v string) permissionContext {
	return permissionContext{CtxType: t, Value: v}
}

func Contexts(t contextType, values []string) []permissionContext {
	contexts := make([]permissionContext, len(values))
	for i, v := range values {
		contexts[i] = permissionContext{CtxType: t, Value: v}
	}
	return contexts
}

type contextType string

var (
	CtxGlobal          = contextType("global")
	CtxApp             = contextType("app")
	CtxTeam            = contextType("team")
	CtxPool            = contextType("pool")
	CtxIaaS            = contextType("iaas")
	CtxServiceInstance = contextType("service-instance")

	ContextTypes = []contextType{
		CtxGlobal, CtxApp, CtxTeam, CtxPool, CtxIaaS, CtxServiceInstance,
	}
)

func parseContext(ctx string) (contextType, error) {
	for _, t := range ContextTypes {
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
	contexts := []contextType{CtxGlobal}
	if s.contexts != nil {
		return append(contexts, s.contexts...)
	}
	parent := s
	for parent != nil {
		if parent.contexts != nil {
			return append(contexts, parent.contexts...)
		}
		parent = parent.parent
	}
	return contexts
}

type Permission struct {
	Scheme  *permissionScheme
	Context permissionContext
}

type Token interface {
	Permissions() ([]Permission, error)
}

func ContextsForPermission(token Token, scheme *permissionScheme, ctxTypes ...contextType) []permissionContext {
	perms, err := token.Permissions()
	if err != nil {
		return []permissionContext{}
	}
	var contexts []permissionContext
	for _, perm := range perms {
		if perm.Scheme.isParent(scheme) {
			if len(ctxTypes) > 0 {
				for _, t := range ctxTypes {
					if t == perm.Context.CtxType {
						contexts = append(contexts, perm.Context)
					}
				}
			} else {
				contexts = append(contexts, perm.Context)

			}
		}
	}
	return contexts
}

func Check(token Token, scheme *permissionScheme, contexts ...permissionContext) bool {
	perms, err := token.Permissions()
	if err != nil {
		return false
	}
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
