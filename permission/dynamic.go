// Copyright 2026 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package permission

import (
	"context"
	"fmt"
	"strings"

	"github.com/tsuru/tsuru/db/storagev2"
	permTypes "github.com/tsuru/tsuru/types/permission"
	serviceTypes "github.com/tsuru/tsuru/types/service"
	mongoBSON "go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

const (
	DynamicPermissionPrefix = "service-action"
)

// Dynamic schemes are pure values derived from the permission name; the
// service manifests stored in the database are the source of truth for which
// action names exist.
var dynamicPermissionContexts = []permTypes.ContextType{
	permTypes.CtxServiceInstance,
	permTypes.CtxService,
	permTypes.CtxTeam,
}

// NewDynamic builds the scheme (with its parent lineage) for a dynamic
// permission name.
func NewDynamic(name string) (*permTypes.PermissionScheme, bool) {
	if !IsDynamicPermissionName(name) {
		return nil, false
	}
	var scheme *permTypes.PermissionScheme
	for part := range strings.SplitSeq(name, ".") {
		if part == DynamicPermissionPrefix {
			continue
		}
		if scheme == nil {
			part = fmt.Sprintf("%s.%s", DynamicPermissionPrefix, part)
		}
		scheme = &permTypes.PermissionScheme{Name: part, Parent: scheme, Contexts: dynamicPermissionContexts}
	}
	return scheme, true
}

// ExistsDynamic reports whether name is an ancestor-or-equal of an action
// declared by an enabled service manifest.
func ExistsDynamic(ctx context.Context, name string) (bool, error) {
	serviceName, _, _ := strings.Cut(strings.TrimPrefix(name, DynamicPermissionPrefix+"."), ".")
	servicesCollection, err := storagev2.ServicesCollection()
	if err != nil {
		return false, err
	}
	var svc serviceTypes.Service
	err = servicesCollection.FindOne(ctx, mongoBSON.M{"_id": serviceName}).Decode(&svc)
	if err == mongo.ErrNoDocuments {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if svc.Manifest == nil || !svc.Manifest.Enabled {
		return false, nil
	}
	for _, op := range svc.Manifest.Operations {
		if isDynamicAncestorOrSelf(name, DynamicPermissionPrefix+"."+serviceName+"."+op.Action) {
			return true, nil
		}
	}
	return false, nil
}

func IsDynamicPermissionName(name string) bool {
	if !strings.HasPrefix(name, DynamicPermissionPrefix) {
		return false
	}
	if strings.TrimSpace(name) == DynamicPermissionPrefix {
		return false
	}
	parts := strings.Split(name, ".")
	return len(parts) >= 2
}

// DynamicActionPermissionName builds the dynamic permission name for a service
// manifest action.
func DynamicActionPermissionName(serviceName, action string) string {
	return DynamicPermissionPrefix + "." + serviceName + "." + action
}

// CheckDynamic returns true if any granted name is an ancestor-or-equal of the
// requested name and the granted permission context matches the request
// contexts. Callers must only pass requested names taken from an enabled
// service manifest.
func CheckDynamic(granted []permTypes.Permission, requested string, contexts ...permTypes.PermissionContext) bool {
	if !IsDynamicPermissionName(requested) {
		return false
	}
	for _, perm := range granted {
		if perm.Scheme == nil || !isDynamicAncestorOrSelf(perm.Scheme.FullName(), requested) {
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
