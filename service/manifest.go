// Copyright 2026 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/tsuru/tsuru/db/storagev2"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/permission"
	mongoBSON "go.mongodb.org/mongo-driver/bson"
)

var (
	manifestActionValidationRegexp = regexp.MustCompile(`^[a-z0-9-]+(\.[a-z0-9-]+)*$`)
	validManifestHTTPMethods       = map[string]struct{}{
		http.MethodDelete:  {},
		http.MethodGet:     {},
		http.MethodHead:    {},
		http.MethodOptions: {},
		http.MethodPatch:   {},
		http.MethodPost:    {},
		http.MethodPut:     {},
	}
	// Using NUL character commonly used for separator between values for a single concatenated key
	routeKeySeparator = "\x00"
)

type ManifestGrantConflict struct {
	Action string   `json:"action"`
	Roles  []string `json:"roles"`
}

type ManifestConflictError struct {
	Service   string                  `json:"service"`
	Conflicts []ManifestGrantConflict `json:"conflicts"`
}

func (e *ManifestConflictError) Error() string {
	if e == nil {
		return ""
	}
	parts := make([]string, 0, len(e.Conflicts))
	for _, conflict := range e.Conflicts {
		parts = append(parts, fmt.Sprintf("%s (%s)", conflict.Action, strings.Join(conflict.Roles, ", ")))
	}
	return fmt.Sprintf("manifest for service %q would orphan active dynamic grants: %s", e.Service, strings.Join(parts, "; "))
}

func UpdateManifest(ctx context.Context, serviceName string, manifest *ServiceManifest, force bool) error {
	svc, err := Get(ctx, serviceName)
	if err != nil {
		return err
	}
	return svc.IngestManifest(ctx, manifest, force)
}

func (s *Service) IngestManifest(ctx context.Context, manifest *ServiceManifest, force bool) error {
	normalized, err := normalizeManifest(manifest, s.Manifest)
	if err != nil {
		return &tsuruErrors.ValidationError{Message: err.Error()}
	}
	removedActions := removedManifestActions(s.Manifest, normalized)
	conflicts, err := manifestGrantConflicts(ctx, s.Name, removedActions)
	if err != nil {
		return err
	}
	if len(conflicts) > 0 && !force {
		return &ManifestConflictError{Service: s.Name, Conflicts: conflicts}
	}
	if len(conflicts) > 0 {
		log.Errorf("WARNING: manifest ingest for service %q forced removal of actions: %#v", s.Name, conflicts)
	}
	if err := persistManifest(ctx, s.Name, normalized); err != nil {
		return err
	}
	s.Manifest = normalized
	return nil
}

func normalizeManifest(manifest *ServiceManifest, previous *ServiceManifest) (*ServiceManifest, error) {
	if manifest == nil {
		return nil, nil
	}
	normalized := &ServiceManifest{
		Enabled:       manifest.Enabled,
		StrictActions: manifest.StrictActions,
		LegacyCompat:  manifest.LegacyCompat,
		Operations:    make([]ManifestOperation, 0, len(manifest.Operations)),
	}
	if normalized.LegacyCompat {
		switch {
		case manifest.LegacyEnabledAt != nil:
			t := manifest.LegacyEnabledAt.UTC()
			normalized.LegacyEnabledAt = &t
		case previous != nil && previous.LegacyCompat && previous.LegacyEnabledAt != nil:
			t := previous.LegacyEnabledAt.UTC()
			normalized.LegacyEnabledAt = &t
		default:
			now := time.Now().UTC()
			normalized.LegacyEnabledAt = &now
		}
	}
	seenActions := map[string]struct{}{}
	seenRoutes := map[string]struct{}{}
	for _, rawOp := range manifest.Operations {
		op := ManifestOperation{
			Method: strings.ToUpper(strings.TrimSpace(rawOp.Method)),
			Path:   manifestPatternPath(rawOp.Path),
			Action: strings.TrimSpace(rawOp.Action),
		}
		if op.Action == "" {
			return nil, fmt.Errorf("manifest action is required for operation %q", op.Path)
		}
		if _, ok := seenActions[op.Action]; ok {
			return nil, fmt.Errorf("duplicate manifest operation action %q", op.Action)
		}
		seenActions[op.Action] = struct{}{}
		if _, ok := validManifestHTTPMethods[op.Method]; !ok {
			return nil, fmt.Errorf("invalid manifest method %q for operation %q", op.Method, op.Action)
		}
		if !manifestActionValidationRegexp.MatchString(op.Action) {
			return nil, fmt.Errorf("invalid manifest action %q for operation %q", op.Action, op.Path)
		}
		if strings.TrimSpace(rawOp.Path) == "" {
			return nil, fmt.Errorf("manifest path is required for operation %q", op.Action)
		}
		routeKey := op.Method + routeKeySeparator + op.Path
		if _, ok := seenRoutes[routeKey]; ok {
			return nil, fmt.Errorf("duplicate manifest route %s %s", op.Method, op.Path)
		}
		seenRoutes[routeKey] = struct{}{}
		normalized.Operations = append(normalized.Operations, op)
	}
	if _, _, err := normalized.compiledMatcher(); err != nil {
		return nil, err
	}
	return normalized, nil
}

