// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package permission

import (
	"context"
	"sort"
	"strings"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/db/storagev2"
	permTypes "github.com/tsuru/tsuru/types/permission"
	mongoBSON "go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

type Role struct {
	Name        string                `bson:"_id" json:"name"`
	ContextType permTypes.ContextType `json:"context"`
	Description string
	SchemeNames []string `json:"scheme_names,omitempty"`
	Events      []string `json:"events,omitempty"`
}

func NewRole(ctx context.Context, name string, permissionCtx string, description string) (Role, error) {
	ctxType, err := parseContext(permissionCtx)
	if err != nil {
		return Role{}, err
	}
	name = strings.TrimSpace(name)
	if len(name) == 0 {
		return Role{}, permTypes.ErrInvalidRoleName
	}

	collection, err := storagev2.RolesCollection()
	if err != nil {
		return Role{}, err
	}
	role := Role{Name: name, ContextType: ctxType, Description: description}
	_, err = collection.InsertOne(ctx, role)
	if mongo.IsDuplicateKeyError(err) {
		return Role{}, permTypes.ErrRoleAlreadyExists
	}
	return role, err
}

func ListRoles(ctx context.Context) ([]Role, error) {
	var roles []Role
	collection, err := storagev2.RolesCollection()
	if err != nil {
		return roles, err
	}

	cursor, err := collection.Find(ctx, mongoBSON.M{})
	if err != nil {
		return nil, err
	}
	err = cursor.All(ctx, &roles)
	if err != nil {
		return nil, err
	}
	for i := range roles {
		roles[i].filterValidSchemes()
	}
	return roles, nil
}

func ListRolesWithEvents(ctx context.Context) ([]Role, error) {
	var roles []Role
	collection, err := storagev2.RolesCollection()
	if err != nil {
		return roles, err
	}
	cursor, err := collection.Find(ctx, mongoBSON.M{"events": mongoBSON.M{"$not": mongoBSON.M{"$size": 0}, "$exists": true}})
	if err != nil {
		return nil, err
	}
	err = cursor.All(ctx, &roles)
	if err != nil {
		return nil, err
	}
	for i := range roles {
		roles[i].filterValidSchemes()
	}
	return roles, nil
}

func ListRolesForEvent(ctx context.Context, evt *permTypes.RoleEvent) ([]Role, error) {
	if evt == nil {
		return nil, errors.New("invalid role event")
	}
	var roles []Role
	collection, err := storagev2.RolesCollection()
	if err != nil {
		return roles, err
	}
	cursor, err := collection.Find(ctx, mongoBSON.M{"events": evt.Name})
	if err != nil {
		return nil, err
	}
	err = cursor.All(ctx, &roles)
	if err != nil {
		return nil, err
	}
	for i := range roles {
		roles[i].filterValidSchemes()
	}
	return roles, nil
}

// ListRolesWithPermissionWithContextMap returns a map with all roles valid for a
// specific Context or having any scheme permission which is valid for the specific Context.
func ListRolesWithPermissionWithContextMap(ctx context.Context, contextValue permTypes.ContextType) (map[string]Role, error) {
	allRoles, err := ListRoles(ctx)
	if err != nil {
		return nil, err
	}

	filteredRoles := make(map[string]Role)
	for _, role := range allRoles {
		if role.ContextType == contextValue || role.hasPermissionWithContext(contextValue) {
			filteredRoles[role.Name] = role
		}
	}

	return filteredRoles, nil
}

func (r *Role) hasPermissionWithContext(contextValue permTypes.ContextType) bool {
	for _, schemeName := range r.SchemeNames {
		scheme, err := SafeGet(schemeName)
		if err != nil {
			continue
		}
		for _, sCtx := range scheme.AllowedContexts() {
			if sCtx == contextValue {
				return true
			}
		}
	}
	return false
}

func FindRole(ctx context.Context, name string) (Role, error) {
	var role Role
	collection, err := storagev2.RolesCollection()
	if err != nil {
		return role, err
	}
	err = collection.FindOne(ctx, mongoBSON.M{"_id": name}).Decode(&role)
	if err == mongo.ErrNoDocuments {
		return role, permTypes.ErrRoleNotFound
	}
	if err != nil {
		return role, err
	}
	role.filterValidSchemes()
	return role, nil
}

func DestroyRole(ctx context.Context, name string) error {
	collection, err := storagev2.RolesCollection()
	if err != nil {
		return err
	}

	result, err := collection.DeleteOne(ctx, mongoBSON.M{"_id": name})

	if err == mongo.ErrNoDocuments {
		return permTypes.ErrRoleNotFound
	}

	if err != nil {
		return err
	}

	if result.DeletedCount == 0 {
		return permTypes.ErrRoleNotFound
	}

	return nil
}

func (r *Role) AddPermissions(ctx context.Context, permNames ...string) error {
	for _, permName := range permNames {
		if permName == "" {
			return permTypes.ErrInvalidPermissionName
		}
		if permName == "*" {
			permName = ""
		}
		reg := PermissionRegistry.getSubRegistry(permName)
		if reg == nil {
			return &permTypes.ErrPermissionNotFound{Permission: permName}
		}
		var found bool
		for _, ctxType := range reg.AllowedContexts() {
			if ctxType == r.ContextType {
				found = true
				break
			}
		}
		if !found {
			return &permTypes.ErrPermissionNotAllowed{
				Permission:  permName,
				ContextType: r.ContextType,
			}
		}
	}
	collection, err := storagev2.RolesCollection()
	if err != nil {
		return err
	}
	_, err = collection.UpdateOne(ctx, mongoBSON.M{"_id": r.Name}, mongoBSON.M{"$addToSet": mongoBSON.M{"schemenames": mongoBSON.M{"$each": permNames}}})
	if err != nil {
		return err
	}
	dbRole, err := FindRole(ctx, r.Name)
	if err != nil {
		return err
	}
	r.SchemeNames = dbRole.SchemeNames
	return nil
}

func (r *Role) RemovePermissions(ctx context.Context, permNames ...string) error {
	collection, err := storagev2.RolesCollection()
	if err != nil {
		return err
	}
	_, err = collection.UpdateOne(ctx, mongoBSON.M{"_id": r.Name}, mongoBSON.M{"$pullAll": mongoBSON.M{"schemenames": permNames}})
	if err != nil {
		return err
	}
	dbRole, err := FindRole(ctx, r.Name)
	if err != nil {
		return err
	}
	r.SchemeNames = dbRole.SchemeNames
	return nil
}

func (r *Role) filterValidSchemes() PermissionSchemeList {
	schemes := make(PermissionSchemeList, 0, len(r.SchemeNames))
	sort.Strings(r.SchemeNames)
	for i := 0; i < len(r.SchemeNames); i++ {
		schemeName := r.SchemeNames[i]
		if schemeName == "*" {
			schemeName = ""
		}
		scheme := PermissionRegistry.getSubRegistry(schemeName)
		if scheme == nil {
			// permission schemes might be removed or renamed, invalid entries
			// in the database shouldn't be a problem.
			r.SchemeNames = append(r.SchemeNames[:i], r.SchemeNames[i+1:]...)
			i--
			continue
		}
		schemes = append(schemes, &scheme.PermissionScheme)
	}
	return schemes
}

func (r *Role) PermissionsFor(contextValue string) []Permission {
	schemes := r.filterValidSchemes()
	permissions := make([]Permission, len(schemes))
	for i, scheme := range schemes {
		permissions[i] = Permission{
			Scheme: scheme,
			Context: permTypes.PermissionContext{
				CtxType: r.ContextType,
				Value:   contextValue,
			},
		}
	}
	return permissions
}

func (r *Role) AddEvent(ctx context.Context, eventName string) error {
	roleEvent := permTypes.RoleEventMap[eventName]
	if roleEvent == nil {
		return permTypes.ErrRoleEventNotFound
	}
	if r.ContextType != roleEvent.Context {
		return permTypes.ErrRoleEventWrongContext{Expected: string(roleEvent.Context), Role: string(r.ContextType)}
	}
	collection, err := storagev2.RolesCollection()
	if err != nil {
		return err
	}
	_, err = collection.UpdateOne(ctx, mongoBSON.M{"_id": r.Name}, mongoBSON.M{"$addToSet": mongoBSON.M{"events": eventName}})
	if err != nil {
		return err
	}
	dbRole, err := FindRole(ctx, r.Name)
	if err != nil {
		return err
	}
	r.Events = dbRole.Events
	return nil
}

func (r *Role) RemoveEvent(ctx context.Context, eventName string) error {
	collection, err := storagev2.RolesCollection()
	if err != nil {
		return err
	}
	_, err = collection.UpdateOne(ctx, mongoBSON.M{"_id": r.Name}, mongoBSON.M{"$pull": mongoBSON.M{"events": eventName}})
	if err != nil {
		return err
	}
	dbRole, err := FindRole(ctx, r.Name)
	if err != nil {
		return err
	}
	r.Events = dbRole.Events
	return nil
}

func (r *Role) Update(ctx context.Context) error {
	collection, err := storagev2.RolesCollection()
	if err != nil {
		return err
	}
	err = collection.FindOneAndUpdate(ctx, mongoBSON.M{"_id": r.Name}, mongoBSON.M{"$set": mongoBSON.M{"contexttype": r.ContextType, "description": r.Description}}).Err()

	if err != nil {
		return err
	}

	return nil
}

func (r *Role) Add(ctx context.Context) error {
	name := strings.TrimSpace(r.Name)
	if len(name) == 0 {
		return permTypes.ErrInvalidRoleName
	}
	collection, err := storagev2.RolesCollection()
	if err != nil {
		return err
	}
	insertRole := Role{Name: name, ContextType: r.ContextType, Description: r.Description, SchemeNames: r.SchemeNames, Events: r.Events}
	_, err = collection.InsertOne(ctx, insertRole)
	if mongo.IsDuplicateKeyError(err) {
		return permTypes.ErrRoleAlreadyExists
	}
	return nil
}
