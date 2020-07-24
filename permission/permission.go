// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package permission

import (
	"fmt"
	"net/http"
	"reflect"
	"strings"

	"github.com/pkg/errors"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/log"
	permTypes "github.com/tsuru/tsuru/types/permission"
)

var ErrUnauthorized = &tsuruErrors.HTTP{Code: http.StatusForbidden, Message: "You don't have permission to do this action"}
var ErrTooManyTeams = &tsuruErrors.HTTP{Code: http.StatusBadRequest, Message: "You must provide a team to execute this action."}

type PermissionScheme struct {
	name     string
	parent   *PermissionScheme
	contexts []permTypes.ContextType
}

type PermissionSchemeList []*PermissionScheme

func Context(t permTypes.ContextType, v string) permTypes.PermissionContext {
	return permTypes.PermissionContext{CtxType: t, Value: v}
}

func Contexts(t permTypes.ContextType, values []string) []permTypes.PermissionContext {
	contexts := make([]permTypes.PermissionContext, len(values))
	for i, v := range values {
		contexts[i] = permTypes.PermissionContext{CtxType: t, Value: v}
	}
	return contexts
}

func ParseContext(ctx string) (permTypes.ContextType, error) {
	return parseContext(ctx)
}

func parseContext(ctx string) (permTypes.ContextType, error) {
	for _, t := range permTypes.ContextTypes {
		if string(t) == ctx {
			return t, nil
		}
	}
	return "", errors.Errorf("invalid context type %q", ctx)
}

func (l PermissionSchemeList) Len() int           { return len(l) }
func (l PermissionSchemeList) Swap(i, j int)      { l[i], l[j] = l[j], l[i] }
func (l PermissionSchemeList) Less(i, j int) bool { return l[i].FullName() < l[j].FullName() }

func (s *PermissionScheme) nameParts() []string {
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

func (s *PermissionScheme) IsParent(other *PermissionScheme) bool {
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

func (s *PermissionScheme) FullName() string {
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

func (s *PermissionScheme) Identifier() string {
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

func (s *PermissionScheme) AllowedContexts() []permTypes.ContextType {
	contexts := []permTypes.ContextType{permTypes.CtxGlobal}
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
	Scheme  *PermissionScheme
	Context permTypes.PermissionContext
}

func (p *Permission) String() string {
	value := p.Context.Value
	if value != "" {
		value = " " + value
	}
	return fmt.Sprintf("%s(%s%s)", p.Scheme.FullName(), p.Context.CtxType, value)
}

type Token interface {
	Permissions() ([]Permission, error)
}

func ListContextValues(t Token, scheme *PermissionScheme, failIfEmpty bool) ([]string, error) {
	contexts := ContextsForPermission(t, scheme)
	if len(contexts) == 0 && failIfEmpty {
		return nil, ErrUnauthorized
	}
	values := make([]string, 0, len(contexts))
	for _, ctx := range contexts {
		if ctx.CtxType == permTypes.CtxGlobal {
			return nil, nil
		}
		values = append(values, ctx.Value)
	}
	return values, nil
}

func ContextsFromListForPermission(perms []Permission, scheme *PermissionScheme, ctxTypes ...permTypes.ContextType) []permTypes.PermissionContext {
	var contexts []permTypes.PermissionContext
	for _, perm := range perms {
		if perm.Scheme.IsParent(scheme) {
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

func ContextsForPermission(token Token, scheme *PermissionScheme, ctxTypes ...permTypes.ContextType) []permTypes.PermissionContext {
	perms, err := token.Permissions()
	if err != nil {
		return []permTypes.PermissionContext{}
	}
	return ContextsFromListForPermission(perms, scheme, ctxTypes...)
}

func Check(token Token, scheme *PermissionScheme, contexts ...permTypes.PermissionContext) bool {
	perms, err := token.Permissions()
	if err != nil {
		log.Errorf("unable to read token permissions: %v", err)
		return false
	}
	return CheckFromPermList(perms, scheme, contexts...)
}

func CheckFromPermList(perms []Permission, scheme *PermissionScheme, contexts ...permTypes.PermissionContext) bool {
	for _, perm := range perms {
		if perm.Scheme.IsParent(scheme) {
			if perm.Context.CtxType == permTypes.CtxGlobal {
				return true
			}
			for _, ctx := range contexts {
				if ctx.CtxType == perm.Context.CtxType && ctx.Value == perm.Context.Value {
					return true
				}
			}
		}
	}
	return false
}

func TeamForPermission(t Token, scheme *PermissionScheme) (string, error) {
	allContexts := ContextsForPermission(t, scheme)
	teams := make([]string, 0, len(allContexts))
	for _, ctx := range allContexts {
		if ctx.CtxType == permTypes.CtxGlobal {
			teams = nil
			break
		}
		if ctx.CtxType == permTypes.CtxTeam {
			teams = append(teams, ctx.Value)
		}
	}
	if teams != nil && len(teams) == 0 {
		return "", ErrUnauthorized
	}
	if len(teams) == 1 {
		return teams[0], nil
	}
	return "", ErrTooManyTeams
}