func removedManifestActions(current *ServiceManifest, next *ServiceManifest) []string {
	currentActions := manifestActionNames(current)
	nextActions := manifestActionNames(next)
	var removed []string
	for existingAction := range currentActions {
		if _, shouldKeepAction := nextActions[existingAction]; !shouldKeepAction {
			removed = append(removed, existingAction)
		}
	}
	sort.Strings(removed)
	return removed
}

func manifestActionNames(manifest *ServiceManifest) map[string]struct{} {
	result := map[string]struct{}{}
	if manifest == nil {
		return result
	}
	for _, op := range manifest.Operations {
		action := strings.TrimSpace(op.Action)
		if action == "" {
			continue
		}
		result[action] = struct{}{}
	}
	return result
}

func persistManifest(ctx context.Context, serviceName string, manifest *ServiceManifest) error {
	servicesCollection, err := storagev2.ServicesCollection()
	if err != nil {
		return err
	}
	update := mongoBSON.M{"$unset": mongoBSON.M{"manifest": 1}}
	if manifest != nil {
		update = mongoBSON.M{"$set": mongoBSON.M{"manifest": manifest}}
	}
	result, err := servicesCollection.UpdateOne(ctx, mongoBSON.M{"_id": serviceName}, update)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return ErrServiceNotFound
	}
	return nil
}

func manifestGrantConflicts(ctx context.Context, serviceName string, removedActions []string) ([]ManifestGrantConflict, error) {
	if len(removedActions) == 0 {
		return nil, nil
	}
	targetPerms := make(map[string]string, len(removedActions))
	permNames := make([]string, 0, len(removedActions))
	for _, action := range removedActions {
		permName := permission.DynamicActionPermissionName(serviceName, action)
		targetPerms[permName] = action
		permNames = append(permNames, permName)
	}
	rolesCollection, err := storagev2.RolesCollection()
	if err != nil {
		return nil, err
	}
	cursor, err := rolesCollection.Find(ctx, mongoBSON.M{
		"dynamic_scheme_names": mongoBSON.M{"$in": permNames},
	})
	if err != nil {
		return nil, err
	}
	var roles []permission.Role
	if err := cursor.All(ctx, &roles); err != nil {
		return nil, err
	}
	roleNamesByPermission := make(map[string][]string, len(permNames))
	for _, role := range roles {
		for _, permName := range role.DynamicSchemeNames {
			if _, ok := targetPerms[permName]; ok {
				roleNamesByPermission[permName] = append(roleNamesByPermission[permName], role.Name)
			}
		}
	}
	conflicts := make([]ManifestGrantConflict, 0, len(roleNamesByPermission))
	for permName, roleNames := range roleNamesByPermission {
		sort.Strings(roleNames)
		conflicts = append(conflicts, ManifestGrantConflict{
			Action: targetPerms[permName],
			Roles:  roleNames,
		})
	}
	sort.Slice(conflicts, func(i, j int) bool {
		return conflicts[i].Action < conflicts[j].Action
	})
	return conflicts, nil
}
