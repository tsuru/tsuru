// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package permission

import (
	"context"
	"net/http"

	"github.com/pkg/errors"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/log"
	permTypes "github.com/tsuru/tsuru/types/permission"
)

var (
	ErrUnauthorized = &tsuruErrors.HTTP{Code: http.StatusForbidden, Message: "You don't have permission to do this action"}
	ErrTooManyTeams = &tsuruErrors.HTTP{Code: http.StatusBadRequest, Message: "You must provide a team to execute this action."}
)

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

type Token interface {
	Permissions(ctx context.Context) ([]permTypes.Permission, error)
}

func ContextsFromListForPermission(perms []permTypes.Permission, scheme *permTypes.PermissionScheme, ctxTypes ...permTypes.ContextType) []permTypes.PermissionContext {
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

func ContextsForPermission(ctx context.Context, token Token, scheme *permTypes.PermissionScheme, ctxTypes ...permTypes.ContextType) []permTypes.PermissionContext {
	perms, err := token.Permissions(ctx)
	if err != nil {
		return []permTypes.PermissionContext{}
	}
	return ContextsFromListForPermission(perms, scheme, ctxTypes...)
}

func Check(ctx context.Context, token Token, scheme *permTypes.PermissionScheme, contexts ...permTypes.PermissionContext) bool {
	perms, err := token.Permissions(ctx)
	if err != nil {
		log.Errorf("unable to read token permissions: %v", err)
		return false
	}
	return CheckFromPermList(perms, scheme, contexts...)
}

func CheckFromPermList(perms []permTypes.Permission, scheme *permTypes.PermissionScheme, contexts ...permTypes.PermissionContext) bool {
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

func TeamForPermission(ctx context.Context, t Token, scheme *permTypes.PermissionScheme) (string, error) {
	allContexts := ContextsForPermission(ctx, t, scheme)
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
