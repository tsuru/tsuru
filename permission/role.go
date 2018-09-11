// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package permission

import (
	"fmt"
	"sort"
	"strings"

	"github.com/globalsign/mgo"
	"github.com/globalsign/mgo/bson"
	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/storage"
	permTypes "github.com/tsuru/tsuru/types/permission"
)

var (
	ErrRoleNotFound          = errors.New("role not found")
	ErrRoleAlreadyExists     = errors.New("role already exists")
	ErrRoleEventNotFound     = errors.New("role event not found")
	ErrInvalidRoleName       = errors.New("invalid role name")
	ErrInvalidPermissionName = errors.New("invalid permission name")

	RoleEventUserCreate = &RoleEvent{
		name:        "user-create",
		context:     CtxGlobal,
		Description: "role added to user when user is created",
	}
	RoleEventTeamCreate = &RoleEvent{
		name:        "team-create",
		context:     CtxTeam,
		Description: "role added to user when a new team is created",
	}

	RoleEventMap = map[string]*RoleEvent{
		RoleEventUserCreate.name: RoleEventUserCreate,
		RoleEventTeamCreate.name: RoleEventTeamCreate,
	}
)

type ErrRoleEventWrongContext struct {
	expected string
	role     string
}

func (e ErrRoleEventWrongContext) Error() string {
	return fmt.Sprintf("wrong context type for role event, expected %q role has %q", e.expected, e.role)
}

type ErrPermissionNotFound struct {
	permission string
}

func (e ErrPermissionNotFound) Error() string {
	return fmt.Sprintf("permission named %q not found", e.permission)
}

type ErrPermissionNotAllowed struct {
	permission  string
	contextType permTypes.ContextType
}

func (e ErrPermissionNotAllowed) Error() string {
	return fmt.Sprintf("permission %q not allowed with context of type %q", e.permission, e.contextType)
}

type RoleEvent struct {
	name        string
	context     permTypes.ContextType
	Description string
}

func (e *RoleEvent) String() string {
	return e.name
}

type Role struct {
	Name        string                `bson:"_id" json:"name"`
	ContextType permTypes.ContextType `json:"context"`
	Description string
	SchemeNames []string `json:"scheme_names,omitempty"`
	Events      []string `json:"events,omitempty"`
}

func NewRole(name string, ctx string, description string) (Role, error) {
	ctxType, err := parseContext(ctx)
	if err != nil {
		return Role{}, err
	}
	name = strings.TrimSpace(name)
	if len(name) == 0 {
		return Role{}, ErrInvalidRoleName
	}
	coll, err := rolesCollection()
	if err != nil {
		return Role{}, err
	}
	defer coll.Close()
	role := Role{Name: name, ContextType: ctxType, Description: description}
	err = coll.Insert(role)
	if mgo.IsDup(err) {
		return Role{}, ErrRoleAlreadyExists
	}
	return role, err
}

func ListRoles() ([]Role, error) {
	var roles []Role
	coll, err := rolesCollection()
	if err != nil {
		return roles, err
	}
	defer coll.Close()
	err = coll.Find(nil).All(&roles)
	if err != nil {
		return nil, err
	}
	for i := range roles {
		roles[i].filterValidSchemes()
	}
	return roles, nil
}

func ListRolesWithEvents() ([]Role, error) {
	var roles []Role
	coll, err := rolesCollection()
	if err != nil {
		return roles, err
	}
	defer coll.Close()
	err = coll.Find(bson.M{"events": bson.M{"$not": bson.M{"$size": 0}, "$exists": true}}).All(&roles)
	if err != nil {
		return nil, err
	}
	for i := range roles {
		roles[i].filterValidSchemes()
	}
	return roles, nil
}

func ListRolesForEvent(evt *RoleEvent) ([]Role, error) {
	if evt == nil {
		return nil, errors.New("invalid role event")
	}
	var roles []Role
	coll, err := rolesCollection()
	if err != nil {
		return roles, err
	}
	defer coll.Close()
	err = coll.Find(bson.M{"events": evt.name}).All(&roles)
	if err != nil {
		return nil, err
	}
	for i := range roles {
		roles[i].filterValidSchemes()
	}
	return roles, nil
}

func FindRole(name string) (Role, error) {
	var role Role
	coll, err := rolesCollection()
	if err != nil {
		return role, err
	}
	defer coll.Close()
	err = coll.FindId(name).One(&role)
	if err == mgo.ErrNotFound {
		return role, ErrRoleNotFound
	}
	if err != nil {
		return role, err
	}
	role.filterValidSchemes()
	return role, nil
}

func DestroyRole(name string) error {
	coll, err := rolesCollection()
	if err != nil {
		return err
	}
	defer coll.Close()
	err = coll.RemoveId(name)
	if err == mgo.ErrNotFound {
		return ErrRoleNotFound
	}
	return err
}

func (r *Role) AddPermissions(permNames ...string) error {
	for _, permName := range permNames {
		if permName == "" {
			return ErrInvalidPermissionName
		}
		if permName == "*" {
			permName = ""
		}
		reg := PermissionRegistry.getSubRegistry(permName)
		if reg == nil {
			return &ErrPermissionNotFound{permission: permName}
		}
		var found bool
		for _, ctxType := range reg.AllowedContexts() {
			if ctxType == r.ContextType {
				found = true
				break
			}
		}
		if !found {
			return &ErrPermissionNotAllowed{
				permission:  permName,
				contextType: r.ContextType,
			}
		}
	}
	coll, err := rolesCollection()
	if err != nil {
		return err
	}
	defer coll.Close()
	err = coll.UpdateId(r.Name, bson.M{"$addToSet": bson.M{"schemenames": bson.M{"$each": permNames}}})
	if err != nil {
		return err
	}
	dbRole, err := FindRole(r.Name)
	if err != nil {
		return err
	}
	r.SchemeNames = dbRole.SchemeNames
	return nil
}

func (r *Role) RemovePermissions(permNames ...string) error {
	coll, err := rolesCollection()
	if err != nil {
		return err
	}
	defer coll.Close()
	err = coll.UpdateId(r.Name, bson.M{"$pullAll": bson.M{"schemenames": permNames}})
	if err != nil {
		return err
	}
	dbRole, err := FindRole(r.Name)
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

func (r *Role) AddEvent(eventName string) error {
	roleEvent := RoleEventMap[eventName]
	if roleEvent == nil {
		return ErrRoleEventNotFound
	}
	if r.ContextType != roleEvent.context {
		return ErrRoleEventWrongContext{expected: string(roleEvent.context), role: string(r.ContextType)}
	}
	coll, err := rolesCollection()
	if err != nil {
		return err
	}
	defer coll.Close()
	err = coll.UpdateId(r.Name, bson.M{"$addToSet": bson.M{"events": eventName}})
	if err != nil {
		return err
	}
	dbRole, err := FindRole(r.Name)
	if err != nil {
		return err
	}
	r.Events = dbRole.Events
	return nil
}

func (r *Role) RemoveEvent(eventName string) error {
	coll, err := rolesCollection()
	if err != nil {
		return err
	}
	defer coll.Close()
	err = coll.UpdateId(r.Name, bson.M{"$pull": bson.M{"events": eventName}})
	if err != nil {
		return err
	}
	dbRole, err := FindRole(r.Name)
	if err != nil {
		return err
	}
	r.Events = dbRole.Events
	return nil
}

func rolesCollection() (*storage.Collection, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	return conn.Roles(), nil
}

func (r *Role) Update() error {
	coll, err := rolesCollection()
	if err != nil {
		return err
	}
	defer coll.Close()
	return coll.Update(bson.M{"_id": r.Name}, bson.M{"$set": bson.M{"contexttype": r.ContextType, "description": r.Description}})
}

func (r *Role) Add() error {
	name := strings.TrimSpace(r.Name)
	if len(name) == 0 {
		return ErrInvalidRoleName
	}
	coll, err := rolesCollection()
	if err != nil {
		return err
	}
	defer coll.Close()
	insertRole := Role{Name: name, ContextType: r.ContextType, Description: r.Description, SchemeNames: r.SchemeNames, Events: r.Events}
	err = coll.Insert(insertRole)
	if mgo.IsDup(err) {
		return ErrRoleAlreadyExists
	}
	return nil
}
